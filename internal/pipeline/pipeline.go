// Package pipeline orchestrates the end-to-end deployment lifecycle:
// source-code preparation, container image building, container startup,
// and reverse-proxy route registration. It streams each phase's logs
// to the WebSocket hub and persists them to the database. Tasks can be
// cancelled via context cancellation, which triggers cleanup.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/moby/moby/client"
	"github.com/theolujay/appa/internal/data"
	"github.com/theolujay/appa/internal/hub"
)

const (
	phaseBuild   = "build"
	phaseDeploy  = "deploy"
	phaseRouting = "routing"
	phaseCancel  = "cancel"
)

// Pipeline manages the deployment workflow for applications, including
// code preparation, containerization, and traffic routing.
type Pipeline struct {
	deployment   data.DeploymentModeler
	hub          *hub.Hub
	router       *Router
	mu           sync.Mutex
	activeTasks  map[int64]context.CancelFunc
	dockerClient *client.Client
}

// New creates a new Pipeline with the necessary models and WebSocket hub.
func New(dm data.DeploymentModeler, h *hub.Hub, r *Router) *Pipeline {
	c, err := client.New(client.FromEnv)
	if err != nil {
		panic(fmt.Errorf("failed to initialize docker client: %w", err))
	}
	return &Pipeline{
		deployment:   dm,
		hub:          h,
		router:       r,
		activeTasks:  make(map[int64]context.CancelFunc),
		dockerClient: c,
	}
}

// Run performs the end-to-end deployment lifecycle for a deployment record.
func (p *Pipeline) Run(d *data.Deployment) {
	var buildDir string
	var imageTag string
	var address string
	var phase = "preparation"
	var err error

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

	buildDir, err = p.Prepare(ctx, d.ID, d.Source)
	if buildDir != "" {
		defer os.RemoveAll(buildDir)
	}

	if err == nil {
		phase = phaseBuild
		imageTag, err = p.Build(ctx, d.ID, buildDir)
	}

	if err == nil {
		phase = phaseDeploy
		address, err = p.StartContainer(ctx, d.ID, imageTag)
	}

	if err == nil {
		phase = phaseRouting
		err = p.router.AddRoute(d.ID, address)
	}

	if err != nil {
		status := data.FAILED
		if errors.Is(ctx.Err(), context.Canceled) {
			status = data.CANCELED
		}

		// TODO: handle database update error returned
		p.deployment.UpdateAndGet(d.ID, data.DeploymentUpdate{Status: &status})

		msg := fmt.Sprintf("%s failed: %v", phase, err)
		logID, _ := p.deployment.AppendLog(d.ID, phase, msg)
		p.hub.PublishLog(d.ID, hub.LogMessage{ID: logID, Line: msg})
		p.hub.PublishStatus(d.ID, status, "")
		return
	}

	url := fmt.Sprintf("http://%d.localhost", d.ID)

	status := data.RUNNING
	// TODO: handle this error
	p.deployment.UpdateAndGet(d.ID, data.DeploymentUpdate{
		Status:  &status,
		URL:     &url,
		Address: &address,
	})

	msg := fmt.Sprintf("deployment live at %s", url)
	logID, _ := p.deployment.AppendLog(d.ID, phaseDeploy, msg)
	p.hub.PublishLog(d.ID, hub.LogMessage{ID: logID, Line: msg})
	p.hub.PublishStatus(d.ID, status, url)
}

// Cancel stops a deployment by either cancelling the active context
// or stopping the associated container if it's already running.
func (p *Pipeline) Cancel(deploymentID int64) error {
	p.mu.Lock()
	cancel, ok := p.activeTasks[deploymentID]
	p.mu.Unlock()

	if !ok {
		return p.StopContainer(deploymentID)
	}
	cancel()

	msg := "cancellation requested"
	logID, _ := p.deployment.AppendLog(deploymentID, phaseCancel, msg)
	p.hub.PublishLog(deploymentID, hub.LogMessage{ID: logID, Line: msg})

	return nil
}
