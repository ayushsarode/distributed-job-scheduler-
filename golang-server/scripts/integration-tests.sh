#!/bin/bash

set -e
export TESTCONTAINERS_RYUK_DISABLED=true
if command -v gotestsum &> /dev/null; then
    gotestsum --format standard-verbose -- -v -count=1 -p 1 ./testing/...
else
    go test -v -count=1 -p 1 ./testing/...
fi
