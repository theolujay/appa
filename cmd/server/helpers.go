package main

import (
	"fmt"
	"net/http"
	"runtime/debug"
)

// serverError helper writes an error message and stack trace to the errorLog,
// then sends a generic 500 Internal Server Error response to the user.
func (app *application) serverError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())
	app.errorLog.Output(2, trace)

	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// clientError helper sends a specific status code and corresponding description
func (app *application) clientError(w http.ResponseWriter, message string, status int) {
	if message == "" {
		message = http.StatusText(status)
	}
	http.Error(w, message, status)
}

// notFound helper sends the same NotFound error from http,
// but this is used for consistency
func (app *application) notFound(w http.ResponseWriter) {
	app.clientError(w, "", http.StatusNotFound)
}
