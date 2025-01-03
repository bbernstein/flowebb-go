package config

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var envMutex sync.Mutex

// TestGetCacheConfig runs serially to handle environment variables
func TestGetCacheConfig(t *testing.T) {
	// Disable parallelism for this test group
	if testing.Short() {
		t.Skip("skipping environment-dependent test in short mode")
	}

	// Helper functions to handle environment variable operations
	setEnv := func(key, value string) error {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("setting environment variable %s: %w", key, err)
		}
		return nil
	}

	unsetEnv := func(key string) error {
		if err := os.Unsetenv(key); err != nil {
			return fmt.Errorf("unsetting environment variable %s: %w", key, err)
		}
		return nil
	}

	// Save original environment
	envMutex.Lock()
	originalEnv := make(map[string]string)
	envVars := []string{
		"CACHE_LRU_SIZE",
		"CACHE_LRU_TTL_MINUTES",
		"CACHE_DYNAMO_TTL_DAYS",
		"CACHE_STATION_LIST_TTL_DAYS",
		"CACHE_BATCH_SIZE",
		"CACHE_MAX_BATCH_RETRIES",
		"CACHE_ENABLE_LRU",
		"CACHE_ENABLE_DYNAMO",
	}

	for _, k := range envVars {
		originalEnv[k] = os.Getenv(k)
	}

	// Clear environment for test
	for _, k := range envVars {
		if err := unsetEnv(k); err != nil {
			t.Fatalf("Failed to clear environment: %v", err)
		}
	}
	envMutex.Unlock()

	// Restore environment after test
	defer func() {
		envMutex.Lock()
		for k, v := range originalEnv {
			if v != "" {
				if err := setEnv(k, v); err != nil {
					t.Errorf("Failed to restore environment variable %s: %v", k, err)
				}
			} else {
				if err := unsetEnv(k); err != nil {
					t.Errorf("Failed to restore environment variable %s: %v", k, err)
				}
			}
		}
		envMutex.Unlock()
	}()

	tests := []struct {
		name          string
		envVars       map[string]string
		wantLRUSize   int
		wantTTL       time.Duration
		wantEnableLRU bool
	}{
		{
			name:          "default configuration",
			envVars:       map[string]string{},
			wantLRUSize:   defaultLRUSize,
			wantTTL:       time.Duration(defaultLRUTTLMinutes) * time.Minute,
			wantEnableLRU: true,
		},
		{
			name: "custom configuration",
			envVars: map[string]string{
				"CACHE_LRU_SIZE":        "2000",
				"CACHE_LRU_TTL_MINUTES": "30",
				"CACHE_ENABLE_LRU":      "true",
			},
			wantLRUSize:   2000,
			wantTTL:       30 * time.Minute,
			wantEnableLRU: true,
		},
		{
			name: "disabled LRU cache",
			envVars: map[string]string{
				"CACHE_ENABLE_LRU": "false",
			},
			wantLRUSize:   defaultLRUSize,
			wantTTL:       time.Duration(defaultLRUTTLMinutes) * time.Minute,
			wantEnableLRU: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment
			envMutex.Lock()
			for k, v := range tt.envVars {
				if err := setEnv(k, v); err != nil {
					t.Fatalf("Failed to set test environment: %v", err)
				}
			}
			envMutex.Unlock()

			config := GetCacheConfig()

			assert.Equal(t, tt.wantLRUSize, config.LRUSize)
			assert.Equal(t, tt.wantTTL, config.GetLRUTTL())
			assert.Equal(t, tt.wantEnableLRU, config.EnableLRUCache)

			// Clear test environment
			envMutex.Lock()
			for k := range tt.envVars {
				if err := unsetEnv(k); err != nil {
					t.Fatalf("Failed to clear test environment: %v", err)
				}
			}
			envMutex.Unlock()
		})
	}
}

