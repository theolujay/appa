package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Caddy JSON Spec-compliant structs
type caddyRoute struct {
	ID       string        `json:"@id,omitempty"`
	Match    []caddyMatch  `json:"match,omitempty"`
	Handle   []caddyHandle `json:"handle,omitempty"`
	Terminal bool          `json:"terminal,omitempty"`
}

type caddyMatch struct {
	Host []string `json:"host,omitempty"`
	Path []string `json:"path,omitempty"`
}

type caddyHandle struct {
	Handler         string                 `json:"handler"`
	Upstreams       []caddyUpstream        `json:"upstreams,omitempty"`
	Headers         *caddyHeaderOpsWrapper `json:"headers,omitempty"`
	StripPathPrefix string                 `json:"strip_path_prefix,omitempty"`
}

type caddyHeaderOpsWrapper struct {
	Request *caddyHeaderOps `json:"request,omitempty"`
}

type caddyHeaderOps struct {
	Set map[string][]string `json:"set,omitempty"`
}

type caddyUpstream struct {
	Dial string `json:"dial,omitempty"`
}

func (p *Pipeline) AddRoute(deploymentID, address string) error {
	routeID := fmt.Sprintf("deployment-%s", deploymentID)

	// Ensure idempotency by removing old route
	_ = p.RemoveRoute(deploymentID)

	// Construct route according to Caddy JSON handler spec
	route := caddyRoute{
		ID: routeID,
		Match: []caddyMatch{
			{
				Host: []string{"localhost", "127.0.0.1"},
				Path: []string{
					fmt.Sprintf("/deploys/%s", deploymentID),
					fmt.Sprintf("/deploys/%s/*", deploymentID),
				},
			},
		},
		Handle: []caddyHandle{
			{
				Handler:         "rewrite",
				StripPathPrefix: fmt.Sprintf("/deploys/%s", deploymentID),
			},
			{
				Handler:   "reverse_proxy",
				Upstreams: []caddyUpstream{{Dial: address}},
			},
		},
		Terminal: true,
	}

	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("failed to marshal route: %w", err)
	}

	fmt.Printf("[DEBUG] Prepending route for %s to Caddy\n", deploymentID)

	// Prepend to srv0 routes array
	url := "http://caddy:2019/config/apps/http/servers/srv0/routes/0"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("caddy admin api unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caddy rejected config (%d): %s", resp.StatusCode, string(errBody))
	}

	return nil
}

func (p *Pipeline) RemoveRoute(deploymentID string) error {
	routeID := fmt.Sprintf("deployment-%s", deploymentID)
	url := fmt.Sprintf("http://caddy:2019/id/%s", routeID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
