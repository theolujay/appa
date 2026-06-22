package commands

import "errors"

var (
	errInvalidPath = errors.New("invalid path")
	errInvalidName    = errors.New("invalid name")
	errConfigNotFound = errors.New("config not found")
	errDuplicateConfig  = errors.New("config already exists")
	errNoSSHTarget    = errors.New("no SSH target set")
	errInvalidTarget  = errors.New("target must be in format user@host or user@host:port")
)
