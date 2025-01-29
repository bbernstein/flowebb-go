package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bbernstein/flowebb/backend-go/internal/models"
	"github.com/rs/zerolog/log"
	"io"
	"time"
)

// S3Client defines the interface for S3 operations we need
type S3Client interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

const (
	cacheKey = "stations.json"
)

// S3StationCache provides caching for station lists in S3
type S3StationCache struct {
	client     S3Client
	bucketName string
	ttl        time.Duration
	clock      clock // Use the same clock interface from cache package
}

// StationListCacheRecord represents the cached station list with metadata
type StationListCacheRecord struct {
	Stations    []models.Station `json:"stations"`
	LastUpdated int64            `json:"lastUpdated"`
	TTL         int64            `json:"ttl"`
}

// StationListCacheProvider defines interface for station list caching
type StationListCacheProvider interface {
	GetStations(ctx context.Context) ([]models.Station, error)
	SaveStations(ctx context.Context, stations []models.Station) error
}

// GetStations retrieves stations from S3 cache if available and valid
func (c *S3StationCache) GetStations(ctx context.Context) ([]models.Station, error) {
	if c.bucketName == "" {
		return nil, fmt.Errorf("empty bucket name")
	}

	// Get object from S3
	result, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(cacheKey),
	})
	if err != nil {
		// If object doesn't exist, return nil without error
		return nil, nil
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Error().Err(err).Msg("Error closing S3 object body")
		}
	}(result.Body)

	// Decode cache record
	var record StationListCacheRecord
	if err := json.NewDecoder(result.Body).Decode(&record); err != nil {
		return nil, fmt.Errorf("decoding cache record: %w", err)
	}

	// Check if cache is expired
	if c.clock.Now().Unix() > record.TTL {
		log.Debug().Msg("Station list cache expired")
		return nil, nil
	}

	return record.Stations, nil
}

// SaveStations saves stations to S3 cache
func (c *S3StationCache) SaveStations(ctx context.Context, stations []models.Station) error {
	if c.bucketName == "" {
		return fmt.Errorf("empty bucket name")
	}

	// Create cache record
	now := c.clock.Now().Unix()
	record := StationListCacheRecord{
		Stations:    stations,
		LastUpdated: now,
		TTL:         now + int64(c.ttl.Seconds()),
	}

	// Encode record
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(record); err != nil {
		return fmt.Errorf("encoding cache record: %w", err)
	}

	// Save to S3
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(cacheKey),
		Body:   bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		return fmt.Errorf("saving to S3: %w", err)
	}

	log.Debug().Int("station_count", len(stations)).Msg("Saved station list to S3 cache")
	return nil
}
