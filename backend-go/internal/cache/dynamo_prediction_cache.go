package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/rs/zerolog/log"
)

const (
	tableName         = "tide-predictions-cache"
	cacheValidityDays = 7
)

// TidePredictionRecord represents a cached set of predictions for a station and date
type TidePredictionRecord struct {
	StationID   string                  `dynamodbav:"stationId"`   // Hash key
	Date        string                  `dynamodbav:"date"`        // Range key
	StationType string                  `dynamodbav:"stationType"` // R for reference, S for subordinate
	Predictions []models.TidePrediction `dynamodbav:"predictions"`
	Extremes    []models.TideExtreme    `dynamodbav:"extremes"`
	LastUpdated int64                   `dynamodbav:"lastUpdated"`
	TTL         int64                   `dynamodbav:"ttl"`
}

// DynamoPredictionCache handles caching tide predictions in DynamoDB
type DynamoPredictionCache struct {
	client *dynamodb.Client
}

func NewDynamoPredictionCache(client *dynamodb.Client) *DynamoPredictionCache {
	return &DynamoPredictionCache{
		client: client,
	}
}

// GetPredictions retrieves cached predictions for a station and date
func (c *DynamoPredictionCache) GetPredictions(ctx context.Context, stationID string, date time.Time) (*TidePredictionRecord, error) {
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

	var record TidePredictionRecord
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
func (c *DynamoPredictionCache) SavePredictions(ctx context.Context, record TidePredictionRecord) error {
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
func (c *DynamoPredictionCache) SavePredictionsBatch(ctx context.Context, records []TidePredictionRecord) error {
	// Process in batches of 25 (DynamoDB limit)
	batchSize := 25
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
			record.TTL = now + (cacheValidityDays * 24 * 60 * 60)

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

		input := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				tableName: writeRequests,
			},
		}

		if _, err := c.client.BatchWriteItem(ctx, input); err != nil {
			return fmt.Errorf("batch writing predictions: %w", err)
		}
	}

	return nil
}

func (c *DynamoPredictionCache) isValid(record TidePredictionRecord) bool {
	now := time.Now().Unix()
	return now < record.TTL
}
