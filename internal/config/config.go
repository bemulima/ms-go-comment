package config

import "github.com/kelseyhightower/envconfig"

// Config holds process configuration. Domain-specific settings belong to the
// database-owned space/thread policy rather than environment variables.
type Config struct {
	HTTPPort                  string `envconfig:"HTTP_PORT" default:"8080"`
	DatabaseURL               string `envconfig:"DATABASE_URL" default:"postgres://postgres:postgres@localhost:5432/ms_comment?sslmode=disable"`
	NATSURL                   string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	InternalAPIToken          string `envconfig:"INTERNAL_API_TOKEN" default:"change-me"`
	FileStorageServiceBaseURL string `envconfig:"FILESTORAGE_SERVICE_BASE_URL" default:"http://localhost:8088"`
	ShutdownTimeout           int    `envconfig:"SHUTDOWN_TIMEOUT" default:"10"`
	ServiceMode               string `envconfig:"SERVICE_MODE" default:"all"`
}

// Load reads environment variables into Config.
func Load() (Config, error) {
	var cfg Config
	err := envconfig.Process("", &cfg)
	return cfg, err
}
