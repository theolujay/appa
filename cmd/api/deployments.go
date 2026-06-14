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

	var input struct {
		Source  string `json:"source"`
		EnvVars string `json:"env_vars"`
	}
	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	deployment := &data.Deployment{
		Source:  input.Source,
		EnvVars: &input.EnvVars,
	}

	if !user.IsAnonymous() {
		deployment.UserID = &user.ID
	}

	if errs := data.ValidateDeployment(deployment); len(errs) > 0 {
		app.failedValidationResponse(w, r, errs)
		return
	}

	if err := app.models.Deployments.Create(deployment); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.background(func() {
		app.pipeline.Run(deployment)
	})

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/deployments/%d/logs", deployment.ID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"deployment": deployment}, headers)
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

	deployment := &data.Deployment{
		Source:  "uploaded-project",
		EnvVars: &envVars,
	}

	if !user.IsAnonymous() {
		deployment.UserID = &user.ID
	}

	if err = app.models.Deployments.Create(deployment); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.background(func() {
		deployment.Source = uploadDir
		app.pipeline.Run(deployment)
	})

	err = app.writeJSON(w, http.StatusAccepted, envelope{"deployment": deployment}, nil)
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

	deploymentID, err := app.readIDParam(r)

	if err != nil || deploymentID < 1 {
		app.notFoundResponse(w, r)
		return
	}

	deployment, err := app.models.Deployments.Get(deploymentID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if deployment.UserID != nil && *deployment.UserID != user.ID {
		app.notPermittedResponse(w, r)
		return
	}

	if err := app.pipeline.Cancel(deploymentID); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

}

func (app *application) listDeploymentsHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	var input struct {
		Status string
		data.Filters
	}

	qs := r.URL.Query()

	input.Status = app.readString(qs, "status", "")

	errs := vd.Error{}
	page, vdErr := app.readInt(qs, "page", 1)
	if vdErr != nil {
		maps.Copy(errs, vdErr)
	}
	pageSize, vdErr := app.readInt(qs, "page_size", 20)
	if vdErr != nil {
		maps.Copy(errs, vdErr)
	}

	input.Filters.Page = page
	input.Filters.PageSize = pageSize
	input.Filters.Sort = app.readString(qs, "sort", "id")
	input.Filters.SortSafelist = []string{"id", "status"}

	vdErr = data.ValidateFilters(input.Filters)
	if vdErr != nil {
		maps.Copy(errs, vdErr)
	}

	if len(errs) > 0 {
		app.failedValidationResponse(w, r, errs)
		return
	}

	deployments, metadata, err := app.models.Deployments.GetAllForUser(user.ID, input.Status, input.Filters)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"deployments": deployments, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}
