package main

import (
	"errors"
	"maps"
	"net/http"
	"time"

	"github.com/theolujay/appa/internal/data"
)

// createAuthenticationTokenHandler() verifies the user's email and
// password and, if they are valid, generates a new authentication
// token with a 24-hour expiry and returns it to the client.
func (app *application) createAuthenticationTokenHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := app.readJSON(w, r, &input); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	errs := data.ValidateEmail(input.Email)
	maps.Copy(errs, data.ValidatePasswordPlaintext(input.Password))

	if len(errs) > 0 {
		app.failedValidationResponse(w, r, errs)
		return
	}

	user, err := app.models.Users.GetByEmail(input.Email)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.invalidCredentialsResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	match, err := user.Password.Matches(input.Password)
	if !match {
		switch err {
		case nil:
			app.invalidCredentialsResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	token, err := app.models.Tokens.New(user.ID, 24*time.Hour, data.ScopeAuthentication)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusCreated, envelope{"authentication_token": token, "user": user}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
