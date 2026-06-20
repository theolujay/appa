package main

import (
	"errors"
	"net/http"
	"time"

	da "github.com/theolujay/appa/internal/data"
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

	var errs []error
	if err := da.ValidateEmail(input.Email); err != nil {
		errs = append(errs, err)
	}

	if err := da.ValidatePasswordPlaintext(input.Password); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		err := errors.Join(errs...)
		app.failedValidationResponse(w, r, err)
		return
	}

	user, err := app.models.Users.GetByEmail(input.Email)
	if err != nil {
		switch {
		case errors.Is(err, da.ErrRecordNotFound):
			app.invalidCredentialsResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	match, err := user.Password.Matches(input.Password)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	if !match {
		app.invalidCredentialsResponse(w, r)
		return
	}

	token, err := app.models.Tokens.New(user.ID, 24*time.Hour, da.ScopeAuthentication)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusCreated, envelope{"authentication_token": token, "user": user}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
