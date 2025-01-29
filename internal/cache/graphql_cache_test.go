package cache

import (
	"context"
	"fmt"
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestLRUCacheService_GraphQLCacheOperations(t *testing.T) {
	cfg := &config.CacheConfig{
		GraphQLLRUSize:       1000,
		GraphQLLRUTTLMinutes: 15,
	}
	cache, err := NewGraphQLCache(cfg)
	require.NoError(t, err)

	tests := []struct {
		name    string
		key     string
		value   interface{}
		wantGet interface{}
		wantHit bool
	}{
		{
			name:    "cache prediction record",
			key:     "query1",
			value:   `query { user(id: "123") { name email } }`,
			wantGet: `query { user(id: "123") { name email } }`,
			wantHit: true,
		},
		{
			name:    "cache invalid type",
			key:     "query2",
			value:   123, // numbers aren't valid GraphQL responses
			wantGet: nil,
			wantHit: false,
		},
		{
			name:    "get non-existent key",
			key:     "nonexistent",
			value:   nil,
			wantGet: nil,
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != nil {
				cache.Add(context.Background(), tt.key, tt.value)
			}

			got, hit := cache.Get(context.Background(), tt.key)
			assert.Equal(t, tt.wantHit, hit)

			if hit {
				assert.Equal(t, tt.wantGet, got)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}

func TestLRUCacheService_GraphQLCacheExpiration(t *testing.T) {
	shortTTL := 1
	cfg := &config.CacheConfig{
		GraphQLLRUSize:       1000,
		GraphQLLRUTTLMinutes: shortTTL,
	}

	cache, err := NewGraphQLCache(cfg)
	require.NoError(t, err)

	mockClock := &mockClock{now: time.Now()}
	cache.clock = mockClock

	// Add item to cache
	key := "test-key"
	value := `query { user(id: "123") { name email } }`
	cache.Add(context.Background(), key, value)

	// Verify item is in cache
	got, hit := cache.Get(context.Background(), key)
	require.True(t, hit)
	assert.Equal(t, value, got)

	// Advance clock past TTL
	mockClock.now = mockClock.now.Add(time.Duration(shortTTL) * time.Minute * 2)

	// Verify item is expired
	got, hit = cache.Get(context.Background(), key)
	assert.False(t, hit)
	assert.Nil(t, got)
}

func TestLRUCacheService_ConcurrentGraphQLAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent test in short mode")
	}

	cfg := &config.CacheConfig{
		GraphQLLRUSize:       1000,
		GraphQLLRUTTLMinutes: 15,
	}

	cache, err := NewGraphQLCache(cfg)
	require.NoError(t, err)

	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := &models.TidePredictionRecord{StationID: fmt.Sprintf("TEST%d", id)}

				if j%2 == 0 {
					cache.Add(context.Background(), key, value)
				} else {
					got, _ := cache.Get(context.Background(), key)
					if got != nil {
						assert.Equal(t, value.StationID, got.(*models.TidePredictionRecord).StationID)
					}
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestLRUCacheService_GraphQLCacheClear(t *testing.T) {
	cfg := &config.CacheConfig{
		GraphQLLRUSize:       1000,
		GraphQLLRUTTLMinutes: 15,
	}
	cache, err := NewGraphQLCache(cfg)
	require.NoError(t, err)

	// Add items to cache
	cache.Add(context.Background(), "key1", `query { user(id: "123") { name email } }`)
	cache.Add(context.Background(), "key2", `query { user(id: "456") { name email } }`)

	// Verify items are in cache
	_, hit1 := cache.Get(context.Background(), "key1")
	require.True(t, hit1)
	_, hit2 := cache.Get(context.Background(), "key2")
	require.True(t, hit2)

	// Clear the cache
	cache.Clear()

	// Verify cache is empty
	_, hit1 = cache.Get(context.Background(), "key1")
	assert.False(t, hit1)
	_, hit2 = cache.Get(context.Background(), "key2")
	assert.False(t, hit2)
}
