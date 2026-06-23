package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	da "github.com/theolujay/appa/internal/data"
)

func (app *application) createDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	var p struct {
		Source      string `json:"source"`
		EnvVars     string `json:"env_vars"`
		ProjectName string `json:"project_name"`
		ProjectID   *int64 `json:"project_id,omitempty"`
	}
	err := app.readJSON(w, r, &p)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var project *da.Project

	if p.ProjectName != "" {
		project, err = app.models.Projects.GetByName(p.ProjectName)
		if errors.Is(err, da.ErrRecordNotFound) {
			project = &da.Project{
				Name:   p.ProjectName,
				UserID: &user.ID,
			}
			if user.IsAnonymous() {
				project.UserID = nil
			}
			if err = app.models.Projects.Insert(project); err != nil {
				app.serverErrorResponse(w, r, err)
				return
			}
		} else if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}
	} else if p.ProjectID != nil {
		project, err = app.models.Projects.Get(*p.ProjectID)
		if err != nil {
			switch {
			case errors.Is(err, da.ErrRecordNotFound):
				app.notFoundResponse(w, r)
			default:
				app.serverErrorResponse(w, r, err)
			}
			return
		}
	}

	d := da.Deployment{
		Source:  p.Source,
		EnvVars: &p.EnvVars,
	}
	if project != nil {
		d.ProjectID = &project.ID
		d.EnvVars = app.mergeProjectEnvVars(project.ID, d.EnvVars)
	}

	if !user.IsAnonymous() {
		d.UserID = &user.ID
	}
	if err = da.ValidateDeployment(d); err != nil {
		app.failedValidationResponse(w, r, err)
		return
	}

	if err := app.models.Deployments.Create(&d); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.background(func() {
		app.pipeline.Run(&d)
	})

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/deployments/%d/logs", d.ID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"deployment": d}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) uploadProjectHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
		app.badRequestResponse(w, r, err)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		switch {
		case errors.Is(err, http.ErrMissingFile):
			app.badRequestResponse(w, r, err)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}
	defer file.Close()

	envVars := r.FormValue("env_vars")
	projectName := r.FormValue("project_name")

	dir := uuid.New().String()
	uploadDir := filepath.Join("/tmp", "appa-upload", dir)
	if err = os.MkdirAll(uploadDir, 0755); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	if err = unzip(file, header.Size, uploadDir); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	d := da.Deployment{
		Source:  "uploaded-project",
		EnvVars: &envVars,
	}

	if !user.IsAnonymous() {
		d.UserID = &user.ID
	}

	if projectName != "" {
		project, err := app.models.Projects.GetByName(projectName)
		if errors.Is(err, da.ErrRecordNotFound) {
			project = &da.Project{
				Name:   projectName,
				UserID: &user.ID,
			}
			if user.IsAnonymous() {
				project.UserID = nil
			}
			if err = app.models.Projects.Insert(project); err != nil {
				app.serverErrorResponse(w, r, err)
				return
			}
		} else if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}
		d.ProjectID = &project.ID
		d.EnvVars = app.mergeProjectEnvVars(project.ID, d.EnvVars)
	}

	if err = app.models.Deployments.Create(&d); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.background(func() {
		d.Source = uploadDir
		app.pipeline.Run(&d)
	})

	err = app.writeJSON(w, http.StatusAccepted, envelope{"deployment": d}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func unzip(r io.ReaderAt, size int64, dest string) error {
	reader, err := zip.NewReader(r, size)
	if err != nil {
		return err
	}

	for _, f := range reader.File {
		fpath := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

func (app *application) stopDeploymentHandler(w http.ResponseWriter, r *http.Request) {
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

	d, err := app.models.Deployments.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, da.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if d.UserID != nil && *d.UserID != user.ID {
		app.notPermittedResponse(w, r)
		return
	}

	app.background(func() {
		if err := app.pipeline.Stop(d.ID); err != nil {
			app.logger.Error("stop deployment failed", "error", err)
		}
	})

	err = app.writeJSON(w, http.StatusAccepted, envelope{"message": "stopping deployment"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) restartDeploymentHandler(w http.ResponseWriter, r *http.Request) {
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

	d, err := app.models.Deployments.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, da.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if d.UserID != nil && *d.UserID != user.ID {
		app.notPermittedResponse(w, r)
		return
	}

	app.background(func() {
		if err := app.pipeline.Restart(d.ID); err != nil {
			app.logger.Error("restart deployment failed", "error", err)
		}
	})

	err = app.writeJSON(w, http.StatusAccepted, envelope{"message": "restarting deployment"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) getDeploymentHandler(w http.ResponseWriter, r *http.Request) {
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

	d, err := app.models.Deployments.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, da.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if d.UserID != nil && *d.UserID != user.ID {
		app.notPermittedResponse(w, r)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"deployment": d}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) mergeProjectEnvVars(projectID int64, deploymentEnvVars *string) *string {
	projectEnvVars, err := app.models.ProjectEnvVars.GetAll(projectID)
	if err != nil {
		app.logger.Error("failed to fetch project env vars for merge", "project_id", projectID, "error", err)
		return deploymentEnvVars
	}
	if len(projectEnvVars) == 0 {
		return deploymentEnvVars
	}
	return da.MergeProjectEnvVars(projectEnvVars, deploymentEnvVars)
}

func (app *application) listDeploymentsHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	var q struct {
		Status      string
		ProjectName string
		ProjectID   int64
		da.Filters
	}

	qs := r.URL.Query()

	q.Status = app.readString(qs, "status", "")
	q.ProjectName = app.readString(qs, "project_name", "")
	pid, _ := app.readInt(qs, "project_id", 0)
	q.ProjectID = int64(pid)

	if q.ProjectName != "" && q.ProjectID == 0 {
		p, err := app.models.Projects.GetByName(q.ProjectName)
		if err != nil {
			switch {
			case errors.Is(err, da.ErrRecordNotFound):
				app.notFoundResponse(w, r)
			default:
				app.serverErrorResponse(w, r, err)
			}
			return
		}
		q.ProjectID = p.ID
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
	q.Filters.SortSafelist = []string{"id", "status", "-id", "-status"}

	err = da.ValidateFilters(q.Filters)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		err = errors.Join(errs...)
		app.failedValidationResponse(w, r, err)
		return
	}

	d, m, err := app.models.Deployments.GetAllForUser(user.ID, q.Status, q.ProjectID, q.Filters)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"deployments": d, "metadata": m}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}
