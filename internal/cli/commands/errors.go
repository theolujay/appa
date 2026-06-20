package commands

import "errors"

var (
	errProfileNotFound = errors.New("profile not found")
	errNoSSHTarget     = errors.New("no SSH target set")
	errInvalidTarget   = errors.New("target must be in format user@host or user@host:port")
	errProfileExists   = errors.New("profile already exists")
)
