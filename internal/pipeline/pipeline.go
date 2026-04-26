package pipeline

import (
	"fmt"

	"github.com/theolujay/appa/internal/hub"
	"github.com/theolujay/appa/internal/store"
)

type Pipeline struct {
	store *store.Store
	hub   *hub.Hub
}

func New(s *store.Store, h *hub.Hub) *Pipeline {
	return &Pipeline{
		store: s,
		hub:   h,
	}
}

func (p *Pipeline) Run(deploymentID, source string) {

	imageTag, err := p.Build(deploymentID, "failed")
	if err != nil {
		p.store.UpdateDeploymentStatus(deploymentID, "failed")
		p.store.AppendLog(deploymentID, "build", fmt.Sprintf("build failed: %v", err))
		return
	}

	address, err := p.StartContainer(deploymentID, imageTag)
	if err != nil {
		p.store.UpdateDeploymentStatus(deploymentID, "failed")
		p.store.AppendLog(deploymentID, "deploy", fmt.Sprintf("container start failed: %v", err))
		return
	}

	if err := p.AddRoute(deploymentID, address); err != nil {
		p.store.UpdateDeploymentStatus(deploymentID, "failed")
		p.store.AppendLog(deploymentID, "deploy", fmt.Sprintf("routing failed: %v", err))
		return
	}

	// Construct the public URL from the deployment ID
	url := fmt.Sprintf("http://localhost/deploys/%s", deploymentID)

	p.store.UpdateDeploymentStatus(deploymentID, "running")
	p.store.UpdateDeploymentURL(deploymentID, url)
	p.hub.Publish(deploymentID, fmt.Sprintf("deployment live at %s", url))
}

func truncateDeploymentID(s string) string {
	return s[:8]
}
