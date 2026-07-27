.PHONY: help test unit-test integration-test services-up services-down services-logs migrate

help:
	@echo "Available targets:"
	@echo "  make unit-test         Run unit tests only"
	@echo "  make integration-test  Run black-box integration tests"
	@echo "  make test              Run unit and integration tests"
	@echo "  make services-up       Start Postgres, Redis, Kafka, Prometheus, Grafana"
	@echo "  make services-down     Stop Docker Compose services"
	@echo "  make services-logs     Tail Docker Compose service logs"
	@echo "  make migrate           Run database migrations"

unit-test:
	go test -count=1 ./cmd/... ./internal/...

integration-test:
	./scripts/integration-tests.sh

test: unit-test integration-test

services-up:
	docker compose -f deploy/docker/docker-compose.yml up -d postgres redis kafka prometheus grafana

services-down:
	docker compose -f deploy/docker/docker-compose.yml down

services-logs:
	docker compose -f deploy/docker/docker-compose.yml logs -f

migrate:
	./scripts/migrate.sh
