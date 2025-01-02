package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetCacheConfig(t *testing.T) {
	t.Parallel()

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
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Save original environment
			origEnv := make(map[string]string)
			for k := range tt.envVars {
				origEnv[k] = os.Getenv(k)
			}

			// Clear all relevant environment variables
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
				err := os.Unsetenv(k)
				if err != nil {
					return
				}
			}

			// Set test environment
			for k, v := range tt.envVars {
				err := os.Setenv(k, v)
				if err != nil {
					return
				}
			}

			// Restore environment after test
			defer func() {
				for k, v := range origEnv {
					err := os.Setenv(k, v)
					if err != nil {
						return
					}
				}
			}()

			config := GetCacheConfig()

			assert.Equal(t, tt.wantLRUSize, config.LRUSize)
			assert.Equal(t, tt.wantTTL, config.GetLRUTTL())
			assert.Equal(t, tt.wantEnableLRU, config.EnableLRUCache)
		})
	}
}

func TestEnvironmentOverrides(t *testing.T) {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment
			origEnv := make(map[string]string)
			for k := range tt.envVars {
				origEnv[k] = os.Getenv(k)
			}

			// Set test environment
			for k, v := range tt.envVars {
				err := os.Setenv(k, v)
				if err != nil {
					return
				}
			}

			// Restore environment after test
			defer func() {
				for k, v := range origEnv {
					err := os.Setenv(k, v)
					if err != nil {
						return
					}
				}
			}()

			config := GetCacheConfig()
			tt.check(t, config)
		})
	}
}

func TestDefaultValues(t *testing.T) {
	t.Parallel()

	// Clear all relevant environment variables
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

	// Save original environment
	origEnv := make(map[string]string)
	for _, k := range envVars {
		origEnv[k] = os.Getenv(k)
		err := os.Unsetenv(k)
		if err != nil {
			return
		}
	}

	// Restore environment after test
	defer func() {
		for k, v := range origEnv {
			err := os.Setenv(k, v)
			if err != nil {
				return
			}
		}
	}()

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

// Benchmark configuration loading
func BenchmarkGetCacheConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GetCacheConfig()
	}
}
