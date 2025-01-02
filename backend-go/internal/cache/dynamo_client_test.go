package cache

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDynamoClient(t *testing.T) {
	// Save original environment and restore after test
	originalEndpoint := os.Getenv("DYNAMODB_ENDPOINT")
	defer func(key, value string) {
		err := os.Setenv(key, value)
		if err != nil {
			t.Errorf("Error restoring environment variable: %v", err)
		}
	}("DYNAMODB_ENDPOINT", originalEndpoint)

	tests := []struct {
		name           string
		endpoint       string
		wantLocalSetup bool
		wantErr        bool
	}{
		{
			name:           "local development setup",
			endpoint:       "http://localhost:8000",
			wantLocalSetup: true,
			wantErr:        false,
		},
		{
			name:           "production setup",
			endpoint:       "",
			wantLocalSetup: false,
			wantErr:        false,
		},
		{
			name:           "invalid endpoint",
			endpoint:       "invalid-endpoint",
			wantLocalSetup: true,
			wantErr:        false, // Should not error as config is still created
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment for test
			err := os.Setenv("DYNAMODB_ENDPOINT", tt.endpoint)
			if err != nil {
				return
			}

			// Create client
			client, err := NewDynamoClient(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)

			// For local setup, verify the endpoint was set correctly
			if tt.wantLocalSetup && tt.endpoint != "invalid-endpoint" {
				// Implementation specific: verify the endpoint was set correctly
				// Note: Actual verification depends on AWS SDK internals
				// You might need to use the client to make a test call to verify configuration

				// Example verification (adjust based on your needs):
				listTablesOutput, err := client.ListTables(context.Background(), nil)
				require.NoError(t, err)
				assert.NotNil(t, listTablesOutput)
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
			return
		}
		err = os.Setenv("ENV", originalEnv)
		if err != nil {
			return
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
			if err != nil {
				return
			}
			err = os.Setenv("DYNAMODB_ENDPOINT", tt.endpoint)
			if err != nil {
				return
			}

			// Create client
			client, err := NewDynamoClient(context.Background())
			require.NoError(t, err)
			require.NotNil(t, client)

			if tt.wantLocalSetup {
				// Verify local configuration
				// Note: The actual verification depends on how you can inspect the client configuration
				// You might need to make a test API call to verify the setup

				// Example verification (adjust based on your needs):
				listTablesOutput, err := client.ListTables(context.Background(), nil)
				require.NoError(t, err)
				assert.NotNil(t, listTablesOutput)
			}
		})
	}
}
