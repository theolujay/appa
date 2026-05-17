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
	Users		UserModel
	Tokens	    TokenModel
}

func NewModels(db *sql.DB) Models {
	return Models{
		Deployments: DeploymentModel{DB: db},
		Users:		 UserModel{DB: db},
		Tokens:		 TokenModel{DB: db},
	}
}
