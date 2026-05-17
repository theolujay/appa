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

type Router struct {
	client  *http.Client
	baseURL string
}

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
		Terminal: true,
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

func (r *Router) RestoreRoutes(dm *data.DeploymentModel) error {
	filters := data.Filters{
		Page:         1,
		PageSize:     1_000_000,
		Sort:         "id",
		SortSafelist: []string{"id"},
	}

	deployments, _, err := dm.GetAll(0, data.RUNNING, filters)
	if err != nil {
		return fmt.Errorf("failed to list deployments for sync: %w", err)
	}

	fmt.Println("Syncing active deployment routes with Caddy...")
	if len(deployments) < 1 {
		fmt.Println("No active deployments found. Skipping...")
	} else {
		for _, d := range deployments {
			if d.Status == data.RUNNING && d.Address != nil {
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
