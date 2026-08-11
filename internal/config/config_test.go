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
	t.Setenv("ARTICLE_PUBLISHED_WEBHOOK_URL", "https://example.com/webhook")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "local", cfg.Env)
	assert.Equal(t, 8000, cfg.Port)
	assert.Equal(t, "postgres://localhost:5432/test", cfg.DatabaseURL)
	assert.Equal(t, "redis://localhost:6379", cfg.RedisURL)
	assert.Equal(t, "test-secret", cfg.JWTSecret)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "https://example.com/webhook", cfg.ArticlePublishedWebhookURL)
}

func TestLoad_MissingRequiredVar_ReturnsError(t *testing.T) {
	t.Setenv("ENV", "local")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("JWT_SECRET")

	_, err := Load()
	assert.Error(t, err)
}

func TestLoad_UsesENVVariable(t *testing.T) {
	// Change to temp directory and write a custom .env file
	t.Chdir(t.TempDir())

	// Write .env.custom with a distinct DATABASE_URL value
	envContent := "DATABASE_URL=postgres://custom:5432/custom_db\n"
	err := os.WriteFile(".env.custom", []byte(envContent), 0o600)
	require.NoError(t, err)

	// Set ENV to "custom" so it should load .env.custom
	t.Setenv("ENV", "custom")
	// Set other required vars except DATABASE_URL (it should come from .env.custom)
	t.Setenv("PORT", "9000")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	require.NoError(t, err)

	// Verify the DATABASE_URL came from .env.custom, not from env vars
	assert.Equal(t, "postgres://custom:5432/custom_db", cfg.DatabaseURL)
	assert.Equal(t, "custom", cfg.Env)
	assert.Equal(t, 9000, cfg.Port)
}

func TestLoadDatabaseOnly_ReadsDatabaseURL(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	os.Unsetenv("REDIS_URL")
	os.Unsetenv("JWT_SECRET")

	cfg, err := LoadDatabaseOnly()
	require.NoError(t, err)

	assert.Equal(t, "postgres://localhost:5432/test", cfg.DatabaseURL)
}

func TestLoadDatabaseOnly_MissingDatabaseURL_ReturnsError(t *testing.T) {
	t.Setenv("ENV", "local")
	os.Unsetenv("DATABASE_URL")

	_, err := LoadDatabaseOnly()
	assert.Error(t, err)
}

func TestLoadServerOnly_ReadsEnvAndPort(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("PORT", "9090")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("REDIS_URL")
	os.Unsetenv("JWT_SECRET")

	cfg, err := LoadServerOnly()
	require.NoError(t, err)

	assert.Equal(t, "local", cfg.Env)
	assert.Equal(t, 9090, cfg.Port)
}

func TestLoadServerOnly_DefaultsPortTo8000(t *testing.T) {
	t.Setenv("ENV", "local")
	os.Unsetenv("PORT")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("REDIS_URL")
	os.Unsetenv("JWT_SECRET")

	cfg, err := LoadServerOnly()
	require.NoError(t, err)

	assert.Equal(t, 8000, cfg.Port)
}
