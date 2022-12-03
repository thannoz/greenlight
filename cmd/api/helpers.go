package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
)

type envelope map[string]any

func (app *application) readIDParam(r *http.Request) (int64, error) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.ParseInt(params.ByName("id"), 10, 64)
	if err != nil || id < 0 {
		return 0, errors.New("invalid id parameter")
	}

	return id, nil
}

// writeJSON sends responses & takes the destination
// http.ResponseWriter, the HTTP status code to send, the data to encode to JSON, and a
// header map containing any additional HTTP headers we want to include in the response.
func (app *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error {
	js, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	js = append(js, '\n')

	// At this point, we know that we won't encounter any more errors before writing the
	// response, so it's safe to add any headers that we want to include. We loop
	// through the header map and add each header to the http.ResponseWriter header map.
	for key, value := range headers {
		w.Header()[key] = value
	}

	// Add the "Content-Type: application/json" header, then write the status code and
	// JSON response.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil
}

// readJSON decodes the JSON from the request body
func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	maxBites := 1_048_576
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBites))

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	// Decode the request body into the target destination
	err := dec.Decode(dst)
	if err != nil {
		// If there is an error during decoding, start the triage process
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError
		var maxBitesError *http.MaxBytesError

		switch {
		// Check whether the error has the type *json.SyntaxError
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

		// Check for syntax errors in the JSON.
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")

		// Check whether JSON value is the wrong type for the target destination.
		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON	type at (at character %d)", unmarshalTypeError.Offset)

		// Check if request body is empty
		case errors.Is(err, io.EOF):
			return errors.New("body cannot be empty")

		// Check for field name that cannot be mapped to the target destination
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field")
			return fmt.Errorf("body contains unknown key %s", fieldName)

		// Check whether the error has a type of *http.MaxBytesError
		case errors.As(err, &maxBitesError):
			return fmt.Errorf("body cannot be larger than %d bytes", maxBitesError.Limit)

		// This error is returned when we pass something that is not a non-nil pointer to Decode-method.
		case errors.As(err, &invalidUnmarshalError):
			panic(err)

		default:
			return err
		}
	}

	// Call Decode() again, using a pointer to an empty anonymous struct as the
	// destination. If the request body only contained a single JSON value this will
	// return an io.EOF error. So if we get anything else, we know that there is
	// additional data in the request body and we return our own custom error message.
	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		return errors.New("body can only contain a single json value")
	}

	return nil
}

// getStrEnv reads from the environment variables & returns as string
func getStrEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic("error reading environment variable")
	}
	return val
}

// getIntEnv converts env to int
func getIntEnv(key string) int {
	val := getStrEnv(key)
	env, err := strconv.Atoi(val)
	if err != nil {
		panic("error converting environment variable")
	}
	return env
}
