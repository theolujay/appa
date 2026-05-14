package main

import (
	"expvar"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
)

func (app *application) routes() http.Handler {
	router := httprouter.New()
	router.NotFound = http.HandlerFunc(app.notFoundResponse)
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)

	router.HandlerFunc(http.MethodGet, "/v1/deployments", app.ListDeployments)
	router.HandlerFunc(http.MethodPost, "/v1/deployments", app.CreateDeployment)
	router.HandlerFunc(http.MethodPost, "/v1/deployments/upload", app.UploadProject)
	router.HandlerFunc(http.MethodPatch, "/v1/deployments/:id", app.CancelDeployment)
	router.HandlerFunc(http.MethodGet, "/v1/deployments/:id/logs", app.StreamLogs)

	router.Handler(http.MethodGet, "/debug/vars", expvar.Handler())

	standard := alice.New(app.metrics, app.recoverPanic, app.enableCORS, app.logRequest, app.rateLimit)

	return standard.Then(router)
}

func (app *application) healthcheckHandler(w http.ResponseWriter, r *http.Request) {

	env := envelope{
		"status": "available",
		"system_info": map[string]string{
			"environment": app.config.env,
			"version":     version,
		},
	}

	err := app.writeJSON(w, http.StatusOK, env, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
