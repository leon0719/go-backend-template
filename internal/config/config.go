package config

import (
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// ServerConfig holds only the settings needed to know where the server is
// listening. It is used by the in-container healthcheck probe, which must
// work regardless of unrelated config (DATABASE_URL/REDIS_URL/JWT_SECRET).
// Config embeds it so the Port/Env defaults are declared exactly once.
type ServerConfig struct {
	Env  string `env:"ENV" envDefault:"local"`
	Port int    `env:"PORT" envDefault:"8000"`
}

// LoadServerOnly loads only ServerConfig (Env/Port), for the in-container
// healthcheck probe which must work regardless of unrelated config.
func LoadServerOnly() (*ServerConfig, error) {
	loadDotenv()

	cfg := &ServerConfig{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse server config: %w", err)
	}
	return cfg, nil
}

type Config struct {
	ServerConfig
	DatabaseURL string `env:"DATABASE_URL,required"`
	RedisURL    string `env:"REDIS_URL,required"`
	JWTSecret   string `env:"JWT_SECRET,required"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

	// CORSAllowedOrigins is an exact-match allow-list of browser origins
	// (comma-separated, e.g. "https://app.example.com,https://admin.example.com").
	// Empty (the default) disables CORS entirely — no CORS headers are
	// emitted, which is the safe default for a template.
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`

	// TrustedProxies lists CIDRs (comma-separated) whose requests are allowed
	// to set X-Forwarded-For. Only when the direct peer falls inside one of
	// these ranges is the header used to derive the client IP; otherwise the
	// header is ignored and r.RemoteAddr wins. Empty (the default) means the
	// server assumes it is directly internet-facing.
	//
	// SECURITY: setting this too broadly (e.g. 0.0.0.0/0) lets any client
	// spoof its IP and bypass IP rate limiting. Leaving it empty while
	// running behind a reverse proxy collapses all clients into one bucket.
	TrustedProxies []string `env:"TRUSTED_PROXIES" envSeparator:","`

	ArticlePublishedWebhookURL string `env:"ARTICLE_PUBLISHED_WEBHOOK_URL" envDefault:""`
}

func Load() (*Config, error) {
	loadDotenv()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// DatabaseConfig holds only the settings needed to reach the database. It is
// used by tools (e.g. the migrate binary) that must be able to run without
// the full application config — in particular without REDIS_URL/JWT_SECRET,
// which a migration run has no business needing.
type DatabaseConfig struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
}

// LoadDatabaseOnly loads only DatabaseConfig, for tools (migrations) that
// must not require the full application config.
func LoadDatabaseOnly() (*DatabaseConfig, error) {
	loadDotenv()

	cfg := &DatabaseConfig{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	return cfg, nil
}

// loadDotenv loads the dotenv file matching ENV (defaulting to "local"),
// e.g. .env.local or .env.prod. It is best-effort: a missing file is not an
// error, since env vars may be supplied directly (e.g. in containers).
func loadDotenv() {
	envName := os.Getenv("ENV")
	if envName == "" {
		envName = "local"
	}
	_ = godotenv.Load(fmt.Sprintf(".env.%s", envName))
}
