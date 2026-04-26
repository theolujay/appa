package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// Mirros the JSON structure Caddy expects when a route is added
// via its Admin API.
type caddyRoute struct {
	ID     string        `json:"@id"`
	Match  []caddyMatch  `json:"match"`
	Handle []caddyHandle `json:"handle"`
}

type caddyMatch struct {
	Path []string `json:"path"`
}

type caddyHandle struct {
	Handler   string          `json:"handler"`
	Upstreams []caddyUpstream `json:"upstreams"`
}

type caddyUpstream struct {
	// Dial is the host:port address of the upstream container
	Dial string `json:"dial"`
}

// AddRoute registers a new reerse proxy route in Caddy's live configuration,
// routing traffic from /deploys/<deploymentID>/* to the container at address.
// This takes effect immediately... no Caddy restart required.
func (p *Pipeline) AddRoute(deploymentID, address string) error {
	truncDeployID := truncateDeploymentID(deploymentID)
	route := caddyRoute{
		ID: fmt.Sprintf("deployment-%s", truncDeployID),
		Match: []caddyMatch{
			{Path: []string{fmt.Sprintf("deploys/%s/*", truncDeployID)}},
		},
		Handle: []caddyHandle{
			{
				Handler: "reverse_proxy",
				Upstreams: []caddyUpstream{
					{Dial: address},
				},
			},
		},
	}

	// Serialize the route struct to JSON. This will be sent to Caddy
	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("failed to marshal caddy route: %w", err)
	}

	// POST to Caddy's Admin API to append the new route to the route list.
	// The URL assumes Caddy's Admin API is reachable at caddy:2019
	// TODO: set Caddy's port as env var
	resp, err := http.Post(
		"http://caddy:2019/config/apps/http/servers/srv0/routes",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to reach caddy admin api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caddy admin api returned unexpected status: %d", resp.StatusCode)
	}

	return nil
}
