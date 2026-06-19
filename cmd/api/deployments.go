package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/theolujay/appa/internal/data"
	vd "github.com/theolujay/appa/internal/validator"
)

func (app *application) createDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	var p struct {
		Source  string `json:"source"`
		EnvVars string `json:"env_vars"`
	}
	err := app.readJSON(w, r, &p)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	d := data.Deployment{
		Source:  p.Source,
		EnvVars: &p.EnvVars,
	}

	if !user.IsAnonymous() {
		d.UserID = &user.ID
	}

	if errs := data.ValidateDeployment(&d); len(errs) > 0 {
		app.failedValidationResponse(w, r, errs)
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

	d := data.Deployment{
		Source:  "uploaded-project",
		EnvVars: &envVars,
	}

	if !user.IsAnonymous() {
		d.UserID = &user.ID
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

func (app *application) cancelDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	id, err := app.readIDParam(r)

	if err != nil || id < 1 {
		app.notFoundResponse(w, r)
		return
	}

	d, err := app.models.Deployments.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
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

	if err := app.pipeline.Cancel(d.ID); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

}

func (app *application) listDeploymentsHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	var q struct {
		Status string
		data.Filters
	}

	qs := r.URL.Query()

	q.Status = app.readString(qs, "status", "")

	errs := vd.Error{}
	page, vdErr := app.readInt(qs, "page", 1)
	if vdErr != nil {
		maps.Copy(errs, vdErr)
	}
	pageSize, vdErr := app.readInt(qs, "page_size", 20)
	if vdErr != nil {
		maps.Copy(errs, vdErr)
	}

	q.Filters.Page = page
	q.Filters.PageSize = pageSize
	q.Filters.Sort = app.readString(qs, "sort", "id")
	q.Filters.SortSafelist = []string{"id", "status"}

	vdErr = data.ValidateFilters(q.Filters)
	if vdErr != nil {
		maps.Copy(errs, vdErr)
	}

	if len(errs) > 0 {
		app.failedValidationResponse(w, r, errs)
		return
	}

	d, m, err := app.models.Deployments.GetAllForUser(user.ID, q.Status, q.Filters)
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
