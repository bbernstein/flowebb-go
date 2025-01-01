package cache

import (
	"context"
	"fmt"
	"github.com/hashicorp/golang-lru/v2"
	"github.com/rs/zerolog/log"
	"time"
)

const (
	// DefaultLRUSize is the default size of the LRU cache
	DefaultLRUSize = 1000
	// DefaultTTL is the default time-to-live for cache entries
	DefaultTTL = 15 * time.Minute
)

// LRUCacheEntry wraps the cached data with metadata
type LRUCacheEntry struct {
	Data      *TidePredictionRecord
	ExpiresAt time.Time
}

// CacheService provides a two-layer caching system using LRU and DynamoDB
type CacheService struct {
	lru          *lru.Cache[string, *LRUCacheEntry]
	dynamoCache  *DynamoPredictionCache
	ttl          time.Duration
	lruHits      uint64
	lruMisses    uint64
	dynamoHits   uint64
	dynamoMisses uint64
}

// NewCacheService creates a new cache service with both LRU and DynamoDB caching
func NewCacheService(ctx context.Context, lruSize int, ttl time.Duration) (*CacheService, error) {
	if lruSize <= 0 {
		lruSize = DefaultLRUSize
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	// Initialize DynamoDB client
	dynamoClient, err := NewDynamoClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating DynamoDB client: %w", err)
	}

	// Create LRU cache
	lruCache, err := lru.New[string, *LRUCacheEntry](lruSize)
	if err != nil {
		return nil, fmt.Errorf("creating LRU cache: %w", err)
	}

	return &CacheService{
		lru:         lruCache,
		dynamoCache: NewDynamoPredictionCache(dynamoClient),
		ttl:         ttl,
	}, nil
}

// getCacheKey generates a unique cache key for a station and date
func getCacheKey(stationID string, date time.Time) string {
	return fmt.Sprintf("%s:%s", stationID, date.Format("2006-01-02"))
}

// GetPredictions tries to get predictions first from LRU cache, then from DynamoDB
func (c *CacheService) GetPredictions(ctx context.Context, stationID string, date time.Time) (*TidePredictionRecord, error) {
	key := getCacheKey(stationID, date)

	// Try LRU cache first
	if entry, ok := c.lru.Get(key); ok {
		if time.Now().Before(entry.ExpiresAt) {
			c.lruHits++
			log.Debug().
				Str("station_id", stationID).
				Str("date", date.Format("2006-01-02")).
				Uint64("lru_hits", c.lruHits).
				Msg("LRU cache hit")
			return entry.Data, nil
		}
		// Entry expired, remove it
		c.lru.Remove(key)
	}
	c.lruMisses++

	// Try DynamoDB cache
	record, err := c.dynamoCache.GetPredictions(ctx, stationID, date)
	if err != nil {
		return nil, fmt.Errorf("getting predictions from DynamoDB: %w", err)
	}

	if record != nil {
		c.dynamoHits++
		// Cache hit in DynamoDB, store in LRU cache
		c.lru.Add(key, &LRUCacheEntry{
			Data:      record,
			ExpiresAt: time.Now().Add(c.ttl),
		})
		log.Debug().
			Str("station_id", stationID).
			Str("date", date.Format("2006-01-02")).
			Uint64("dynamo_hits", c.dynamoHits).
			Msg("DynamoDB cache hit")
		return record, nil
	}
	c.dynamoMisses++

	log.Debug().
		Str("station_id", stationID).
		Time("date", date).
		Uint64("lru_misses", c.lruMisses).
		Uint64("dynamo_misses", c.dynamoMisses).
		Msg("Cache miss")
	return nil, nil
}

// SavePredictions saves predictions to both LRU and DynamoDB caches
func (c *CacheService) SavePredictions(ctx context.Context, record TidePredictionRecord) error {
	// Parse date string to generate cache key
	date, err := time.Parse("2006-01-02", record.Date)
	if err != nil {
		return fmt.Errorf("parsing date: %w", err)
	}

	key := getCacheKey(record.StationID, date)

	// Save to LRU cache
	c.lru.Add(key, &LRUCacheEntry{
		Data:      &record,
		ExpiresAt: time.Now().Add(c.ttl),
	})

	// Save to DynamoDB
	if err := c.dynamoCache.SavePredictions(ctx, record); err != nil {
		return fmt.Errorf("saving predictions to DynamoDB: %w", err)
	}

	return nil
}

// SavePredictionsBatch saves multiple predictions to both caches
func (c *CacheService) SavePredictionsBatch(ctx context.Context, records []TidePredictionRecord) error {
	// Save to LRU cache
	for _, record := range records {
		date, err := time.Parse("2006-01-02", record.Date)
		if err != nil {
			return fmt.Errorf("parsing date: %w", err)
		}

		key := getCacheKey(record.StationID, date)
		c.lru.Add(key, &LRUCacheEntry{
			Data:      &record,
			ExpiresAt: time.Now().Add(c.ttl),
		})
	}

	// Save to DynamoDB
	if err := c.dynamoCache.SavePredictionsBatch(ctx, records); err != nil {
		return fmt.Errorf("saving predictions batch to DynamoDB: %w", err)
	}

	return nil
}

// GetCacheStats returns statistics about cache hits and misses
func (c *CacheService) GetCacheStats() map[string]uint64 {
	return map[string]uint64{
		"lru_hits":      c.lruHits,
		"lru_misses":    c.lruMisses,
		"dynamo_hits":   c.dynamoHits,
		"dynamo_misses": c.dynamoMisses,
	}
}

// Clear removes all entries from the LRU cache
func (c *CacheService) Clear() {
	c.lru.Purge()
}
