package config

import (
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestNewConfigWithDefaults(t *testing.T) {
	cfg := New()

	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, zerolog.InfoLevel, cfg.LogLevel)
	assert.Equal(t, 10*time.Second, cfg.HTTPTimeout)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, "https://api.tidesandcurrents.noaa.gov", cfg.NOAABaseURL)
}

func TestWithEnvironment(t *testing.T) {
	cfg := New(WithEnvironment("development"))

	assert.Equal(t, "development", cfg.Environment)
}

func TestWithLogLevel(t *testing.T) {
	cfg := New(WithLogLevel("debug"))

	assert.Equal(t, zerolog.DebugLevel, cfg.LogLevel)
}

func TestWithHTTPTimeout(t *testing.T) {
	cfg := New(WithHTTPTimeout(30 * time.Second))

	assert.Equal(t, 30*time.Second, cfg.HTTPTimeout)
}

func TestInitializeLogging(t *testing.T) {
	cfg := New(WithEnvironment("local"), WithLogLevel("debug"))
	cfg.InitializeLogging()

	assert.Equal(t, zerolog.DebugLevel, zerolog.GlobalLevel())
}

func TestLoadFromEnv(t *testing.T) {
	err := os.Setenv("ENV", "test")
	if err != nil {
		return
	}
	err = os.Setenv("LOG_LEVEL", "warn")
	if err != nil {
		return
	}
	err = os.Setenv("HTTP_TIMEOUT", "5s")
	if err != nil {
		return
	}

	cfg := LoadFromEnv()

	assert.Equal(t, "test", cfg.Environment)
	assert.Equal(t, zerolog.WarnLevel, cfg.LogLevel)
	assert.Equal(t, 5*time.Second, cfg.HTTPTimeout)

	// Clean up
	err = os.Unsetenv("ENV")
	if err != nil {
		return
	}
	err = os.Unsetenv("LOG_LEVEL")
	if err != nil {
		return
	}
	err = os.Unsetenv("HTTP_TIMEOUT")
	if err != nil {
		return
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	err := os.Setenv("TEST_ENV_VAR", "value")
	if err != nil {
		return
	}
	defer func() {
		err := os.Unsetenv("TEST_ENV_VAR")
		if err != nil {
			return
		}
	}()

	assert.Equal(t, "value", getEnvOrDefault("TEST_ENV_VAR", "default"))
	assert.Equal(t, "default", getEnvOrDefault("NON_EXISTENT_ENV_VAR", "default"))
}

func TestGetDurationEnvOrDefault(t *testing.T) {
	err := os.Setenv("TEST_DURATION_ENV_VAR", "2s")
	if err != nil {
		return
	}
	defer func() {
		err := os.Unsetenv("TEST_DURATION_ENV_VAR")
		if err != nil {
			return
		}
	}()

	assert.Equal(t, 2*time.Second, getDurationEnvOrDefault("TEST_DURATION_ENV_VAR", 1*time.Second))
	assert.Equal(t, 1*time.Second, getDurationEnvOrDefault("NON_EXISTENT_DURATION_ENV_VAR", 1*time.Second))
}
