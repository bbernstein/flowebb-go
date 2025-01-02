package cache

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"sync"
	"testing"
	"time"

	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock implements a mock time source for testing
type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}

type mockDynamoDBClientLRU struct {
	getItemFunc        func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	putItemFunc        func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	batchWriteItemFunc func(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	listTablesFunc     func(ctx context.Context, params *dynamodb.ListTablesInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error)
}

func (m *mockDynamoDBClientLRU) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if m.getItemFunc != nil {
		return m.getItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (m *mockDynamoDBClientLRU) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if m.putItemFunc != nil {
		return m.putItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockDynamoDBClientLRU) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	if m.batchWriteItemFunc != nil {
		return m.batchWriteItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.BatchWriteItemOutput{}, nil
}

func (m *mockDynamoDBClientLRU) ListTables(ctx context.Context, params *dynamodb.ListTablesInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error) {
	if m.listTablesFunc != nil {
		return m.listTablesFunc(ctx, params, optFns...)
	}
	return &dynamodb.ListTablesOutput{}, nil
}

// Helper function to create test cache service with mock DynamoDB client
func createTestCacheService(_ *testing.T, cfg *config.CacheConfig) *LRUCacheService {
	mockDynamo := &mockDynamoDBClientLRU{
		getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
		putItemFunc: func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	service, err := NewCacheService(context.Background(), cfg)
	if err != nil {
		return nil
	}
	if service != nil {
		service.dynamoCache = NewDynamoPredictionCache(mockDynamo, cfg)
	}
	return service
}

func TestNewCacheService(t *testing.T) {
	tests := []struct {
		name      string
		lruSize   int
		ttl       time.Duration
		wantError bool
	}{
		{
			name:      "valid configuration",
			lruSize:   1000,
			ttl:       15 * time.Minute,
			wantError: false,
		},
		{
			name:      "zero size",
			lruSize:   0,
			ttl:       15 * time.Minute,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.CacheConfig{
				LRUSize:       tt.lruSize,
				LRUTTLMinutes: int(tt.ttl.Minutes()),
			}

			service := createTestCacheService(t, cfg)

			if tt.wantError {
				assert.Nil(t, service)
			} else {
				assert.NotNil(t, service)
				assert.NotNil(t, service.lru)
				assert.NotNil(t, service.dynamoCache)
			}
		})
	}
}

func TestCacheHitAndMiss(t *testing.T) {
	t.Parallel()

	cfg := &config.CacheConfig{
		LRUSize:       1000,
		LRUTTLMinutes: 15,
	}

	service := createTestCacheService(t, cfg)
	service.Clear() // Ensure clean state

	stationID := "TEST001"
	date := time.Now()

	testRecord := models.TidePredictionRecord{
		StationID:   stationID,
		Date:        date.Format("2006-01-02"),
		StationType: "R",
		Predictions: []models.TidePrediction{
			{
				Timestamp: date.Unix() * 1000,
				LocalTime: date.Format("2006-01-02T15:04:05"),
				Height:    1.5,
			},
		},
	}

	// Test cache miss
	result, err := service.GetPredictions(context.Background(), stationID, date)
	require.NoError(t, err)
	assert.Nil(t, result)

	// Save to cache
	err = service.SavePredictions(context.Background(), testRecord)
	require.NoError(t, err)

	// Test cache hit
	result, err = service.GetPredictions(context.Background(), stationID, date)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, testRecord.StationID, result.StationID)
	assert.Equal(t, testRecord.Date, result.Date)

	// Verify cache stats
	stats := service.GetCacheStats()
	assert.Equal(t, uint64(1), stats["lru_hits"])
	assert.Equal(t, uint64(1), stats["lru_misses"])
}

func TestCacheExpiration(t *testing.T) {
	t.Parallel()

	shortTTL := 1 * time.Minute
	cfg := &config.CacheConfig{
		LRUSize:       1000,
		LRUTTLMinutes: int(shortTTL.Minutes()),
	}

	clock := &fakeClock{now: time.Now()}
	service := createTestCacheService(t, cfg)
	service.clock = clock
	service.Clear()

	stationID := "TEST001"
	date := clock.Now()

	testRecord := models.TidePredictionRecord{
		StationID:   stationID,
		Date:        date.Format("2006-01-02"),
		StationType: "R",
	}

	// Save to cache
	err := service.SavePredictions(context.Background(), testRecord)
	require.NoError(t, err)

	// Immediate lookup should succeed
	result, err := service.GetPredictions(context.Background(), stationID, date)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Advance clock beyond TTL
	clock.Advance(2 * time.Minute)

	// Lookup after expiration should miss
	result, err = service.GetPredictions(context.Background(), stationID, date)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent test in short mode")
	}

	t.Parallel()

	cfg := &config.CacheConfig{
		LRUSize:       1000,
		LRUTTLMinutes: 15,
	}

	service := createTestCacheService(t, cfg)
	service.Clear() // Ensure clean state

	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				stationID := fmt.Sprintf("TEST%d", id)
				date := time.Now()

				record := models.TidePredictionRecord{
					StationID:   stationID,
					Date:        date.Format("2006-01-02"),
					StationType: "R",
				}

				// Mix reads and writes
				if j%2 == 0 {
					err := service.SavePredictions(context.Background(), record)
					assert.NoError(t, err)
				} else {
					_, err := service.GetPredictions(context.Background(), stationID, date)
					assert.NoError(t, err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// Benchmark basic cache operations
func BenchmarkCacheOperations(b *testing.B) {
	cfg := &config.CacheConfig{
		LRUSize:       1000,
		LRUTTLMinutes: 15,
	}

	service := createTestCacheService(nil, cfg)
	service.Clear() // Ensure clean state

	stationID := "TEST001"
	date := time.Now()

	testRecord := models.TidePredictionRecord{
		StationID:   stationID,
		Date:        date.Format("2006-01-02"),
		StationType: "R",
	}

	b.Run("SavePredictions", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := service.SavePredictions(context.Background(), testRecord)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GetPredictions", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := service.GetPredictions(context.Background(), stationID, date)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
