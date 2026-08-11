package respond

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSON_WritesStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, 201, map[string]string{"id": "abc"})

	assert.Equal(t, 201, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "abc", body["id"])
}

func TestError_WritesEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, 404, CodeNotFound, "article not found")

	assert.Equal(t, 404, rec.Code)

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, CodeNotFound, body.Error.Code)
	assert.Equal(t, "article not found", body.Error.Message)
}
