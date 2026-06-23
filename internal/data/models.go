// Package data provides the database access layer for the application.
// It defines models for deployments, users, and tokens, along with
// CRUD operations, validation helpers, and pagination/filtering support.
// All queries use context-based timeouts to prevent resource leaks.
package data

import (
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

var (
	ErrDuplicateEmail   = errors.New("duplicate email")
	ErrDuplicateProject = errors.New("duplicate project")
	ErrEditConflict     = errors.New("edit conflict")
	ErrRecordNotFound   = errors.New("record not found")
)

const (
	pqUniqueViolation = "23505"
	pqUsersEmailKey   = "users_email_key"
	pqProjectsNameKey = "projects_name_key"
)

type Models struct {
	Deployments DeploymentModeler
	Users       UserModeler
	Tokens      TokenModeler
	Projects    ProjectModeler
}

func NewModels(db *sql.DB) Models {
	return Models{
		Deployments: &DeploymentModel{DB: db},
		Users:       &UserModel{DB: db},
		Tokens:      &TokenModel{DB: db},
		Projects:    &ProjectModel{DB: db},
	}
}

func isUniqueViolation(err error, constraint string) bool {
	pqErr := &pq.Error{}
	if errors.As(err, &pqErr) {
		return pqErr.Code == pqUniqueViolation && pqErr.Constraint == constraint
	}
	return false
}
