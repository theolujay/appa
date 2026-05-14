package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/theolujay/appa/internal/validator"
)

type envelope map[string]any

func (app *application) readIDParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errors.New("invalid id parameter")
	}
	return id, nil
}

func (app *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error {
	// Encode the data to JSON, returning the error if there's one
	js, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	// Add a newline to make it easier to view in the terminal
	js = append(js, '\n')

	// There surely won't be any more errors at this point before writing
	// the response, so it's safe to add a any headers to be included. It's
	// OK if the privided header map is nil. Go doesn't throw and error if
	// it is to range over (or generally, read from) a nil map, even though
	// using maps.Copy is also possible cleaner.
	maps.Copy(w.Header(), headers)
	// for key, val := range headers {
	// 	w.Header()[key] = val
	// }

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil
}

func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	// Use http.MaxBytesReader() to limit the size of the request body to 1,048,576
	// bytes (1MB).
	r.Body = http.MaxBytesReader(w, r.Body, 1_048_576)

	// Initialize a new json.Decoder instance which reads from the request body, and
	// call the DisallowUnknownFields() method on it before decoding.
	// Afterwards, use the Decode() method to decode the body content into the `dst`.
	// NOTE: `dst` must be a non-nil pointer, as we'll need it to update the exact
	// anonymous struct we created and be able to use it afterwards. And http.Server
	// closes r.Body afterwards, so its not necessary to do it.
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err != nil {
		// Decoding failed. Now triage the error to decide what to return to the client.
		// The core question for each case is: do we just need to know this error exists
		// (errors.Is), or do we need to read structured data off if (errors.As)?

		// Declare typed variables for the errors that carry structured payloads.
		// errors.As will populate these if it finds a matching type anywhere in the error
		// tree, even if the error has been wrapped by intermediate layers.
		var (
			syntaxError           *json.SyntaxError
			unmarshalTypeError    *json.UnmarshalTypeError
			invalidUnmarshalError *json.InvalidUnmarshalError
			maxBytesError         *http.MaxBytesError
		)

		switch {
		// *json.SyntaxError is a typed error that carries an Offset field -- the byte
		// position in the input where the syntax problem was detected. Use errors.As
		// (not errors.Is) because we need to read that field to give the client a
		// precise location
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

		// Due to an inconsistency in the encoding/json package, Decode() can also return
		// io.ErrUnexpectedEOF for certain syntax errors -- particularly truncated input
		// -- instad of wrapping the problem in a *json.SyntaxError. Handle this
		// separately because the error arrives as a plain sentinel value with no
		// structured payload to extract
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")

		// *json.UnmarshalTypeError means the JSON was syntactically valid, but a value's
		// type doesn't match the targe Go field. For example, sending a sring where an
		// integer is expected. Like SyntaxError, this type carries structured data -- the
		// field name and byte offset -- so we use errors.As to extract them and build a
		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)

		// io.EOF means the request body was completely empty. This is a plain sentinel
		//-- no payload to extract -- so errors.Is is suffienct.
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")

		// If the JSON contains a field that cannot be mapped to the target destination
		// then Decoded() will now return an error message in the format `json: unknown
		// field "<name>"`. Check for this, extract the field name from the error and
		// interpolate it into the custom error message.
		// NOTE: there's an open issue at
		// https://github.com/golang/go/issues/29035 regarding turning this into a distinc
		// error type in the future.
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("body contains unknown key %s", fieldName)

		// Use errors.As() to check whether the error has the type *http.MaxBytesError.
		// If it does, then it means the request body exceeded our size limit of 1MB
		// and we return a clear error message.
		case errors.As(err, &maxBytesError):
			return fmt.Errorf("body must not be larger than %d bytes", maxBytesError.Limit)

		// *json.InvalidUnmarshalError means `dst` was passed as nil or a non-pointer.
		// This is a programming error -- a bug where readJSON was called -- not a
		// client error. Rather panic than return because:
		//		(a) the client did nothing wrong and shouldn't receive this as an
		//			HTTP error, and
		// 		(b) panics are a convention for signalling bugs that should be caught
		// 			in development, not conditions to handle gracefully at runtime, and
		//		(c) we do handle this with recoverPanic in ./cmd/api/middleware.go
		case errors.As(err, &invalidUnmarshalError):
			panic(err)

		// Any other error is returned as-is. This covers edge cases not anticipated --
		// connection ressets, oversized bodies, and so on...
		default:
			return err
		}
	}

	// Call Decode() again, using a pointer to an empty anonymous struct as the destination.
	// If the request body only contained a single JSON value, this will return an io.EOF
	// error. If anything else is gotten, then there is additional data in the request body,
	// so return a custom error message.
	err = dec.Decode(&struct{}{})
	if !errors.Is(err, io.EOF) {
		return errors.New("body must only contain a single JSON value")
	}

	return nil
}

// The readString() helper returns a string value from the query string,
// or the provided default value if no matching key could be found.
func (app *application) readString(qs url.Values, key, defaultValue string) string {
	s := qs.Get(key)

	if s == "" {
		return defaultValue
	}

	return s

}

// The readCSV() helper reads a string value from the query sting and
// then splits it into a slice on the comma character. If no matching
// key could be found, it returns the provided default value.
func (app *application) readCSV(qs url.Values, key string, defaultValue []string) []string {
	csv := qs.Get(key)

	if csv == "" {
		return defaultValue
	}

	return strings.Split(csv, ",")
}

// The readInt() helper reads a string value from the query string and
// converts it into an integer before returning. If no matching key
// could be found, it returns the provided default value. If the value
// couldn't be converted to an integer, then we record an error message
// in the provided Validator instance.
func (app *application) readInt(qs url.Values, key string, defaultValue int, v *validator.Validator) int {
	s := qs.Get(key)

	if s == "" {
		return defaultValue
	}

	i, err := strconv.Atoi(s)
	if err != nil {
		v.AddError(key, "must be an integer value")
		return defaultValue
	}

	return i
}

// The background() helper accepts an arbitrary function as a parameter
// to run withing a recover-able goroutine
func (app *application) background(fn func()) {
	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		defer func() {
			if err := recover(); err != nil {
				app.logger.Error(fmt.Sprintf("%v", err))
			}
		}()

		fn()
	}()
}