// TestEnvironmentOverrides tests environment variable overrides
func TestEnvironmentOverrides(t *testing.T) {
	// Helper functions to handle environment variable operations
	setEnv := func(key, value string) error {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("setting environment variable %s: %w", key, err)
		}
		return nil
	}

	unsetEnv := func(key string) error {
		if err := os.Unsetenv(key); err != nil {
			return fmt.Errorf("unsetting environment variable %s: %w", key, err)
		}
		return nil
	}

	// Don't run parallel at the top level
	tests := []struct {
		name    string
		envVars map[string]string
		check   func(*testing.T, *CacheConfig)
	}{
		{
			name: "batch size override",
			envVars: map[string]string{
				"CACHE_BATCH_SIZE": "50",
			},
			check: func(t *testing.T, c *CacheConfig) {
				assert.Equal(t, 50, c.BatchSize)
			},
		},
		{
			name: "dynamo TTL override",
			envVars: map[string]string{
				"CACHE_DYNAMO_TTL_DAYS": "14",
			},
			check: func(t *testing.T, c *CacheConfig) {
				assert.Equal(t, 14*24*time.Hour, c.GetDynamoTTL())
			},
		},
		{
			name: "invalid numeric values",
			envVars: map[string]string{
				"CACHE_LRU_SIZE":   "invalid",
				"CACHE_BATCH_SIZE": "not_a_number",
			},
			check: func(t *testing.T, c *CacheConfig) {
				// Should fall back to defaults
				assert.Equal(t, defaultLRUSize, c.LRUSize)
				assert.Equal(t, defaultBatchSize, c.BatchSize)
			},
		},
	}

	// Save all original env vars
	envMutex.Lock()
	originalEnv := make(map[string]string)
	envVars := []string{
		"CACHE_LRU_SIZE",
		"CACHE_LRU_TTL_MINUTES",
		"CACHE_DYNAMO_TTL_DAYS",
		"CACHE_STATION_LIST_TTL_DAYS",
		"CACHE_BATCH_SIZE",
		"CACHE_MAX_BATCH_RETRIES",
		"CACHE_ENABLE_LRU",
		"CACHE_ENABLE_DYNAMO",
	}
	for _, k := range envVars {
		originalEnv[k] = os.Getenv(k)
		if err := unsetEnv(k); err != nil {
			t.Fatalf("Failed to clear environment: %v", err)
		}
	}
	envMutex.Unlock()

	// Restore all env vars after tests complete
	defer func() {
		envMutex.Lock()
		for k, v := range originalEnv {
			if v != "" {
				if err := setEnv(k, v); err != nil {
					t.Errorf("Failed to restore environment variable %s: %v", k, err)
				}
			} else {
				if err := unsetEnv(k); err != nil {
					t.Errorf("Failed to restore environment variable %s: %v", k, err)
				}
			}
		}
		envMutex.Unlock()
	}()

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars before setting test-specific ones
			envMutex.Lock()
			for _, k := range envVars {
				if err := unsetEnv(k); err != nil {
					t.Fatalf("Failed to clear environment: %v", err)
				}
			}
			// Set only the test-specific env vars
			for k, v := range tt.envVars {
				if err := setEnv(k, v); err != nil {
					t.Fatalf("Failed to set test environment: %v", err)
				}
			}
			envMutex.Unlock()

			config := GetCacheConfig()
			tt.check(t, config)
		})
	}
}

// TestDefaultValues can run in parallel since it doesn't modify environment
func TestDefaultValues(t *testing.T) {
	t.Parallel()

	config := GetCacheConfig()

	// Verify all default values
	assert.Equal(t, defaultLRUSize, config.LRUSize)
	assert.Equal(t, defaultLRUTTLMinutes, config.LRUTTLMinutes)
	assert.Equal(t, defaultDynamoTTLDays, config.DynamoTTLDays)
	assert.Equal(t, defaultStationListTTLDays, config.StationListTTLDays)
	assert.Equal(t, defaultBatchSize, config.BatchSize)
	assert.Equal(t, defaultMaxBatchRetries, config.MaxBatchRetries)
	assert.True(t, config.EnableLRUCache)
	assert.True(t, config.EnableDynamoCache)

	// Verify helper methods return expected values
	assert.Equal(t, time.Duration(defaultLRUTTLMinutes)*time.Minute, config.GetLRUTTL())
	assert.Equal(t, time.Duration(defaultDynamoTTLDays)*24*time.Hour, config.GetDynamoTTL())
	assert.Equal(t, time.Duration(defaultStationListTTLDays)*24*time.Hour, config.GetStationListTTL())
}
