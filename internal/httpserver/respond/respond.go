package respond

import (
	"encoding/json"
	"net/http"
)

const (
	CodeValidation   = "validation_error"
	CodeUnauthorized = "unauthorized"
	CodeNotFound     = "not_found"
	CodeInternal     = "internal_error"
	CodeRateLimited  = "rate_limited"
)

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	body := errorBody{}
	body.Error.Code = code
	body.Error.Message = message
	JSON(w, status, body)
}
