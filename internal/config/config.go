package config

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"time"
)

type Config struct {
	Environment string
	LogLevel    zerolog.Level
	HTTPTimeout time.Duration
	MaxRetries  int
	NOAABaseURL string
	// Add other common configurations here
}

type Option func(*Config)

// WithEnvironment allows setting the environment
func WithEnvironment(env string) Option {
	return func(c *Config) {
		c.Environment = env
	}
}

// WithLogLevel allows setting the log level
func WithLogLevel(level string) Option {
	return func(c *Config) {
		parsedLevel, err := zerolog.ParseLevel(level)
		if err != nil {
			parsedLevel = zerolog.InfoLevel
		}
		c.LogLevel = parsedLevel
	}
}

// WithHTTPTimeout allows setting the HTTP timeout
func WithHTTPTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.HTTPTimeout = timeout
	}
}

// New creates a new configuration with default values
func New(opts ...Option) *Config {
	cfg := &Config{
		Environment: "production",
		LogLevel:    zerolog.InfoLevel,
		HTTPTimeout: 10 * time.Second,
		MaxRetries:  3,
		NOAABaseURL: "https://api.tidesandcurrents.noaa.gov",
	}

	// Apply options
	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// InitializeLogging sets up logging based on the configuration
func (c *Config) InitializeLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(c.LogLevel)

	// Setup console logger for development environments
	if c.Environment == "local" || c.Environment == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	return New(
		WithEnvironment(getEnvOrDefault("ENV", "production")),
		WithLogLevel(getEnvOrDefault("LOG_LEVEL", "info")),
		WithHTTPTimeout(getDurationEnvOrDefault("HTTP_TIMEOUT", 10*time.Second)),
	)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDurationEnvOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
