#!/bin/bash

rm exiro.ai
find application/models -type f -name "*.go" -delete
find infra/database/postgres/gen -type f -name "*.go" -delete
find . -type d -name "mocks" -exec find {} -type f -name "*.go" -delete \;
