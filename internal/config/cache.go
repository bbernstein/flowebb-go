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
	TidePredictionLRUSize       int
	TidePredictionLRUTTLMinutes int

	// DynamoDB Cache settings
	TidePredictionDynamoTTLDays int
	StationListTTLDays          int

	// GraphQL Cache settings
	GraphQLLRUSize       int
	GraphQLLRUTTLMinutes int

	// Batch processing settings
	BatchSize       int
	MaxBatchRetries int

	// General settings
	EnableLRUCache    bool
	EnableDynamoCache bool
}

const (
	// Default values
	defaultTidePredictionLRUSize    = 1000
	defaultTidePredictionTTLMinutes = 15
	defaultDynamoTTLDays            = 2
	defaultStationListTTLDays       = 2
	defaultGraphQLLRUSize           = 5000
	defaultGraphQLTTLMinutes        = 60
	defaultBatchSize                = 25
	defaultMaxBatchRetries          = 3
)

// GetCacheConfig returns the cache configuration from environment variables or defaults
func GetCacheConfig() *CacheConfig {
	config := &CacheConfig{
		// Set defaults
		TidePredictionLRUSize:       getEnvInt("CACHE_TIDE_LRU_SIZE", defaultTidePredictionLRUSize),
		TidePredictionLRUTTLMinutes: getEnvInt("CACHE_TIDE_LRU_TTL_MINUTES", defaultTidePredictionTTLMinutes),
		TidePredictionDynamoTTLDays: getEnvInt("CACHE_DYNAMO_TTL_DAYS", defaultDynamoTTLDays),
		StationListTTLDays:          getEnvInt("CACHE_STATION_LIST_TTL_DAYS", defaultStationListTTLDays),
		GraphQLLRUSize:              getEnvInt("CACHE_GRAPHQL_LRU_SIZE", defaultGraphQLLRUSize),
		GraphQLLRUTTLMinutes:        getEnvInt("CACHE_GRAPHQL_TTL_MINUTES", defaultGraphQLTTLMinutes),
		BatchSize:                   getEnvInt("CACHE_BATCH_SIZE", defaultBatchSize),
		MaxBatchRetries:             getEnvInt("CACHE_MAX_BATCH_RETRIES", defaultMaxBatchRetries),
		EnableLRUCache:              getEnvBool("CACHE_ENABLE_LRU", true),
		EnableDynamoCache:           getEnvBool("CACHE_ENABLE_DYNAMO", true),
	}

	log.Debug().
		Int("TidePredictionLRUSize", config.TidePredictionLRUSize).
		Int("TidePredictionLRUTTLMinutes", config.TidePredictionLRUTTLMinutes).
		Int("TidePredictionDynamoTTLDays", config.TidePredictionDynamoTTLDays).
		Int("StationListTTLDays", config.StationListTTLDays).
		Int("GraphQLLRUSize", config.GraphQLLRUSize).
		Int("GraphQLLRUTTLMinutes", config.GraphQLLRUTTLMinutes).
		Int("BatchSize", config.BatchSize).
		Int("MaxBatchRetries", config.MaxBatchRetries).
		Bool("EnableLRUCache", config.EnableLRUCache).
		Bool("EnableDynamoCache", config.EnableDynamoCache).
		Msg("Cache configuration loaded")

	return config
}

// Helper methods for the CacheConfig struct
func (c *CacheConfig) GetTidePredictionLRUTTL() time.Duration {
	return time.Duration(c.TidePredictionLRUTTLMinutes) * time.Minute
}

func (c *CacheConfig) GetGraphQLLRUTTL() time.Duration {
	return time.Duration(c.GraphQLLRUTTLMinutes) * time.Minute
}

func (c *CacheConfig) GetDynamoTTL() time.Duration {
	return time.Duration(c.TidePredictionDynamoTTLDays) * 24 * time.Hour
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
