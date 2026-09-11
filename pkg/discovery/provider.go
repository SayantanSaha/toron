package discovery

import (
	"context"
	"fmt"
	"time"
)

// ContainerEventType indicates the status lifecycle event of an OCI container.
type ContainerEventType string

const (
	EventStart ContainerEventType = "start"
	EventStop  ContainerEventType = "stop"
	EventDie   ContainerEventType = "die"
)

// PortMapping represents an exposed container port mapping.
type PortMapping struct {
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
	IP          string `json:"IP"`
}

// Container represents a normalized OCI container instance across Docker, Podman, Containerd, or CRI-O.
type Container struct {
	ID        string            `json:"Id"`
	Names     []string          `json:"Names"`
	Image     string            `json:"Image"`
	State     string            `json:"State"`
	Status    string            `json:"Status"`
	IPAddress string            `json:"IPAddress"`
	Labels    map[string]string `json:"Labels"`
	Ports     []PortMapping     `json:"Ports"`
}

// ContainerEvent captures real-time lifecycle event notifications.
type ContainerEvent struct {
	Type        ContainerEventType
	ContainerID string
	Container   *Container
}

// DiscoveredRoute captures routing specifications extracted from container labels or annotations.
type DiscoveredRoute struct {
	ContainerID         string
	ContainerName       string
	Host                string
	Prefix              string
	Method              string            // Normalized uppercase HTTP method constraint (e.g., "GET", "POST"), or "" for any method
	Headers             map[string]string // Canonical HTTP header match constraints (e.g., {"X-Version": "canary"})
	TargetIP            string
	TargetPort          int
	Weight              int
	HealthCheckPath     string
	HealthCheckInterval time.Duration
	StripPrefix         *bool
	RewriteRedirects    *bool
	RewriteCookiePath   *bool
}

// TargetURL formats the upstream destination URL (e.g., http://172.17.0.2:8080).
func (r *DiscoveredRoute) TargetURL() string {
	ip := r.TargetIP
	if ip == "" {
		ip = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d", ip, r.TargetPort)
}

// Provider represents an OCI container discovery source interface.
type Provider interface {
	Name() string
	ListContainers(ctx context.Context) ([]Container, error)
	WatchEvents(ctx context.Context, events chan<- ContainerEvent) error
}
