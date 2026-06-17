package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/theolujay/appa/internal/data"
	vd "github.com/theolujay/appa/internal/validator"
)

// registerUserHandler() creates a new user in the database, generates an
// activation token, and sends a welcome email with the token in the background.
func (app *application) registerUserHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := &data.User{
		Name:      input.Name,
		Email:     input.Email,
		Activated: false,
	}

	err = user.Password.Set(input.Password)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	if errs := data.ValidateUser(user); len(errs) > 0 {
		app.failedValidationResponse(w, r, errs)
		return
	}

	err = app.models.Users.Insert(user)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrDuplicateEmail):
			app.failedValidationResponse(w, r, vd.Error{
				"email": "a user with this email already exists",
			})
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	token, err := app.models.Tokens.New(user.ID, 3*24*time.Hour, data.ScopeActivation)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.background(func() {
		data := map[string]any{
			"activationToken": token.Plaintext,
			"userID":          user.ID,
		}
		err = app.mailer.Send(user.Email, "user_welcome.tmpl", data)
		if err != nil {
			// Use app.logger.Error() helper to manage the error
			// instead of the app.serverErrorResponse()
			app.logger.Error(err.Error())
		}
	})

	err = app.writeJSON(w, http.StatusAccepted, envelope{"user": user}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// activateUserHandler() activates a user account by verifying the provided
// activation token. If the token is valid and not expired, the user's
// activated status is set to true and all activation tokens for the user
// are deleted.
func (app *application) activateUserHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TokenPlaintext string `json:"token"`
	}
	if err := app.readJSON(w, r, &input); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if errs := data.ValidateTokenPlaintext(input.TokenPlaintext); len(errs) > 0 {
		app.failedValidationResponse(w, r, errs)
		return
	}

	// Retrieve the details of the user associated with the token using the
	// GetForToken() method. If no matching record is found, then let the
	// client know that the token they provided is not valid.
	user, err := app.models.Users.GetForToken(data.ScopeActivation, input.TokenPlaintext)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.failedValidationResponse(w, r, vd.Error{
				"token": "invalid or expired activation token",
			})
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	user.Activated = true

	if err = app.models.Users.Update(user); err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if err = app.models.Tokens.DeleteAllForUser(data.ScopeActivation, user.ID); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	if err = app.writeJSON(w, http.StatusOK, envelope{"user": user}, nil); err != nil {
		app.serverErrorResponse(w, r, err)
	}

}
