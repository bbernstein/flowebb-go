package cache

import (
	"context"
	"fmt"
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/hashicorp/golang-lru/v2"
	"sync"
	"time"
)

type clock interface {
	Now() time.Time
}

type systemClock struct{}

func (s *systemClock) Now() time.Time {
	return time.Now()
}

// LRUCacheEntry wraps the cached data with metadata
type LRUCacheEntry struct {
	Data      *models.TidePredictionRecord
	ExpiresAt time.Time
}

type CacheService interface {
	GetPredictions(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error)
	SavePredictionsBatch(ctx context.Context, records []models.TidePredictionRecord) error
}

// LRUCacheService provides a two-layer caching system using LRU and DynamoDB
type LRUCacheService struct {
	lru          *lru.Cache[string, *LRUCacheEntry]
	dynamoCache  *DynamoPredictionCache
	ttl          time.Duration
	clock        clock
	statsMutex   sync.RWMutex
	lruHits      uint64
	lruMisses    uint64
	dynamoHits   uint64
	dynamoMisses uint64
}

// NewCacheService creates a new cache service with both LRU and DynamoDB caching
func NewCacheService(ctx context.Context, config *config.CacheConfig) (*LRUCacheService, error) {
	lruCache, err := lru.New[string, *LRUCacheEntry](config.LRUSize)
	if err != nil {
		return nil, fmt.Errorf("creating LRU cache: %w", err)
	}

	dynamoClient, err := NewDynamoClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating DynamoDB client: %w", err)
	}

	return &LRUCacheService{
		lru:         lruCache,
		dynamoCache: NewDynamoPredictionCache(dynamoClient, config),
		ttl:         config.GetLRUTTL(),
		clock:       &systemClock{},
	}, nil
}

// getCacheKey generates a unique cache key for a station and date string
func getCacheKey(stationID string, date string) string {
	return fmt.Sprintf("%s:%s", stationID, date)
}

// GetPredictions tries to get predictions first from LRU cache, then from DynamoDB
func (c *LRUCacheService) GetPredictions(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
	// Try LRU cache
	key := getCacheKey(stationID, date.Format("2006-01-02"))
	if entry, ok := c.lru.Get(key); ok {
		if entry.ExpiresAt.After(c.clock.Now()) {
			c.incrementLRUHits()
			return entry.Data, nil
		} else {
			c.lru.Remove(key)
		}
	}

	c.incrementLRUMisses()

	// Try DynamoDB cache
	record, err := c.dynamoCache.GetPredictions(ctx, stationID, date)
	if err != nil {
		return nil, fmt.Errorf("getting predictions from DynamoDB: %w", err)
	}

	if record != nil {
		c.incrementDynamoHits()
		// Save to LRU cache
		if err := c.SavePredictions(ctx, *record); err != nil {
			return nil, fmt.Errorf("saving predictions to LRU cache: %w", err)
		}
		return record, nil
	}
	c.incrementDynamoMisses()

	return nil, nil
}

// SavePredictions saves predictions to both LRU and DynamoDB caches
func (c *LRUCacheService) SavePredictions(ctx context.Context, record models.TidePredictionRecord) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("invalid prediction record: %w", err)
	}

	key := getCacheKey(record.StationID, record.Date)

	// Save to LRU cache
	c.lru.Add(key, &LRUCacheEntry{
		Data:      &record,
		ExpiresAt: c.clock.Now().Truncate(time.Second).Add(c.ttl),
	})

	// Save to DynamoDB
	if err := c.dynamoCache.SavePredictions(ctx, record); err != nil {
		return fmt.Errorf("saving predictions to DynamoDB: %w", err)
	}

	return nil
}

// SavePredictionsBatch saves multiple predictions to both caches
func (c *LRUCacheService) SavePredictionsBatch(ctx context.Context, records []models.TidePredictionRecord) error {
	// Save to LRU cache
	for _, record := range records {
		// Create a copy of the record
		recordCopy := record // Make a copy of the record

		key := getCacheKey(recordCopy.StationID, recordCopy.Date)
		c.lru.Add(key, &LRUCacheEntry{
			Data:      &recordCopy,
			ExpiresAt: c.clock.Now().Truncate(time.Second).Add(c.ttl),
		})
	}

	// Save to DynamoDB
	if err := c.dynamoCache.SavePredictionsBatch(ctx, records); err != nil {
		return fmt.Errorf("saving predictions batch to DynamoDB: %w", err)
	}

	return nil
}

// GetCacheStats returns statistics about cache hits and misses
func (c *LRUCacheService) GetCacheStats() map[string]uint64 {
	c.statsMutex.RLock()
	defer c.statsMutex.RUnlock()

	return map[string]uint64{
		"lru_hits":      c.lruHits,
		"lru_misses":    c.lruMisses,
		"dynamo_hits":   c.dynamoHits,
		"dynamo_misses": c.dynamoMisses,
	}
}

// Clear removes all entries from the LRU cache
func (c *LRUCacheService) Clear() {
	c.lru.Purge()
}

func (c *LRUCacheService) incrementLRUHits() {
	c.statsMutex.Lock()
	c.lruHits++
	c.statsMutex.Unlock()
}

func (c *LRUCacheService) incrementLRUMisses() {
	c.statsMutex.Lock()
	c.lruMisses++
	c.statsMutex.Unlock()
}

func (c *LRUCacheService) incrementDynamoHits() {
	c.statsMutex.Lock()
	c.dynamoHits++
	c.statsMutex.Unlock()
}

func (c *LRUCacheService) incrementDynamoMisses() {
	c.statsMutex.Lock()
	c.dynamoMisses++
	c.statsMutex.Unlock()
}
