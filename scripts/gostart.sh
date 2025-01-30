#!/bin/bash

export PAGER=""

set -e

ROOT_DIR="$(dirname "$0")/.."

# Change to backend directory
cd "$ROOT_DIR" || exit 1

# Start DynamoDB Local and Admin UI
docker-compose up -d

# Create DynamoDB tables if they don't exist
./scripts/init-local-dynamo.sh

# this was needed for some reason for me on mac m2 docker desktop
export DOCKER_HOST=unix://${HOME}/.docker/run/docker.sock

./scripts/gobuild.sh && \
sam local start-api \
  --warm-containers EAGER \
  --docker-network sam-network \
  --port 8080 \
  --parameter-overrides Stage=local \
  --container-host 0.0.0.0 \
  --container-host-interface 0.0.0.0
