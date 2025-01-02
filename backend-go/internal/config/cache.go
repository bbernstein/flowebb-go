package config

import (
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// CacheConfig holds all cache-related configuration
type CacheConfig struct {
	// LRU Cache settings
	LRUSize       int
	LRUTTLMinutes int

	// DynamoDB Cache settings
	DynamoTTLDays      int
	StationListTTLDays int

	// Batch processing settings
	BatchSize       int
	MaxBatchRetries int

	// Station List Cache settings
	StationListSize int

	// General settings
	EnableLRUCache    bool
	EnableDynamoCache bool
}

const (
	// Default values
	defaultLRUSize            = 1000
	defaultLRUTTLMinutes      = 15
	defaultDynamoTTLDays      = 2
	defaultStationListTTLDays = 2
	defaultBatchSize          = 25
	defaultMaxBatchRetries    = 3
)

// GetCacheConfig returns the cache configuration from environment variables or defaults
func GetCacheConfig() *CacheConfig {
	config := &CacheConfig{
		// Set defaults
		LRUSize:            getEnvInt("CACHE_LRU_SIZE", defaultLRUSize),
		LRUTTLMinutes:      getEnvInt("CACHE_LRU_TTL_MINUTES", defaultLRUTTLMinutes),
		DynamoTTLDays:      getEnvInt("CACHE_DYNAMO_TTL_DAYS", defaultDynamoTTLDays),
		StationListTTLDays: getEnvInt("CACHE_STATION_LIST_TTL_DAYS", defaultStationListTTLDays),
		BatchSize:          getEnvInt("CACHE_BATCH_SIZE", defaultBatchSize),
		MaxBatchRetries:    getEnvInt("CACHE_MAX_BATCH_RETRIES", defaultMaxBatchRetries),
		EnableLRUCache:     getEnvBool("CACHE_ENABLE_LRU", true),
		EnableDynamoCache:  getEnvBool("CACHE_ENABLE_DYNAMO", true),
	}

	log.Debug().
		Int("lru_size", config.LRUSize).
		Int("lru_ttl_minutes", config.LRUTTLMinutes).
		Int("dynamo_ttl_days", config.DynamoTTLDays).
		Int("station_list_ttl_days", config.StationListTTLDays).
		Int("batch_size", config.BatchSize).
		Int("max_batch_retries", config.MaxBatchRetries).
		Bool("enable_lru", config.EnableLRUCache).
		Bool("enable_dynamo", config.EnableDynamoCache).
		Msg("Cache configuration loaded")

	return config
}

// Helper methods for the CacheConfig struct
func (c *CacheConfig) GetLRUTTL() time.Duration {
	return time.Duration(c.LRUTTLMinutes) * time.Minute
}

func (c *CacheConfig) GetDynamoTTL() time.Duration {
	return time.Duration(c.DynamoTTLDays) * 24 * time.Hour
}

func (c *CacheConfig) GetStationListTTL() time.Duration {
	return time.Duration(c.StationListTTLDays) * 24 * time.Hour
}

// Helper functions to get environment variables with defaults
func getEnvInt(key string, defaultVal int) int {
	if val, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
		log.Warn().Str("key", key).Msg("Invalid integer value in environment variable, using default")
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, exists := os.LookupEnv(key); exists {
		return val == "true" || val == "1" || val == "yes"
	}
	return defaultVal
}
