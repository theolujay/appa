// Package validator provides a simple, map-based validation toolkit.
// It collects named errors in a Validator struct and offers helpers
// for common checks (email format, permitted values, uniqueness) as
// well as a generic PermittedValue function for any comparable type.
package validator

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var EmailRX = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

type Validator struct {
	Errors ValidationErrors
}

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type ValidationErrors []ValidationError

func (es ValidationErrors) Error() string {
	if len(es) == 0 {
		return ""
	}
	var b strings.Builder
	for i, e := range es {
		if i > 0 {
			_, _ = b.WriteString("; ")
		}
		b.WriteString(e.Error())
	}
	return b.String()
}

func (es ValidationErrors) Unwrap() []error {
	if len(es) == 0 {
		return nil
	}
	errs := make([]error, len(es))
	for i, e := range es {
		errs[i] = e
	}
	return errs
}

// func (v *Validator) err() error {
// 	if len(v.Errors) == 0 {
// 		return nil
// 	}
// 	var errs []error
// 	for _, e := range v.Errors {
// 		errs = append(errs, e)
// 	}
// 	return errors.Join(errs...)
// }

func New() *Validator {
	return &Validator{ValidationErrors{}}
}

func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}

func (v *Validator) addError(field, message string) {
	e := ValidationError{
		Field:   field,
		Message: message,
	}
	v.Errors = append(v.Errors, e)
}

func (v *Validator) Check(ok bool, field, message string) {
	if !ok {
		v.addError(field, message)
	}
}

func PermittedValue[T comparable](value T, permittedValues ...T) bool {
	return slices.Contains(permittedValues, value)
}

func Matches(value string, rx *regexp.Regexp) bool {
	return rx.MatchString(value)
}

// func unique[T comparable](values []T) bool {
// 	uniqueValues := make(map[T]bool, len(values))
// 	for _, value := range values {
// 		uniqueValues[value] = true
// 	}
// 	return len(values) == len(uniqueValues)
// }
