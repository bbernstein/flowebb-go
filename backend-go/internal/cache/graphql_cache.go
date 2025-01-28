package cache

import (
	"context"
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"github.com/hashicorp/golang-lru/v2"
	"sync"
	"time"
)

type GraphQLCacheEntry struct {
	Data      string // Store the actual query string
	ExpiresAt time.Time
}

type GraphQLCache struct {
	lru   *lru.Cache[string, *GraphQLCacheEntry]
	ttl   time.Duration
	clock clock
	mu    sync.RWMutex
}

func NewGraphQLCache(cfg *config.CacheConfig) (*GraphQLCache, error) {
	lruCache, err := lru.New[string, *GraphQLCacheEntry](cfg.GraphQLLRUSize)
	if err != nil {
		return nil, err
	}

	return &GraphQLCache{
		lru:   lruCache,
		ttl:   cfg.GetGraphQLLRUTTL(),
		clock: &systemClock{},
	}, nil
}

func (c *GraphQLCache) Add(_ context.Context, key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if queryStr, ok := value.(string); ok {
		c.lru.Add(key, &GraphQLCacheEntry{
			Data:      queryStr,
			ExpiresAt: c.clock.Now().Add(c.ttl),
		})
	}
}

func (c *GraphQLCache) Get(_ context.Context, key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.lru.Get(key)
	if !ok {
		return nil, false
	}

	if c.clock.Now().After(entry.ExpiresAt) {
		c.lru.Remove(key)
		return nil, false
	}

	return entry.Data, true
}

func (c *GraphQLCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Purge()
}
