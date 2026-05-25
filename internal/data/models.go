// Package data provides the database access layer for the application.
// It defines models for deployments, users, and tokens, along with
// CRUD operations, validation helpers, and pagination/filtering support.
// All queries use context-based timeouts to prevent resource leaks.
package data

import (
	"database/sql"
	"errors"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
)

type Models struct {
	Deployments DeploymentModel
	Users       UserModel
	Tokens      TokenModel
}

func NewModels(db *sql.DB) Models {
	return Models{
		Deployments: DeploymentModel{DB: db},
		Users:       UserModel{DB: db},
		Tokens:      TokenModel{DB: db},
	}
}
