// Package request holds the shared HTTP request-decoding helpers used by
// every app's handler.go. Keeping one implementation here (rather than one
// per app) is what makes body-size limits and validation uniform across the
// API — a template teaches by example, and duplicated decode code is exactly
// how an endpoint ends up silently skipping validation.
package request

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"

	"go-backend-template/internal/httpserver/respond"
)

// MaxBodyBytes caps the size of a JSON request body (1 MiB). Without it an
// unauthenticated client can stream a multi-GB body at /auth/register and
// exhaust server memory. Raise it deliberately (per-route, via your own
// http.MaxBytesReader) if an endpoint legitimately accepts more.
const MaxBodyBytes int64 = 1 << 20

// Validate is the process-wide validator instance. validator.Validate caches
// struct reflection metadata and is safe for concurrent use, so there should
// be exactly one — not one per package.
var Validate = validator.New()

// DecodeAndValidate reads a size-limited JSON body into dst and runs
// validator struct-tag validation on it. On failure it writes the standard
// 422 error envelope and returns false; callers just `return`.
func DecodeAndValidate[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			respond.Error(w, http.StatusRequestEntityTooLarge, respond.CodeValidation, "request body too large")
			return false
		}
		respond.Error(w, http.StatusUnprocessableEntity, respond.CodeValidation, "invalid JSON body")
		return false
	}
	if err := Validate.Struct(dst); err != nil {
		respond.Error(w, http.StatusUnprocessableEntity, respond.CodeValidation, err.Error())
		return false
	}
	return true
}
