#!/bin/bash

cd "$(dirname "$0")/.." || exit
ENV=development
go test -covermode=atomic -coverpkg="$(go list ./... | grep -v graph/generated | grep -v graph/model | tr '\n' ',')" ./... -coverprofile=test-results/coverage.out
COVERAGE=$(go tool cover -func=test-results/coverage.out | grep total | awk '{print $3}' | sed 's/%//')
echo "Code coverage: $COVERAGE%"
