package main

import (
	"errors"
	"fmt"
	"net/http"

	da "github.com/theolujay/appa/internal/data"
	"github.com/julienschmidt/httprouter"
)

func (app *application) listProjectEnvVarsHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	id, err := app.readIDParam(r)
	if err != nil || id < 1 {
		switch {
		case errors.Is(err, ErrParamInvalid):
			err = fmt.Errorf("%w: ID", err)
			app.badRequestResponse(w, r, err)
		default:
			app.notFoundResponse(w, r)
		}
		return
	}

	project, err := app.models.Projects.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, da.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if project.UserID != nil && !user.IsAnonymous() && *project.UserID != user.ID {
		app.notPermittedResponse(w, r)
		return
	}

	envs, err := app.models.ProjectEnvVars.GetAll(id)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"env_vars": envs}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) setProjectEnvVarsHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	id, err := app.readIDParam(r)
	if err != nil || id < 1 {
		switch {
		case errors.Is(err, ErrParamInvalid):
			err = fmt.Errorf("%w: ID", err)
			app.badRequestResponse(w, r, err)
		default:
			app.notFoundResponse(w, r)
		}
		return
	}

	project, err := app.models.Projects.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, da.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if project.UserID != nil && !user.IsAnonymous() && *project.UserID != user.ID {
		app.notPermittedResponse(w, r)
		return
	}

	var p struct {
		EnvVars map[string]string `json:"env_vars"`
	}
	err = app.readJSON(w, r, &p)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.models.ProjectEnvVars.UpsertMany(id, p.EnvVars); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	envs, err := app.models.ProjectEnvVars.GetAll(id)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	err = app.writeJSON(w, http.StatusOK, envelope{"env_vars": envs}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteProjectEnvVarHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	id, err := app.readIDParam(r)
	if err != nil || id < 1 {
		switch {
		case errors.Is(err, ErrParamInvalid):
			err = fmt.Errorf("%w: ID", err)
			app.badRequestResponse(w, r, err)
		default:
			app.notFoundResponse(w, r)
		}
		return
	}

	params := httprouter.ParamsFromContext(r.Context())
	key := params.ByName("key")
	if key == "" {
		app.notFoundResponse(w, r)
		return
	}

	project, err := app.models.Projects.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, da.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if project.UserID != nil && !user.IsAnonymous() && *project.UserID != user.ID {
		app.notPermittedResponse(w, r)
		return
	}

	if err := app.models.ProjectEnvVars.Delete(id, key); err != nil {
		switch {
		case errors.Is(err, da.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "env var deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
