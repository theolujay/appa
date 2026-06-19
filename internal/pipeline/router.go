package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/theolujay/appa/internal/data"
)

// Router manages HTTP routes in a Caddy instance for Appa deployments.
//
// It communicates with the Caddy admin API to add, remove and restore
// per-deployment reverse-proxy routes.
type Router struct {
	client  *http.Client
	baseURL string
}

// NewRouter constructs a Router that talks to the Caddy admin API at the given
// address. `caddyAddr` should be the host:port of the Caddy admin endpoint
// (for example, "localhost:2019").
func NewRouter(caddyAddr string) *Router {
	return &Router{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: fmt.Sprintf("http://%s", caddyAddr),
	}
}

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

// AddRoute creates a reverse-proxy route in Caddy for the deployment identified
// by `id`, proxying traffic to `address` (host:port). If a route for the same
// deployment already exists it will be removed first.
//
// The function returns an error if the Caddy admin API cannot be reached or
// if Caddy rejects the configuration.
func (r *Router) AddRoute(id int64, address string) error {
	routeID := fmt.Sprintf("deployment-%d", id)

	_ = r.RemoveRoute(id)

	route := caddyRoute{
		ID: routeID,
		Match: []caddyMatch{
			{
				Host: []string{fmt.Sprintf("%d.localhost", id)},
			},
		},
		Handle: []caddyHandle{
			{
				Handler:   "reverse_proxy",
				Upstreams: []caddyUpstream{{Dial: address}},
				Headers: &caddyHeaderOpsWrapper{
					Request: &caddyHeaderOps{
						Set: map[string][]string{
							"Host":              {"{http.request.host}"},
							"X-Forwarded-Host":  {"{http.request.host}"},
							"X-Forwarded-Proto": {"{http.request.scheme}"},
						},
					},
				},
			},
		},
		Terminal: false,
	}

	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("failed to marshal route: %w", err)
	}

	fmt.Printf("[DEBUG] Prepending route for %d to Caddy\n", id)

	url := fmt.Sprintf("%s/config/apps/http/servers/srv0/routes/0", r.baseURL)
	resp, err := r.client.Post(url, "application/json", bytes.NewReader(body))
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

// RemoveRoute deletes the route associated with the given deployment `id` from
// the Caddy instance. It performs an HTTP DELETE against the Caddy admin API
// and returns any error encountered while creating or sending the request.
func (r *Router) RemoveRoute(id int64) error {
	routeID := fmt.Sprintf("deployment-%d", id)
	url := fmt.Sprintf("%s/id/%s", r.baseURL, routeID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// RestoreRoutes synchronizes Caddy routes with currently running deployments
// known to the provided `data.DeploymentModeler`. It queries the model for
// active deployments and calls `AddRoute` for each running deployment that
// exposes an address. Errors during individual route restores are logged but
// do not abort the whole synchronization process.
func (r *Router) RestoreRoutes(dm data.DeploymentModeler) error {
	filters := data.Filters{
		Page:         1,
		PageSize:     1_000_000,
		Sort:         "id",
		SortSafelist: []string{"id"},
	}
	// A user ID of 0 is treated as a wildcard
	// and fetches deployments for all users.
	deployments, metadata, err := dm.GetAllForUser(0, RUNNING, filters)
	if err != nil {
		return fmt.Errorf("failed to list %d deployments for sync: %w", metadata.TotalRecords, err)
	}

	fmt.Println("Syncing active deployment routes with Caddy...")
	if len(deployments) < 1 {
		fmt.Println("No active deployments found. Skipping...")
	} else {
		for _, d := range deployments {
			if d.Status == RUNNING && d.Address != nil {
				fmt.Printf("Restoring route for %d -> %s\n", d.ID, *d.Address)
				err = r.AddRoute(d.ID, *d.Address)
				if err != nil {
					fmt.Printf("failed to restore route for %d: %v\n", d.ID, err)
				}
			}
		}
		fmt.Printf("Synced active deployments")
	}
	return nil
}
