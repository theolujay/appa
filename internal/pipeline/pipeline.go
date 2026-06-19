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
	"time"

	"github.com/moby/moby/client"
	"github.com/theolujay/appa/internal/data"
	"github.com/theolujay/appa/internal/hub"
)

const (
	phasePrepare = "prepare"
	phaseBuild   = "build"
	phaseDeploy  = "deploy"
	phaseRouting = "routing"
)

const (
	PENDING   string = "pending"
	BUILDING  string = "building"
	DEPLOYING string = "deploying"
	RUNNING   string = "running"
	CANCELED  string = "canceled"
	STOPPED   string = "stopped"
	FAILED    string = "failed"
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

// pipelineCtx carries the minimal state needed for pipeline logging and
// status updates: the cancellation-aware context, deployment ID, current
// phase and status, a DeploymentUpdate payload to persist, and any error
// encountered.
type pipelineCtx struct {
	ctx    context.Context
	ID     int64
	phase  string
	status string
	update *data.DeploymentUpdate
	err    error
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

// logLine persists a single log line to the database and publishes it to the
// WebSocket hub. It is the single low-level primitive for all one-off and
// streaming log messages throughout the pipeline.
func (p *Pipeline) logLine(id int64, phase, msg string) (int64, error) {
	logID, err := p.deployment.AppendLog(id, phase, msg)
	if err != nil {
		return 0, err
	}
	p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: msg})
	return logID, nil
}

// publishStatus persists the deployment status to the database, publishes a
// status change to the hub, and appends a human-readable log line via logLine.
func (p *Pipeline) publishStatus(dc pipelineCtx) {
	if errors.Is(dc.ctx.Err(), context.Canceled) {
		dc.status = CANCELED
	}

	dc.update.Status = &dc.status

	// TODO: handle database update error returned
	p.deployment.UpdateAndGet(dc.ID, *dc.update)

	var msg string
	switch dc.status {
	case PENDING:
		msg = "deployment queued"
	case BUILDING:
		msg = "build complete"
	case DEPLOYING:
		msg = "deployment started"
	case FAILED:
		msg = fmt.Sprintf("%s failed: %v", dc.phase, dc.err)
	case RUNNING:
		msg = fmt.Sprintf("deployment live at %s", *dc.update.URL)
	case CANCELED:
		msg = "cancellation requested"
		if dc.err != nil {
			if errors.Is(dc.ctx.Err(), context.DeadlineExceeded) {
				msg = "stopping container took too long"
			}
		}
	case STOPPED:
		msg = "deployment stopped by user"
	}

	p.logLine(dc.ID, dc.phase, msg)

	url := ""
	if dc.update.URL != nil {
		url = *dc.update.URL
	}
	p.hub.PublishStatus(dc.ID, dc.status, url)
}

// Run performs the end-to-end deployment lifecycle for a deployment record.
func (p *Pipeline) Run(d *data.Deployment) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pc := pipelineCtx{
		ctx:    ctx,
		ID:     d.ID,
		phase:  phasePrepare,
		status: PENDING,
		update: &data.DeploymentUpdate{},
	}

	// Register the task so it can be cancelled via the API
	p.mu.Lock()
	p.activeTasks[d.ID] = cancel
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.activeTasks, d.ID)
		p.mu.Unlock()
	}()

	p.publishStatus(pc)

	pc.status = BUILDING
	buildDir, err := p.Prepare(ctx, d.ID, d.Source)
	if err != nil {
		pc.err = err
		p.publishStatus(pc)
		return
	}
	// TODO: reconsider this for pause/resume deployments in the future
	defer os.RemoveAll(buildDir)

	p.publishStatus(pc)

	pc.phase = phaseBuild
	imageTag, err := p.Build(ctx, d.ID, buildDir)
	if err != nil {
		pc.err = err
		p.publishStatus(pc)
		return
	}
	pc.update.ImageTag = &imageTag

	p.publishStatus(pc)

	pc.phase = phaseDeploy
	pc.status = DEPLOYING
	addr, err := p.StartContainer(ctx, d.ID)
	if err != nil {
		pc.err = err
		p.publishStatus(pc)
		return
	}

	p.publishStatus(pc)

	pc.phase = phaseRouting
	err = p.router.AddRoute(d.ID, addr)
	if err != nil {
		pc.err = err
		p.publishStatus(pc)
		return
	}

	p.publishStatus(pc)

	status := RUNNING
	url := fmt.Sprintf("http://%d.localhost", d.ID)

	pc.status = status
	pc.update.URL = &url
	pc.update.Status = &status
	pc.update.Address = &addr

	p.publishStatus(pc)
}

// Cancel stops a deployment by either cancelling the active context
// or stopping the associated container if it's already running.
func (p *Pipeline) Cancel(ID int64) error {
	p.mu.Lock()
	cancel, ok := p.activeTasks[ID]
	if ok {
		cancel()
	}
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dc := pipelineCtx{
		ctx:    ctx,
		status: CANCELED,
		ID:     ID,
		update: &data.DeploymentUpdate{},
	}

	p.StopContainer(&dc)

	p.publishStatus(dc)

	return dc.err
}
