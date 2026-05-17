package pipeline

import (
	"context"
	"fmt"
	"sync"

	"github.com/theolujay/appa/internal/data"
	"github.com/theolujay/appa/internal/hub"
)

type Pipeline struct {
	deployment  *data.DeploymentModel
	hub         *hub.Hub
	router      *Router
	mu          sync.Mutex
	activeTasks map[int64]context.CancelFunc
}

func New(dm *data.DeploymentModel, h *hub.Hub, r *Router) *Pipeline {
	return &Pipeline{
		deployment:  dm,
		hub:         h,
		router:      r,
		activeTasks: make(map[int64]context.CancelFunc),
	}
}

func (p *Pipeline) Run(d *data.Deployment) {
	status := ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Register the task so it can be cancelled via the API
	p.mu.Lock()
	p.activeTasks[d.ID] = cancel
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.activeTasks, d.ID)
		p.mu.Unlock()
	}()

	imageTag, err := p.Build(ctx, d.ID, d.Source)
	if err != nil {
		status = data.FAILED
		if ctx.Err() == context.Canceled {
			status = data.CANCELED
		}
		p.deployment.Update(d.ID, data.DeploymentUpdate{Status: &status})
		msg := fmt.Sprintf("build failed: %v", err)
		logID, _ := p.deployment.AppendLog(d.ID, "build", msg)
		p.hub.PublishLog(d.ID, hub.LogMessage{ID: logID, Line: msg})
		p.hub.PublishStatus(d.ID, status, "")
		return
	}

	address, err := p.StartContainer(ctx, d.ID, imageTag)
	if err != nil {
		status = data.FAILED
		if ctx.Err() == context.Canceled {
			status = data.CANCELED
		}
		p.deployment.Update(d.ID, data.DeploymentUpdate{Status: &status})
		msg := fmt.Sprintf("deployment failed: %v", err)
		logID, _ := p.deployment.AppendLog(d.ID, "deploy", msg)
		p.hub.PublishLog(d.ID, hub.LogMessage{ID: logID, Line: msg})
		p.hub.PublishStatus(d.ID, status, "")
		return
	}

	if err := p.router.AddRoute(d.ID, address); err != nil {
		status = data.FAILED
		p.deployment.Update(d.ID, data.DeploymentUpdate{Status: &status})
		msg := fmt.Sprintf("routing failed: %v", err)
		logID, _ := p.deployment.AppendLog(d.ID, "deploy", msg)
		p.hub.PublishLog(d.ID, hub.LogMessage{ID: logID, Line: msg})
		p.hub.PublishStatus(d.ID, status, "")
		return
	}

	url := fmt.Sprintf("http://%d.localhost", d.ID)

	status = data.RUNNING
	p.deployment.Update(d.ID, data.DeploymentUpdate{
		Status:  &status,
		URL:     &url,
		Address: &address,
	})

	msg := fmt.Sprintf("deployment live at %s", url)
	logID, _ := p.deployment.AppendLog(d.ID, "deploy", msg)
	p.hub.PublishLog(d.ID, hub.LogMessage{ID: logID, Line: msg})
	p.hub.PublishStatus(d.ID, status, url)
}

func (p *Pipeline) Cancel(deploymentID int64) error {
	p.mu.Lock()
	cancel, ok := p.activeTasks[deploymentID]
	p.mu.Unlock()

	if !ok {
		return p.StopContainer(deploymentID)
	}
	cancel()

	msg := "cancellation requested"
	logID, _ := p.deployment.AppendLog(deploymentID, "system", msg)
	p.hub.PublishLog(deploymentID, hub.LogMessage{ID: logID, Line: msg})

	return nil
}

func truncStr(id int64) string {
	s := fmt.Sprintf("%d", id)
	if len(s) < 8 {
		return s
	}
	return s[:8]
}
