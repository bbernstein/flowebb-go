package cache

import (
	"context"
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testConfig = config.GetCacheConfig() // Using default config for tests

// mockDynamoDBClient3 implements a mock DynamoDB client for testing
type mockDynamoDBClient struct {
	getItemFunc        func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	putItemFunc        func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	batchWriteItemFunc func(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	listTablesFunc     func(ctx context.Context, params *dynamodb.ListTablesInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error)
}

func (m *mockDynamoDBClient) ListTables(ctx context.Context, params *dynamodb.ListTablesInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error) {
	if m.listTablesFunc != nil {
		return m.listTablesFunc(ctx, params, optFns...)
	}
	return &dynamodb.ListTablesOutput{}, nil
}

func (m *mockDynamoDBClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if m.getItemFunc != nil {
		return m.getItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (m *mockDynamoDBClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if m.putItemFunc != nil {
		return m.putItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockDynamoDBClient) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	if m.batchWriteItemFunc != nil {
		return m.batchWriteItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.BatchWriteItemOutput{}, nil
}

func createTestPredictionRecord() models.TidePredictionRecord {
	now := time.Now()
	return models.TidePredictionRecord{
		StationID:   "TEST-001",
		Date:        now.Format("2006-01-02"),
		StationType: "R",
		Predictions: []models.TidePrediction{
			{
				Timestamp: now.UnixMilli(),
				LocalTime: now.Format("2006-01-02T15:04:05"),
				Height:    1.5,
			},
		},
		Extremes: []models.TideExtreme{
			{
				Type:      models.TideTypeHigh,
				Timestamp: now.UnixMilli(),
				LocalTime: now.Format("2006-01-02T15:04:05"),
				Height:    2.0,
			},
		},
		LastUpdated: now.Unix(),
		TTL:         now.Add(24 * time.Hour).Unix(),
	}
}

func TestGetPredictions(t *testing.T) {
	tests := []struct {
		name       string
		stationID  string
		date       time.Time
		mockSetup  func() *mockDynamoDBClient
		wantRecord *models.TidePredictionRecord
		wantErr    bool
	}{
		{
			name:      "successful retrieval",
			stationID: "TEST-001",
			date:      time.Now(),
			mockSetup: func() *mockDynamoDBClient {
				record := createTestPredictionRecord()
				item, _ := attributevalue.MarshalMap(record)
				return &mockDynamoDBClient{
					getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
						return &dynamodb.GetItemOutput{
							Item: item,
						}, nil
					},
				}
			},
			wantRecord: &models.TidePredictionRecord{},
			wantErr:    false,
		},
		{
			name:      "record not found",
			stationID: "TEST-002",
			date:      time.Now(),
			mockSetup: func() *mockDynamoDBClient {
				return &mockDynamoDBClient{
					getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
						return &dynamodb.GetItemOutput{
							Item: nil,
						}, nil
					},
				}
			},
			wantRecord: nil,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewDynamoPredictionCache(tt.mockSetup(), testConfig)
			got, err := cache.GetPredictions(context.Background(), tt.stationID, tt.date)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.wantRecord == nil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
			}
		})
	}
}

func TestSavePredictions(t *testing.T) {
	tests := []struct {
		name      string
		record    models.TidePredictionRecord
		mockSetup func() *mockDynamoDBClient
		wantErr   bool
	}{
		{
			name:   "successful save",
			record: createTestPredictionRecord(),
			mockSetup: func() *mockDynamoDBClient {
				return &mockDynamoDBClient{
					putItemFunc: func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
						return &dynamodb.PutItemOutput{}, nil
					},
				}
			},
			wantErr: false,
		},
		{
			name: "invalid record",
			record: models.TidePredictionRecord{
				StationID: "", // Invalid: empty station ID
			},
			mockSetup: func() *mockDynamoDBClient {
				return &mockDynamoDBClient{}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewDynamoPredictionCache(tt.mockSetup(), testConfig)
			err := cache.SavePredictions(context.Background(), tt.record)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestSavePredictionsBatch(t *testing.T) {
	tests := []struct {
		name      string
		records   []models.TidePredictionRecord
		mockSetup func() *mockDynamoDBClient
		wantErr   bool
	}{
		{
			name: "successful batch save",
			records: []models.TidePredictionRecord{
				createTestPredictionRecord(),
				createTestPredictionRecord(),
			},
			mockSetup: func() *mockDynamoDBClient {
				return &mockDynamoDBClient{
					batchWriteItemFunc: func(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
						return &dynamodb.BatchWriteItemOutput{}, nil
					},
				}
			},
			wantErr: false,
		},
		{
			name: "batch with invalid record",
			records: []models.TidePredictionRecord{
				createTestPredictionRecord(),
				{StationID: ""}, // Invalid record
			},
			mockSetup: func() *mockDynamoDBClient {
				return &mockDynamoDBClient{}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewDynamoPredictionCache(tt.mockSetup(), testConfig)
			err := cache.SavePredictionsBatch(context.Background(), tt.records)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestCacheValidation(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		record    models.TidePredictionRecord
		wantValid bool
	}{
		{
			name: "valid record",
			record: models.TidePredictionRecord{
				TTL: now.Add(24 * time.Hour).Unix(),
			},
			wantValid: true,
		},
		{
			name: "expired record",
			record: models.TidePredictionRecord{
				TTL: now.Add(-24 * time.Hour).Unix(),
			},
			wantValid: false,
		},
		{
			name: "about to expire",
			record: models.TidePredictionRecord{
				TTL: now.Add(1 * time.Minute).Unix(),
			},
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewDynamoPredictionCache(&mockDynamoDBClient{}, testConfig)
			isValid := cache.isValid(tt.record)
			assert.Equal(t, tt.wantValid, isValid)
		})
	}
}
