package pipeline

import "errors"

var (
	errPrepareFailed     = errors.New("prepare failed")
	errBuildFailed       = errors.New("build failed")
	errDeployFailed      = errors.New("deploy failed")
	errRoutingFailed     = errors.New("routing failed")
	errContainerFailed   = errors.New("container failed")
	errContainerNotReady = errors.New("container did not become healthy in time")
)
