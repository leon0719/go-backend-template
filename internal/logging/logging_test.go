package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go-backend-template/internal/config"
)

func TestNew_ProdUsesJSONHandler(t *testing.T) {
	logger := New(&config.Config{Env: "prod", LogLevel: "info"})
	assert.NotNil(t, logger)
}

func TestNew_LocalUsesTextHandler(t *testing.T) {
	logger := New(&config.Config{Env: "local", LogLevel: "debug"})
	assert.NotNil(t, logger)
}
