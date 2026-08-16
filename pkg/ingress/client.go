package ingress

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"toron/pkg/config"
)

// Default Kubernetes in-cluster paths.
const (
	InClusterAPIServer = "https://kubernetes.default.svc"
	ServiceAccountDir  = "/var/run/secrets/kubernetes.io/serviceaccount"
	TokenFile          = "token"
	CACertFile         = "ca.crt"
)

// Client is a zero-dependency Kubernetes REST API client.
type Client struct {
	apiServer   string
	bearerToken string
	httpClient  *http.Client
}

// NewClient constructs a K8s REST API client using in-cluster or custom config.
func NewClient(cfg config.IngressConfig) (*Client, error) {
	apiServer := cfg.KubeAPIServer
	if apiServer == "" {
		apiServer = InClusterAPIServer
	}

	saDir := cfg.ServiceAccountDir
	if saDir == "" {
		saDir = ServiceAccountDir
	}

	token := ""
	tokenPath := filepath.Join(saDir, TokenFile)
	if tokenBytes, err := os.ReadFile(tokenPath); err == nil {
		token = strings.TrimSpace(string(tokenBytes))
	}

	tlsConfig := &tls.Config{}
	caPath := filepath.Join(saDir, CACertFile)
	if caBytes, err := os.ReadFile(caPath); err == nil {
		caPool := x509.NewCertPool()
		caPool.AppendCertsFromPEM(caBytes)
		tlsConfig.RootCAs = caPool
	} else {
		// Allow insecure fallback for local development/test apiserver endpoints
		if strings.HasPrefix(apiServer, "http://") || strings.Contains(apiServer, "localhost") || strings.Contains(apiServer, "127.0.0.1") {
			tlsConfig.InsecureSkipVerify = true
		}
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 30 * time.Second,
	}

	return &Client{
		apiServer:   strings.TrimSuffix(apiServer, "/"),
		bearerToken: token,
		httpClient:  httpClient,
	}, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string) (*http.Request, error) {
	urlStr := c.apiServer + path
	req, err := http.NewRequestWithContext(ctx, method, urlStr, nil)
	if err != nil {
		return nil, err
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// ListIngresses fetches all `networking.k8s.io/v1` Ingress objects across namespaces.
func (c *Client) ListIngresses(ctx context.Context) ([]Ingress, error) {
	req, err := c.newRequest(ctx, "GET", "/apis/networking.k8s.io/v1/ingresses")
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list ingresses from %s: %w", c.apiServer, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("k8s apiserver returned status %d", resp.StatusCode)
	}

	var list IngressList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("failed to decode ingress list: %w", err)
	}

	return list.Items, nil
}

// GetEndpoints fetches a Service's Endpoints object in a namespace.
func (c *Client) GetEndpoints(ctx context.Context, namespace, serviceName string) (*Endpoints, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/endpoints/%s", namespace, serviceName)
	req, err := c.newRequest(ctx, "GET", path)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endpoints %s/%s returned status %d", namespace, serviceName, resp.StatusCode)
	}

	var ep Endpoints
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return nil, err
	}

	return &ep, nil
}

// GetSecret fetches a K8s Secret object in a namespace.
func (c *Client) GetSecret(ctx context.Context, namespace, secretName string) (*Secret, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/secrets/%s", namespace, secretName)
	req, err := c.newRequest(ctx, "GET", path)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("secret %s/%s returned status %d", namespace, secretName, resp.StatusCode)
	}

	var sec Secret
	if err := json.NewDecoder(resp.Body).Decode(&sec); err != nil {
		return nil, err
	}

	return &sec, nil
}

// WatchIngresses streams real-time Ingress watch events (`watch=true`).
func (c *Client) WatchIngresses(ctx context.Context, events chan<- K8sWatchEvent) error {
	req, err := c.newRequest(ctx, "GET", "/apis/networking.k8s.io/v1/ingresses?watch=true")
	if err != nil {
		return err
	}

	// Disable HTTP client timeout for streaming watch connection
	watchClient := *c.httpClient
	watchClient.Timeout = 0

	resp, err := watchClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("watch ingresses returned status %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			return err
		}

		if len(line) == 0 {
			continue
		}

		var evt K8sWatchEvent
		if err := json.Unmarshal(line, &evt); err == nil && evt.Type != "" {
			events <- evt
		}
	}
}
