package pipeline

import (
	"context"
	"fmt"
	"sync"

	"github.com/theolujay/appa/internal/hub"
	"github.com/theolujay/appa/internal/store"
)

type Pipeline struct {
	store       *store.Store
	hub         *hub.Hub
	mu          sync.Mutex
	activeTasks map[string]context.CancelFunc
}

func New(s *store.Store, h *hub.Hub) *Pipeline {
	return &Pipeline{
		store:       s,
		hub:         h,
		activeTasks: make(map[string]context.CancelFunc),
	}
}

func (p *Pipeline) Run(deploymentID, source string) {
	// Create a cancellable context for this deployment run.
	status := ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Register the task so it can be cancelled via the API
	p.mu.Lock()
	p.activeTasks[deploymentID] = cancel
	p.mu.Unlock()
	// Ensure task can be unregistered when the pipeline finishes (success or failure)
	defer func() {
		p.mu.Lock()
		delete(p.activeTasks, deploymentID)
		p.mu.Unlock()
	}()

	imageTag, err := p.Build(ctx, deploymentID, source)
	if err != nil {
		status = store.FAILED
		if ctx.Err() == context.Canceled {
			status = store.CANCELED
		}
		p.store.UpdateDeployment(deploymentID, store.DeploymentUpdate{Status: &status})
		msg := fmt.Sprintf("build failed: %v", err)
		id, _ := p.store.AppendLog(deploymentID, "build", msg)
		p.hub.PublishLog(deploymentID, hub.LogMessage{ID: id, Line: msg})
		p.hub.PublishStatus(deploymentID, status, "")
		return
	}

	address, err := p.StartContainer(ctx, deploymentID, imageTag)
	if err != nil {
		status = store.FAILED
		if ctx.Err() == context.Canceled {
			status = store.CANCELED
		}
		p.store.UpdateDeployment(deploymentID, store.DeploymentUpdate{Status: &status})
		msg := fmt.Sprintf("deployment failed: %v", err)
		id, _ := p.store.AppendLog(deploymentID, "deploy", msg)
		p.hub.PublishLog(deploymentID, hub.LogMessage{ID: id, Line: msg})
		p.hub.PublishStatus(deploymentID, status, "")
		return
	}

	if err := p.AddRoute(deploymentID, address); err != nil {
		status = store.FAILED
		p.store.UpdateDeployment(deploymentID, store.DeploymentUpdate{Status: &status})
		msg := fmt.Sprintf("routing failed: %v", err)
		id, _ := p.store.AppendLog(deploymentID, "deploy", msg)
		p.hub.PublishLog(deploymentID, hub.LogMessage{ID: id, Line: msg})
		p.hub.PublishStatus(deploymentID, status, "")
		return
	}

	// Construct the public URL from the deployment ID using subdomain format.
	url := fmt.Sprintf("http://localhost/deploys/%s", deploymentID)

	status = store.RUNNING
	p.store.UpdateDeployment(deploymentID, store.DeploymentUpdate{
		Status:  &status,
		URL:     &url,
		Address: &address,
	})

	msg := fmt.Sprintf("deployment live at %s", url)
	id, _ := p.store.AppendLog(deploymentID, "deploy", msg)
	p.hub.PublishLog(deploymentID, hub.LogMessage{ID: id, Line: msg})
	p.hub.PublishStatus(deploymentID, status, url)
}

func (p *Pipeline) SyncRoutes() error {
	deployments, err := p.store.ListDeployments()
	if err != nil {
		return fmt.Errorf("failed to list deployments for sync: %w", err)
	}

	fmt.Println("Syncing active deployment routes with Caddy...")
	for _, d := range deployments {
		if d.Status == store.RUNNING && d.Address != nil {
			fmt.Printf("Restoring route for %s -> %s\n", d.ID, *d.Address)
			if err := p.AddRoute(d.ID, *d.Address); err != nil {
				fmt.Printf("failed to restore route for %s: %v\n", d.ID, err)
			}
		}
	}
	return nil
}

func (p *Pipeline) Cancel(deploymentID string) error {
	p.mu.Lock()
	cancel, ok := p.activeTasks[deploymentID]
	p.mu.Unlock()

	if !ok {
		// If it's not in activeTasks, it might be already finished or running.
		// If it's running, stop the container.
		return p.StopContainer(deploymentID)
	}
	// Trigger cancellation. This will cause p.Build or p.StartContainer to return an error.
	cancel()

	msg := "cancellation requested"
	id, _ := p.store.AppendLog(deploymentID, "system", msg)
	p.hub.PublishLog(deploymentID, hub.LogMessage{ID: id, Line: msg})

	return nil
}

func truncStr(s string) string {
	if len(s) < 8 {
		return s
	}
	return s[:8]
}
