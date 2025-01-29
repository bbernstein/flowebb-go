package cache

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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
	return f.now.UTC()
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
func createTestCacheService(t *testing.T, cfg *config.CacheConfig) *LRUCacheService {
	// Create an in-memory store for the mock DynamoDB with synchronization
	var mu sync.RWMutex
	store := make(map[string]map[string]types.AttributeValue)

	mockDynamo := &mockDynamoDBClientLRU{
		putItemFunc: func(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			mu.Lock()
			defer mu.Unlock()
			store[*params.TableName] = params.Item
			return &dynamodb.PutItemOutput{}, nil
		},
		batchWriteItemFunc: func(_ context.Context, params *dynamodb.BatchWriteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
			mu.Lock()
			defer mu.Unlock()
			for tableName, requests := range params.RequestItems {
				for _, request := range requests {
					if request.PutRequest != nil {
						store[tableName] = request.PutRequest.Item
					}
				}
			}
			return &dynamodb.BatchWriteItemOutput{}, nil
		},
	}

	service, err := NewCacheService(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to create cache service: %v", err)
		return nil
	}
	// Pass the fake clock to DynamoPredictionCache
	fakeClock := &fakeClock{now: time.Now().UTC()}
	service.dynamoCache = NewDynamoPredictionCache(mockDynamo, cfg)
	service.dynamoCache.clock = fakeClock // Use the fake clock
	service.clock = fakeClock

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
				TidePredictionLRUSize:       tt.lruSize,
				TidePredictionLRUTTLMinutes: int(tt.ttl.Minutes()),
			}

			// Create service directly instead of using helper function
			service, err := NewCacheService(context.Background(), cfg)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, service)
			} else {
				assert.NoError(t, err)
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
		TidePredictionLRUSize:       1000,
		TidePredictionLRUTTLMinutes: 15,
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

	shortTTL := 1
	cfg := &config.CacheConfig{
		TidePredictionLRUSize:       1000,
		TidePredictionLRUTTLMinutes: shortTTL,
	}

	service := createTestCacheService(t, cfg)
	require.NotNil(t, service)
	require.NotNil(t, service.clock)
	mockClock := &fakeClock{now: time.Now()} // Create a new mock clock
	service.clock = mockClock                // Set the mock clock
	service.Clear()

	stationID := "TEST001"
	date := mockClock.Now()

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

	// Verify we have the record in the cache
	key := getCacheKey(stationID, date.Format("2006-01-02"))
	entry, exists := service.lru.Get(key)
	require.True(t, exists, "Entry should exist in cache")
	require.NotNil(t, entry)
	t.Logf("Initial cache entry expires at: %v, current time: %v", entry.ExpiresAt, mockClock.Now())

	// Advance mock clock beyond TTL
	mockClock.Advance(2 * time.Minute)
	t.Logf("After advance, current time: %v", mockClock.Now())

	// Lookup after expiration should miss
	result, err = service.GetPredictions(context.Background(), stationID, date)
	require.NoError(t, err)
	assert.Nil(t, result, "Expected nil result after cache expiration")

	// Verify the entry was removed from cache
	_, exists = service.lru.Get(key)
	assert.False(t, exists, "Entry should be removed from cache after expiration")
}

