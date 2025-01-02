package cache

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/bbernstein/flowebb/backend-go/internal/config"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/rs/zerolog/log"
	"time"
)

const (
	tableName         = "tide-predictions-cache"
	cacheValidityDays = 7
)

// DynamoPredictionCache handles caching tide predictions in DynamoDB
type DynamoPredictionCache struct {
	client DynamoDBClient
	config *config.CacheConfig
}

func NewDynamoPredictionCache(client DynamoDBClient, cacheConfig *config.CacheConfig) *DynamoPredictionCache {
	if cacheConfig == nil {
		cacheConfig = config.GetCacheConfig()
	}
	return &DynamoPredictionCache{
		client: client,
		config: cacheConfig,
	}
}

// GetPredictions retrieves cached predictions for a station and date
func (c *DynamoPredictionCache) GetPredictions(ctx context.Context, stationID string, date time.Time) (*models.TidePredictionRecord, error) {
	dateStr := date.Format("2006-01-02")

	input := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"stationId": &types.AttributeValueMemberS{Value: stationID},
			"date":      &types.AttributeValueMemberS{Value: dateStr},
		},
	}

	result, err := c.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("getting predictions from DynamoDB: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var record models.TidePredictionRecord
	if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		return nil, fmt.Errorf("unmarshaling prediction record: %w", err)
	}

	// Check if cache is valid
	if !c.isValid(record) {
		log.Debug().
			Str("station_id", stationID).
			Str("date", dateStr).
			Msg("Cache expired")
		return nil, nil
	}

	return &record, nil
}

// SavePredictions saves predictions to the cache
func (c *DynamoPredictionCache) SavePredictions(ctx context.Context, record models.TidePredictionRecord) error {
	// Validate the record first
	if err := record.Validate(); err != nil {
		return fmt.Errorf("invalid prediction record: %w", err)
	}

	now := time.Now().Unix()
	record.LastUpdated = now
	record.TTL = now + (cacheValidityDays * 24 * 60 * 60)

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("marshaling prediction record: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	}

	if _, err := c.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("putting predictions in DynamoDB: %w", err)
	}

	log.Debug().
		Str("station_id", record.StationID).
		Str("date", record.Date).
		Msg("Saved predictions to cache")

	return nil
}

// SavePredictionsBatch saves multiple prediction records to the cache
func (c *DynamoPredictionCache) SavePredictionsBatch(ctx context.Context, records []models.TidePredictionRecord) error {
	// Validate all records first
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("invalid prediction record: %w", err)
		}
	}

	// Process in batches using configured batch size
	batchSize := c.config.BatchSize
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}

		batch := records[i:end]
		var writeRequests []types.WriteRequest

		for _, record := range batch {
			now := time.Now().Unix()
			record.LastUpdated = now
			// Use configured TTL
			record.TTL = now + int64(c.config.GetDynamoTTL().Seconds())

			item, err := attributevalue.MarshalMap(record)
			if err != nil {
				return fmt.Errorf("marshaling prediction record: %w", err)
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: item,
				},
			})
		}

		// Add retry logic with configured max retries
		var lastErr error
		for retry := 0; retry < c.config.MaxBatchRetries; retry++ {
			input := &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{
					tableName: writeRequests,
				},
			}

			if _, err := c.client.BatchWriteItem(ctx, input); err != nil {
				lastErr = err
				// Add exponential backoff
				time.Sleep(time.Duration(1<<retry) * 100 * time.Millisecond)
				continue
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			return fmt.Errorf("batch writing predictions after %d retries: %w",
				c.config.MaxBatchRetries, lastErr)
		}
	}

	return nil
}

func (c *DynamoPredictionCache) isValid(record models.TidePredictionRecord) bool {
	now := time.Now().Unix()
	return now < record.TTL
}
