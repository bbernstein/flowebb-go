#!/bin/bash
set -e

export COVERAGE_THRESHOLD=90
export PAGER=""

ROOT_DIR=$(dirname "$0")/..
cd $ROOT_DIR

# Backend Go checks
go install github.com/99designs/gqlgen@latest
gqlgen generate
go mod tidy
golangci-lint run ./...
go test -race ./...


mkdir -p test-results
# Run tests with coverage and enforce minimum threshold (e.g., 80%)
ENV=development
go test -covermode=atomic -coverpkg="$(go list ./... | grep -v graph/generated | grep -v graph/model | tr '\n' ',')" ./... -coverprofile=test-results/coverage.out
# Check if coverage meets threshold
COVERAGE=$(go tool cover -func=test-results/coverage.out | grep total | awk '{print $3}' | sed 's/%//')
echo "Code coverage: $COVERAGE%"
THRESHOLD=$COVERAGE_THRESHOLD
if (( $(echo "$COVERAGE < $THRESHOLD" | bc -l) )); then
  echo "Code coverage $COVERAGE% is below threshold of $THRESHOLD%"
  exit 1
fi
