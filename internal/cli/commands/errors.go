package commands

import "errors"

var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrNoSSHTarget     = errors.New("no SSH target set")
	ErrInvalidTarget   = errors.New("target must be in format user@host or user@host:port")
	ErrProfileExists   = errors.New("profile already exists")
)
