#!/bin/bash

export PAGER=""

# Exit on any error
set -e

# Config
LAMBDA_FUNCTIONS=("flowebb-graphql-prod")
BUILD_DIR="$(dirname "$0")/.."

# Clean any existing build artifacts
rm -f "$BUILD_DIR/*.zip"

echo "Building Go binaries..."
cd "$BUILD_DIR"

# Build and zip graphql function
echo "Building graphql function..."
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap ./cmd/graphql
zip graphql.zip bootstrap
rm bootstrap

# Update each Lambda function
echo "Updating graphql function..."
aws lambda update-function-code \
    --function-name "${LAMBDA_FUNCTIONS[0]}" \
    --zip-file "fileb://$BUILD_DIR/graphql.zip"

# Wait for the updates to complete
for function_name in "${LAMBDA_FUNCTIONS[@]}"; do
    echo "Waiting for function ${function_name} update to complete..."
    aws lambda wait function-updated \
        --function-name "$function_name"
done

# Clean up
rm -f $BUILD_DIR/*.zip

echo "All Lambda functions updated successfully!"
