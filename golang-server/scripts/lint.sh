#!/bin/bash

set -euo pipefail

echo "Running golangci-lint using root .golangci.yml"

golangci-lint run ./...
