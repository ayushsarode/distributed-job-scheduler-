#!/bin/bash

buf generate
buf generate --template  agentservice.buf.gen.yaml
sqlc generate
go generate ./...

go mod tidy
go build .
