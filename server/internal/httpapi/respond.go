package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
)

// maxJSONBody bounds request bodies. Action submissions are the largest
// legitimate payloads and are limited separately inside the app.
const maxJSONBody = 2 << 20

// errorBody is the shape of every error response.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details,omitempty"`
	} `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

var statusByCode = map[app.Code]int{
	app.CodeInvalid:      http.StatusBadRequest,
	app.CodeUnauthorized: http.StatusUnauthorized,
	app.CodeForbidden:    http.StatusForbidden,
	app.CodeNotFound:     http.StatusNotFound,
	app.CodeConflict:     http.StatusConflict,
	app.CodeRateLimited:  http.StatusTooManyRequests,
	app.CodeUnavailable:  http.StatusServiceUnavailable,
	app.CodeUpstream:     http.StatusBadGateway,
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var body errorBody
	body.RequestID = requestID(r.Context())
	status := http.StatusInternalServerError
	var ae *app.Error
	var de *decodeError
	switch {
	case errors.As(err, &ae):
		status = statusByCode[ae.Code]
		if status == 0 {
			status = http.StatusInternalServerError
		}
		body.Error.Code, body.Error.Message, body.Error.Details = string(ae.Code), ae.Message, ae.Details
	case errors.As(err, &de):
		status = http.StatusBadRequest
		body.Error.Code, body.Error.Message = string(app.CodeInvalid), de.Error()
	default:
		s.log.Error("request failed", "request_id", body.RequestID, "method", r.Method, "path", r.URL.Path, "error", err)
		body.Error.Code, body.Error.Message = "internal_error", "something went wrong; the error has been logged"
	}
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "1")
	}
	writeJSON(w, status, body)
}

type decodeError struct{ msg string }

func (e *decodeError) Error() string { return e.msg }

// decode reads a JSON body into v, rejecting unknown fields so typos in
// client code fail loudly instead of being ignored.
func decode(r *http.Request, v any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return &decodeError{"Content-Type must be application/json"}
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		switch {
		case errors.Is(err, io.EOF):
			return &decodeError{"request body is empty"}
		case errors.As(err, &syn):
			return &decodeError{fmt.Sprintf("request body is not valid JSON (at byte %d)", syn.Offset)}
		case errors.As(err, &typ):
			return &decodeError{fmt.Sprintf("field %q must be %s", typ.Field, typ.Type)}
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			return &decodeError{strings.TrimPrefix(err.Error(), "json: ")}
		case err.Error() == "http: request body too large":
			return &decodeError{"request body is too large"}
		}
		return &decodeError{"request body could not be read: " + err.Error()}
	}
	if dec.More() {
		return &decodeError{"request body has data after the JSON value"}
	}
	return nil
}

// waitParam reads ?wait= as seconds or a Go duration, capped at max.
func waitParam(r *http.Request, max time.Duration) (time.Duration, error) {
	v := r.URL.Query().Get("wait")
	if v == "" {
		return 0, nil
	}
	var d time.Duration
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		d = time.Duration(secs * float64(time.Second))
	} else if d, err = time.ParseDuration(v); err != nil {
		return 0, &decodeError{"wait must be seconds, such as 25, or a duration, such as 25s"}
	}
	if d < 0 {
		return 0, &decodeError{"wait must not be negative"}
	}
	return min(d, max), nil
}

func intParam(r *http.Request, name string, def int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, &decodeError{name + " must be an integer"}
	}
	return n, nil
}
