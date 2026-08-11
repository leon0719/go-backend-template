package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ReadsEnvVars(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("PORT", "8000")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "local", cfg.Env)
	assert.Equal(t, 8000, cfg.Port)
	assert.Equal(t, "postgres://localhost:5432/test", cfg.DatabaseURL)
	assert.Equal(t, "redis://localhost:6379", cfg.RedisURL)
	assert.Equal(t, "test-secret", cfg.JWTSecret)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_MissingRequiredVar_ReturnsError(t *testing.T) {
	t.Setenv("ENV", "local")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("JWT_SECRET")

	_, err := Load()
	assert.Error(t, err)
}
