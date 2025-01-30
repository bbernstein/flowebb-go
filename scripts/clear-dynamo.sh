#!/bin/bash

export PAGER=""

# List all tables
aws dynamodb list-tables --endpoint-url http://localhost:8000

# Delete each table
aws dynamodb delete-table --table-name stations-cache --endpoint-url http://localhost:8000
aws dynamodb delete-table --table-name tide-predictions-cache --endpoint-url http://localhost:8000

# Recreate the tables
./scripts/init-local-dynamo.sh
