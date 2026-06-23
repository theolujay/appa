package main

import (
	"errors"
	"fmt"
	"net/http"

	da "github.com/theolujay/appa/internal/data"
)

func (app *application) createProjectHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	var p struct {
		Name string `json:"name"`
	}
	err := app.readJSON(w, r, &p)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	project := &da.Project{
		Name:   p.Name,
		UserID: &user.ID,
	}

	if user.IsAnonymous() {
		project.UserID = nil
	}

	if err := app.models.Projects.Insert(project); err != nil {
		switch {
		case errors.Is(err, da.ErrDuplicateProject):
			app.failedValidationResponse(w, r, fmt.Errorf("name: a project with this name already exists"))
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/projects/%d", project.ID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"project": project}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) listProjectsHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	qs := r.URL.Query()
	name := app.readString(qs, "name", "")

	// If a name is provided, look up by name directly
	if name != "" {
		project, err := app.models.Projects.GetByName(name)
		if err != nil {
			switch {
			case errors.Is(err, da.ErrRecordNotFound):
				app.notFoundResponse(w, r)
			default:
				app.serverErrorResponse(w, r, err)
			}
			return
		}
		err = app.writeJSON(w, http.StatusOK, envelope{"projects": []da.Project{*project}}, nil)
		if err != nil {
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	var q struct {
		da.Filters
	}

	var errs []error
	page, err := app.readInt(qs, "page", 1)
	if err != nil {
		errs = append(errs, err)
	}
	pageSize, err := app.readInt(qs, "page_size", 20)
	if err != nil {
		errs = append(errs, err)
	}

	q.Filters.Page = page
	q.Filters.PageSize = pageSize
	q.Filters.Sort = app.readString(qs, "sort", "id")
	q.Filters.SortSafelist = []string{"id", "name", "created_at", "-id", "-name", "-created_at"}

	err = da.ValidateFilters(q.Filters)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		err = errors.Join(errs...)
		app.failedValidationResponse(w, r, err)
		return
	}

	userID := int64(0)
	if !user.IsAnonymous() {
		userID = user.ID
	}

	projects, metadata, err := app.models.Projects.GetAllForUser(userID, q.Filters)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"projects": projects, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) getProjectHandler(w http.ResponseWriter, r *http.Request) {
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

	err = app.writeJSON(w, http.StatusOK, envelope{"project": project}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateProjectHandler(w http.ResponseWriter, r *http.Request) {
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
		Name *string `json:"name"`
	}
	err = app.readJSON(w, r, &p)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if p.Name != nil {
		project.Name = *p.Name
	}

	if err := app.models.Projects.Update(project); err != nil {
		switch {
		case errors.Is(err, da.ErrEditConflict):
			app.editConflictResponse(w, r)
		case errors.Is(err, da.ErrDuplicateProject):
			app.failedValidationResponse(w, r, fmt.Errorf("name: a project with this name already exists"))
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"project": project}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteProjectHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := app.models.Projects.Delete(id); err != nil {
		switch {
		case errors.Is(err, da.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "project deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
