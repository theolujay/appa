package commands

import "errors"

var (
	errInvalidName    = errors.New("invalid name")
	errConfigNotFound = errors.New("config not found")
	errDuplicateConfig  = errors.New("config already exists")
	errNoSSHTarget    = errors.New("no SSH target set")
	errInvalidTarget  = errors.New("target must be in format user@host or user@host:port")
)
