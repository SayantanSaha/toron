package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Standard OCI container runtime Unix domain socket paths.
var StandardSocketPaths = []string{
	"/var/run/docker.sock",
	"/run/podman/podman.sock",
	"/var/run/podman/podman.sock",
}

// UnixRESTProvider provides container auto-discovery over Unix domain sockets.
type UnixRESTProvider struct {
	name       string
	socketPath string
	client     *http.Client
	mu         sync.Mutex
}

// AutoDetectSocket probes known Unix domain socket paths and returns the first existing socket.
func AutoDetectSocket() string {
	for _, p := range StandardSocketPaths {
		if fi, err := os.Stat(p); err == nil && (fi.Mode()&os.ModeSocket != 0 || fi.Mode().IsRegular()) {
			return p
		}
	}
	// Check user-specific rootless podman socket (e.g. /run/user/1000/podman/podman.sock)
	if uid := os.Getuid(); uid > 0 {
		userSock := fmt.Sprintf("/run/user/%d/podman/podman.sock", uid)
		if fi, err := os.Stat(userSock); err == nil && (fi.Mode()&os.ModeSocket != 0 || fi.Mode().IsRegular()) {
			return userSock
		}
	}
	return "/var/run/docker.sock" // Default fallback
}

// NewUnixRESTProvider constructs a Unix domain socket REST provider.
func NewUnixRESTProvider(name string, socketPath string) *UnixRESTProvider {
	if socketPath == "" || socketPath == "auto" {
		socketPath = AutoDetectSocket()
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		DisableKeepAlives: false,
	}

	return &UnixRESTProvider{
		name:       name,
		socketPath: socketPath,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

func (p *UnixRESTProvider) Name() string {
	return p.name + " (" + p.socketPath + ")"
}

// dockerContainerJSON mirrors the container REST JSON schema returned by Docker/Podman engines.
type dockerContainerJSON struct {
	ID         string            `json:"Id"`
	Names      []string          `json:"Names"`
	Image      string            `json:"Image"`
	State      string            `json:"State"`
	Status     string            `json:"Status"`
	Labels     map[string]string `json:"Labels"`
	Ports      []PortMapping     `json:"Ports"`
	NetworkSettings struct {
		IPAddress string `json:"IPAddress"`
		Networks  map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

// ListContainers queries `/containers/json` over the Unix socket.
func (p *UnixRESTProvider) ListContainers(ctx context.Context) ([]Container, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://unix/containers/json?all=1", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to socket %s: %w", p.socketPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("socket endpoint returned status %d", resp.StatusCode)
	}

	var rawContainers []dockerContainerJSON
	if err := json.NewDecoder(resp.Body).Decode(&rawContainers); err != nil {
		return nil, fmt.Errorf("failed to decode container json: %w", err)
	}

	var containers []Container
	for _, c := range rawContainers {
		ip := c.NetworkSettings.IPAddress
		if ip == "" {
			for _, netConfig := range c.NetworkSettings.Networks {
				if netConfig.IPAddress != "" {
					ip = netConfig.IPAddress
					break
				}
			}
		}

		containers = append(containers, Container{
			ID:        c.ID,
			Names:     c.Names,
			Image:     c.Image,
			State:     c.State,
			Status:    c.Status,
			IPAddress: ip,
			Labels:    c.Labels,
			Ports:     c.Ports,
		})
	}

	return containers, nil
}

type dockerEventJSON struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
	Time int64 `json:"time"`
}

// WatchEvents connects to `/events` streaming endpoint and emits ContainerEvents.
func (p *UnixRESTProvider) WatchEvents(ctx context.Context, events chan<- ContainerEvent) error {
	conn, err := net.Dial("unix", p.socketPath)
	if err != nil {
		return fmt.Errorf("failed to dial socket %s: %w", p.socketPath, err)
	}
	defer conn.Close()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://unix/events?filter=event=start&filter=event=die&filter=event=stop", nil)
	if err != nil {
		return err
	}

	if err := req.Write(conn); err != nil {
		return fmt.Errorf("failed to write request to socket: %w", err)
	}

	reader := bufio.NewReader(conn)
	// Read HTTP response header line (e.g., HTTP/1.1 200 OK)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read response line: %w", err)
	}
	if !strings.Contains(line, "200") {
		return fmt.Errorf("events endpoint returned header: %s", line)
	}

	// Skip response headers until blank line
	for {
		headerLine, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.TrimSpace(headerLine) == "" {
			break
		}
	}

	// Stream event JSON objects line by line
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lineBytes, err := reader.ReadBytes('\n')
		if err != nil {
			return err
		}

		lineStr := strings.TrimSpace(string(lineBytes))
		if lineStr == "" {
			continue
		}

		var evt dockerEventJSON
		if err := json.Unmarshal([]byte(lineStr), &evt); err != nil {
			continue
		}

		var eventType ContainerEventType
		switch evt.Action {
		case "start":
			eventType = EventStart
		case "die", "stop":
			eventType = EventStop
		default:
			continue
		}

		cID := evt.Actor.ID
		if cID == "" {
			continue
		}

		// Fetch full container details for the event actor
		var cObj *Container
		if eventType == EventStart {
			if list, err := p.ListContainers(ctx); err == nil {
				for _, item := range list {
					if item.ID == cID || strings.HasPrefix(item.ID, cID) {
						cObj = &item
						break
					}
				}
			}
		}

		events <- ContainerEvent{
			Type:        eventType,
			ContainerID: cID,
			Container:   cObj,
		}
	}
}
