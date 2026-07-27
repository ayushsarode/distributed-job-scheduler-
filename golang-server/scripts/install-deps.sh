#!/bin/bash

set -e

go install github.com/bufbuild/buf/cmd/buf@v1.65.0
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
go install go.uber.org/mock/mockgen@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1
go install gotest.tools/gotestsum@latest