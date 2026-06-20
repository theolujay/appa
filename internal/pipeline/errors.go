package pipeline

import "errors"

var (
	ErrPrepareFailed     = errors.New("prepare failed")
	ErrBuildFailed       = errors.New("build failed")
	ErrDeployFailed      = errors.New("deploy failed")
	ErrRoutingFailed     = errors.New("routing failed")
	ErrContainerFailed   = errors.New("container failed")
	ErrContainerNotReady = errors.New("container did not become healthy in time")
)
