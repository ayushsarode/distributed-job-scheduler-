#!/bin/bash

set -euo pipefail

go test -v $(go list ./... | grep -v '/testing')

