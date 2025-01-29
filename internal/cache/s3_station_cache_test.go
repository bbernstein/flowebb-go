package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
	"time"
)

// mockS3Client implements a mock S3 client for testing
// Verify mockS3Client implements S3Client interface
var _ S3Client = (*mockS3Client)(nil)

type mockS3Client struct {
	getObjectFunc func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	putObjectFunc func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectFunc != nil {
		return m.getObjectFunc(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{}, nil
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectFunc != nil {
		return m.putObjectFunc(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

// mockClock implements clock interface for testing
type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

// Helper function to create a test S3StationCache with mocks
func createTestCache(s3Client *mockS3Client, clock clock) *S3StationCache {
	if clock == nil {
		clock = &mockClock{now: time.Now()}
	}
	return &S3StationCache{
		client:     s3Client,
		bucketName: "test-bucket",
		ttl:        24 * time.Hour,
		clock:      clock,
	}
}

// Helper function to create test stations
func createTestStations() []models.Station {
	return []models.Station{
		{
			ID:        "TEST001",
			Name:      "Test Station 1",
			Latitude:  47.6062,
			Longitude: -122.3321,
			Source:    models.SourceNOAA,
		},
		{
			ID:        "TEST002",
			Name:      "Test Station 2",
			Latitude:  47.6063,
			Longitude: -122.3322,
			Source:    models.SourceNOAA,
		},
	}
}

func TestGetStations(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*mockS3Client, *mockClock)
		want      []models.Station
		wantErr   bool
	}{
		{
			name: "successful retrieval of valid cache",
			setupMock: func(s3Client *mockS3Client, clock *mockClock) {
				now := time.Now()
				clock.now = now

				record := StationListCacheRecord{
					Stations:    createTestStations(),
					LastUpdated: now.Unix(),
					TTL:         now.Add(24 * time.Hour).Unix(),
				}

				recordBytes, _ := json.Marshal(record)
				s3Client.getObjectFunc = func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					return &s3.GetObjectOutput{
						Body: io.NopCloser(bytes.NewReader(recordBytes)),
					}, nil
				}
			},
			want:    createTestStations(),
			wantErr: false,
		},
		{
			name: "expired cache",
			setupMock: func(s3Client *mockS3Client, clock *mockClock) {
				now := time.Now()
				clock.now = now

				record := StationListCacheRecord{
					Stations:    createTestStations(),
					LastUpdated: now.Add(-48 * time.Hour).Unix(),
					TTL:         now.Add(-24 * time.Hour).Unix(),
				}

				recordBytes, _ := json.Marshal(record)
				s3Client.getObjectFunc = func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					return &s3.GetObjectOutput{
						Body: io.NopCloser(bytes.NewReader(recordBytes)),
					}, nil
				}
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "s3 error",
			setupMock: func(s3Client *mockS3Client, clock *mockClock) {
				s3Client.getObjectFunc = func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					return nil, &types.NoSuchKey{}
				}
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "invalid json",
			setupMock: func(s3Client *mockS3Client, clock *mockClock) {
				s3Client.getObjectFunc = func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					return &s3.GetObjectOutput{
						Body: io.NopCloser(strings.NewReader("invalid json")),
					}, nil
				}
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client and clock
			mockS3 := &mockS3Client{}
			mockClock := &mockClock{now: time.Now()}

			// Setup test-specific mock behavior
			tt.setupMock(mockS3, mockClock)

			// Create cache with mocks
			cache := createTestCache(mockS3, mockClock)

			// Test GetStations
			got, err := cache.GetStations(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSaveStations(t *testing.T) {
	tests := []struct {
		name      string
		stations  []models.Station
		setupMock func(*mockS3Client, *mockClock)
		wantErr   bool
	}{
		{
			name:     "successful save",
			stations: createTestStations(),
			setupMock: func(s3Client *mockS3Client, clock *mockClock) {
				s3Client.putObjectFunc = func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
					// Verify the bucket and key
					assert.Equal(t, "test-bucket", *params.Bucket)
					assert.Equal(t, cacheKey, *params.Key)

					// Verify the content
					body, _ := io.ReadAll(params.Body)
					var record StationListCacheRecord
					err := json.Unmarshal(body, &record)
					require.NoError(t, err)

					// Verify record content
					assert.Equal(t, clock.Now().Unix(), record.LastUpdated)
					assert.Equal(t, clock.Now().Add(24*time.Hour).Unix(), record.TTL)
					assert.Equal(t, 2, len(record.Stations))

					return &s3.PutObjectOutput{}, nil
				}
			},
			wantErr: false,
		},
		{
			name:     "s3 error",
			stations: createTestStations(),
			setupMock: func(s3Client *mockS3Client, clock *mockClock) {
				s3Client.putObjectFunc = func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
					return nil, &types.NoSuchBucket{}
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client and clock
			mockS3 := &mockS3Client{}
			mockClock := &mockClock{now: time.Now()}

			// Setup test-specific mock behavior
			tt.setupMock(mockS3, mockClock)

			// Create cache with mocks
			cache := createTestCache(mockS3, mockClock)

			// Test SaveStations
			err := cache.SaveStations(context.Background(), tt.stations)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestS3StationCache_Integration(t *testing.T) {
	// Create test data
	stations := createTestStations()
	now := time.Now()
	clock := &mockClock{now: now}

	// Create a mock S3 client that tracks the saved data
	var savedData []byte
	mockS3 := &mockS3Client{
		putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			data, err := io.ReadAll(params.Body)
			if err != nil {
				return nil, err
			}
			savedData = data
			return &s3.PutObjectOutput{}, nil
		},
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			if savedData == nil {
				return nil, &types.NoSuchKey{}
			}
			return &s3.GetObjectOutput{
				Body: io.NopCloser(bytes.NewReader(savedData)),
			}, nil
		},
	}

	// Create cache with mocks
	cache := createTestCache(mockS3, clock)

	// Test full save and retrieve cycle
	ctx := context.Background()

	// First, save the stations
	err := cache.SaveStations(ctx, stations)
	require.NoError(t, err)

	// Then retrieve them
	retrievedStations, err := cache.GetStations(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrievedStations)

	// Verify the retrieved data matches what we saved
	assert.Equal(t, stations, retrievedStations)

	// Advance clock past TTL
	clock.now = now.Add(25 * time.Hour)

	// Verify cache is now expired
	expiredStations, err := cache.GetStations(ctx)
	require.NoError(t, err)
	assert.Nil(t, expiredStations)
}

func TestS3StationCache_BucketValidation(t *testing.T) {
	mockS3 := &mockS3Client{}
	mockClock := &mockClock{now: time.Now()}

	// Test with empty bucket name
	cache := &S3StationCache{
		client:     mockS3,
		bucketName: "",
		ttl:        24 * time.Hour,
		clock:      mockClock,
	}

	// Test SaveStations
	err := cache.SaveStations(context.Background(), createTestStations())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty bucket name")

	// Test GetStations
	stations, err := cache.GetStations(context.Background())
	assert.Error(t, err)
	assert.Nil(t, stations)
	assert.Contains(t, err.Error(), "empty bucket name")
}

func TestS3StationCache_ObjectKeyHandling(t *testing.T) {
	mockS3 := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			assert.Equal(t, cacheKey, aws.ToString(params.Key))
			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader("{}")),
			}, nil
		},
		putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			assert.Equal(t, cacheKey, aws.ToString(params.Key))
			return &s3.PutObjectOutput{}, nil
		},
	}

	cache := createTestCache(mockS3, nil)

	// Test that both operations use the correct cache key
	_, _ = cache.GetStations(context.Background())
	_ = cache.SaveStations(context.Background(), createTestStations())
}
