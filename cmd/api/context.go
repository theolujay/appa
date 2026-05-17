package main

import (
	"context"
	"net/http"

	"github.com/theolujay/appa/internal/data"
)

type contextKey string

const userContextKey = contextKey("user")

// The contextSetUser() method returns a new copy of the request with the
// provided User struct added to the context using userContextKey as the key.
func (app *application) contextSetUser(r *http.Request, user *data.User) *http.Request {
	ctx := context.WithValue(r.Context(), userContextKey, user)
	return r.WithContext(ctx)
}

// The contextGetUser() method retrieves the User struct from the request
// context. The only time this helper should be used is when it's logically
// expected for a User struct value to be in the request context; otherwise,
// it is firmly an 'unexpected' error, so it's OK to panic.
func (app *application) contextGetUser(r *http.Request) *data.User {
	user, ok := r.Context().Value(userContextKey).(*data.User)
	if !ok {
		panic("missing user value in request context")
	}

	return user
}
