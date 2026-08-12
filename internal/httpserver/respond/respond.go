package respond

import (
	"encoding/json"
	"errors"
	"net/http"
)

const (
	CodeValidation   = "validation_error"
	CodeUnauthorized = "unauthorized"
	CodeNotFound     = "not_found"
	CodeConflict     = "conflict"
	CodeInternal     = "internal_error"
	CodeRateLimited  = "rate_limited"
)

// ErrorResponse is the standard JSON error envelope returned by API
// endpoints on failure: {"error":{"code":...,"message":...}}.
type ErrorResponse struct {
	Error struct {
		Code    string `json:"code" example:"validation_error"`
		Message string `json:"message" example:"invalid request"`
	} `json:"error"`
}

func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	body := ErrorResponse{}
	body.Error.Code = code
	body.Error.Message = message
	JSON(w, status, body)
}

// ErrMapping maps a known sentinel error to the HTTP response it should
// produce, checked with errors.Is (so wrapped errors still match).
type ErrMapping struct {
	Target  error
	Status  int
	Code    string
	Message string
}

// MapError writes an error response for err and reports whether it did so.
// It checks mappings in order and uses the first match; any err that matches
// none of them gets a generic 500 so handlers never leak internal details.
// Callers still branch on the (false, nil) case themselves to render the
// success response:
//
//	if respond.MapError(w, err, respond.ErrMapping{Target: ErrNotFound, Status: http.StatusNotFound, Code: respond.CodeNotFound, Message: "article not found"}) {
//		return
//	}
//	respond.JSON(w, http.StatusOK, toResponse(result))
func MapError(w http.ResponseWriter, err error, mappings ...ErrMapping) bool {
	if err == nil {
		return false
	}
	for _, m := range mappings {
		if errors.Is(err, m.Target) {
			Error(w, m.Status, m.Code, m.Message)
			return true
		}
	}
	Error(w, http.StatusInternalServerError, CodeInternal, "internal error")
	return true
}
