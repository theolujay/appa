package main

import (
	"fmt"
	"net/http"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("GET /deployments", app.ListDeployments)
	mux.HandleFunc("POST /deployments", app.CreateDeployment)
	mux.HandleFunc("POST /deployments/upload", app.UploadProject)
	mux.HandleFunc("PATCH /deployments/{id}", app.CancelDeployment)
	mux.HandleFunc("GET /deployments/{id}/logs", app.StreamLogs)

	return app.recoverPanic(app.logRequest(secureHeaders(mux)))
}