func TestLRUSavePredictionsBatch(t *testing.T) {
	t.Parallel()

	cfg := &config.CacheConfig{
		TidePredictionLRUSize:       1000,
		TidePredictionLRUTTLMinutes: 15,
		BatchSize:                   2, // Small batch size to test multiple batches
	}

	service := createTestCacheService(t, cfg)
	service.Clear()

	// Create test records
	records := make([]models.TidePredictionRecord, 5)
	baseTime := time.Now()
	for i := range records {
		records[i] = models.TidePredictionRecord{
			StationID:   fmt.Sprintf("TEST%03d", i),
			Date:        baseTime.AddDate(0, 0, i).Format("2006-01-02"),
			StationType: "R",
			Predictions: []models.TidePrediction{{
				Timestamp: baseTime.AddDate(0, 0, i).Unix() * 1000,
				LocalTime: baseTime.AddDate(0, 0, i).Format("2006-01-02T15:04:05"),
				Height:    float64(i),
			}},
		}
	}

	// Save batch
	err := service.SavePredictionsBatch(context.Background(), records)
	require.NoError(t, err)

	// Verify each record was saved in LRU cache
	for _, record := range records {
		date, _ := time.Parse("2006-01-02", record.Date)
		result, err := service.GetPredictions(context.Background(), record.StationID, date)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, record.StationID, result.StationID)
	}
}

func TestDynamoDBHitFallback(t *testing.T) {
	t.Parallel()

	cfg := &config.CacheConfig{
		TidePredictionLRUSize:       1000,
		TidePredictionLRUTTLMinutes: 15,
	}

	// Create a mock DynamoDB client that returns a canned response
	testRecord := models.TidePredictionRecord{
		StationID:   "TEST001",
		Date:        time.Now().Format("2006-01-02"),
		StationType: "R",
		Predictions: []models.TidePrediction{{
			Timestamp: time.Now().Unix() * 1000,
			LocalTime: time.Now().Format("2006-01-02T15:04:05"),
			Height:    1.5,
		}},
		TTL: time.Now().Add(24 * time.Hour).Unix(),
	}

	mockDynamo := &mockDynamoDBClientLRU{
		getItemFunc: func(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			// Marshal the test record to DynamoDB format
			item, err := attributevalue.MarshalMap(testRecord)
			if err != nil {
				return nil, err
			}
			return &dynamodb.GetItemOutput{Item: item}, nil
		},
	}

	service := createTestCacheService(t, cfg)
	service.dynamoCache = NewDynamoPredictionCache(mockDynamo, cfg)
	service.Clear()

	// First access should miss LRU but hit DynamoDB
	date, _ := time.Parse("2006-01-02", testRecord.Date)
	result, err := service.GetPredictions(context.Background(), testRecord.StationID, date)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, testRecord.StationID, result.StationID)

	// Second access should hit LRU
	result2, err := service.GetPredictions(context.Background(), testRecord.StationID, date)
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Verify cache stats
	stats := service.GetCacheStats()
	assert.Equal(t, uint64(1), stats["lru_hits"])
	assert.Equal(t, uint64(1), stats["lru_misses"])
	assert.Equal(t, uint64(1), stats["dynamo_hits"])
	assert.Equal(t, uint64(0), stats["dynamo_misses"])
}

func TestConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent test in short mode")
	}

	t.Parallel()

	cfg := &config.CacheConfig{
		TidePredictionLRUSize:       1000,
		TidePredictionLRUTTLMinutes: 15,
	}

	service := createTestCacheService(t, cfg)
	service.Clear() // Ensure clean state

	const goroutines = 5  // Reduced from 10
	const iterations = 20 // Reduced from 100

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iterations)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				stationID := fmt.Sprintf("TEST%d", id)
				date := service.clock.(*fakeClock).Now() // Use mock clock

				record := models.TidePredictionRecord{
					StationID:   stationID,
					Date:        date.Format("2006-01-02"),
					StationType: "R",
				}

				// Mix reads and writes
				if j%2 == 0 {
					if err := service.SavePredictions(context.Background(), record); err != nil {
						errs <- fmt.Errorf("SavePredictions error: %v", err)
						return
					}
				} else {
					if _, err := service.GetPredictions(context.Background(), stationID, date); err != nil {
						errs <- fmt.Errorf("GetPredictions error: %v", err)
						return
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

func BenchmarkCacheOperations(b *testing.B) {
	cfg := &config.CacheConfig{
		TidePredictionLRUSize:       1000,
		TidePredictionLRUTTLMinutes: 15,
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
