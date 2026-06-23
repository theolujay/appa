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

	router.HandlerFunc(http.MethodPost, "/v1/users", app.registerUserHandler)
	router.HandlerFunc(http.MethodPut, "/v1/users/activated", app.activateUserHandler)
	router.HandlerFunc(http.MethodPost, "/v1/tokens/authentication", app.createAuthenticationTokenHandler)

	router.HandlerFunc(http.MethodGet, "/v1/deployments", app.listDeploymentsHandler)
	router.HandlerFunc(http.MethodPost, "/v1/deployments", app.createDeploymentHandler)
	router.HandlerFunc(http.MethodPost, "/v1/deployments/upload", app.uploadProjectHandler)
	router.HandlerFunc(http.MethodGet, "/v1/deployments/:id", app.getDeploymentHandler)
	router.HandlerFunc(http.MethodPut, "/v1/deployments/:id/stop", app.stopDeploymentHandler)
	router.HandlerFunc(http.MethodPut, "/v1/deployments/:id/restart", app.restartDeploymentHandler)
	router.HandlerFunc(http.MethodGet, "/v1/deployments/:id/logs", app.streamLogsHandler)

	router.Handler(http.MethodGet, "/debug/vars", expvar.Handler())

	standard := alice.New(
		app.metrics,
		app.recoverPanic,
		app.enableCORS,
		app.secureHeaders,
		app.logRequest,
		app.rateLimit,
		app.authenticate,
	)

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
