package cache

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/rs/zerolog/log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// DynamoDBClient interface defines the DynamoDB operations we use
type DynamoDBClient interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	BatchWriteItem(context.Context, *dynamodb.BatchWriteItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	ListTables(context.Context, *dynamodb.ListTablesInput, ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error)
}

// NewDynamoClient creates a new DynamoDB client based on environment
func NewDynamoClient(ctx context.Context) (DynamoDBClient, error) {
	if endpoint := os.Getenv("DYNAMODB_ENDPOINT"); endpoint != "" {
		// Local development configuration
		log.Debug().Str("endpoint", endpoint).Msg("Using local DynamoDB endpoint")
		customOptions := []func(*config.LoadOptions) error{
			config.WithRegion("local"),
			config.WithClientLogMode(aws.LogRetries),
		}

		cfg, err := config.LoadDefaultConfig(ctx, customOptions...)
		if err != nil {
			return nil, err
		}

		// Create the DynamoDB client with local endpoint
		client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})

		return client, nil
	}

	// Production configuration
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	return dynamodb.NewFromConfig(cfg), nil
}
