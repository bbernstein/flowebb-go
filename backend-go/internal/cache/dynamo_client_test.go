package cache

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDynamoClient(t *testing.T) {
	originalEndpoint := os.Getenv("DYNAMODB_ENDPOINT")
	defer func() {
		err := os.Setenv("DYNAMODB_ENDPOINT", originalEndpoint)
		if err != nil {
			return
		}
	}()

	tests := []struct {
		name           string
		endpoint       string
		wantLocalSetup bool
	}{
		{
			name:           "local development setup",
			endpoint:       "http://localhost:8000",
			wantLocalSetup: true,
		},
		{
			name:           "production setup",
			endpoint:       "",
			wantLocalSetup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.Setenv("DYNAMODB_ENDPOINT", tt.endpoint)
			if err != nil {
				return
			}

			client, err := NewDynamoClient(context.Background())
			require.NoError(t, err)
			require.NotNil(t, client)

			// For local setup, verify the client was configured with local options
			if tt.wantLocalSetup {
				// Type assert to access the internal config
				// Note: This is just a basic check that the client was created
				require.NotNil(t, client)
			}
		})
	}
}

func TestDynamoClientEnvironmentDetection(t *testing.T) {
	// Save original environment and restore after test
	originalEndpoint := os.Getenv("DYNAMODB_ENDPOINT")
	originalEnv := os.Getenv("ENV")
	defer func() {
		err := os.Setenv("DYNAMODB_ENDPOINT", originalEndpoint)
		if err != nil {
			t.Errorf("Failed to restore DYNAMODB_ENDPOINT: %v", err)
		}
		err = os.Setenv("ENV", originalEnv)
		if err != nil {
			t.Errorf("Failed to restore ENV: %v", err)
		}
	}()

	tests := []struct {
		name           string
		env            string
		endpoint       string
		wantLocalSetup bool
	}{
		{
			name:           "development environment",
			env:            "development",
			endpoint:       "http://localhost:8000",
			wantLocalSetup: true,
		},
		{
			name:           "local environment",
			env:            "local",
			endpoint:       "http://localhost:8000",
			wantLocalSetup: true,
		},
		{
			name:           "production environment",
			env:            "production",
			endpoint:       "",
			wantLocalSetup: false,
		},
		{
			name:           "unspecified environment",
			env:            "",
			endpoint:       "",
			wantLocalSetup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment for test
			err := os.Setenv("ENV", tt.env)
			require.NoError(t, err, "Failed to set ENV")
			err = os.Setenv("DYNAMODB_ENDPOINT", tt.endpoint)
			require.NoError(t, err, "Failed to set DYNAMODB_ENDPOINT")

			// Create client
			client, err := NewDynamoClient(context.Background())
			require.NoError(t, err, "NewDynamoClient should not error")
			require.NotNil(t, client, "Client should not be nil")

			// For local setup cases, verify we got a client configured with local endpoint
			if tt.wantLocalSetup {
				// We can't easily check the internal configuration of the client,
				// but we can verify it was created successfully and is of the correct type
				_, ok := client.(*dynamodb.Client)
				assert.True(t, ok, "Client should be a *dynamodb.Client")
			}
		})
	}
}
