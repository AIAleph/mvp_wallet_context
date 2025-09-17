SHELL := /bin/sh

# Docker Compose wrapper
DOCKER_COMPOSE ?= docker compose

# Database name selection: use CLICKHOUSE_DB env if set, else default to 'wallets'
CH_DB ?= $(CLICKHOUSE_DB)
ifeq ($(strip $(CH_DB)),)
  CH_DB := wallets
endif

# Public targets requested
.PHONY: schema api ingest test lint go-test api-test ingest-dev go-cover api-cover tools-test \
    dev-up dev-down dev-logs dev-nuke ch-client ensure-clickhouse migrate-schema

# Local dev stack: ClickHouse and Redis (minimal, no Keeper needed)
dev-up:
	$(DOCKER_COMPOSE) up -d

dev-down:
	$(DOCKER_COMPOSE) down

dev-logs:
	$(DOCKER_COMPOSE) logs -f clickhouse redis

# Remove containers, network, and named volumes created by Compose
dev-nuke:
	$(DOCKER_COMPOSE) down -v
	@echo "Compose stack fully removed (containers, network, volumes)."



# Apply schema via script (prefers sql/schema.sql; falls back to dev)
schema:
	./scripts/schema.sh

ensure-clickhouse:
	./scripts/ensure_clickhouse.sh

migrate-schema: ensure-clickhouse
	./scripts/migrate_schema.sh $(if $(TO),TO=$(TO),) $(if $(DB),DB=$(DB),) $(if $(DRY_RUN),DRY_RUN=$(DRY_RUN),)

# Open an interactive ClickHouse client inside the container
ch-client:
	@$(DOCKER_COMPOSE) exec clickhouse clickhouse-client --database "$(CH_DB)"

go-test:
	GOCACHE=$(PWD)/.gocache GOMODCACHE=$(PWD)/.gocache/mod GOPATH=$(PWD)/.gocache/gopath go test -race ./...

go-cover:
	GOCACHE=$(PWD)/.gocache GOMODCACHE=$(PWD)/.gocache/mod GOPATH=$(PWD)/.gocache/gopath go test ./... -coverprofile=coverage.out -covermode=count
	GOCACHE=$(PWD)/.gocache GOMODCACHE=$(PWD)/.gocache/mod GOPATH=$(PWD)/.gocache/gopath go tool cover -func=coverage.out

api-test:
	cd api && npm run test:threads

# Start API in dev mode (Node 20+)
api:
	./scripts/api.sh dev

# API coverage with thresholds enforced in vitest config (100%).
api-cover:
	cd api && npm run test

# Python tools lint + tests with coverage 100% enforced
tools-test:
	cd tools && ruff check . && black --check . && pytest --cov=apply_priority --cov=create_github_issues --cov=check_go_coverage --cov-fail-under=100 -q

# Helper: run ingester for a single address with optional range
# Usage: make ingest-dev ADDRESS=0x... [FROM=0] [TO=0] [MODE=backfill|delta] [BATCH=5000]
ADDRESS ?=
FROM ?= 0
TO ?= 0
MODE ?= backfill
BATCH ?= 5000

ingest-dev:
	./scripts/ingest.sh ADDRESS=$(ADDRESS) MODE=$(MODE) FROM=$(FROM) TO=$(TO) BATCH=$(BATCH)

# Public alias for ingestion
ingest: ingest-dev

# Aggregate targets
test:
	./scripts/test.sh

lint:
	./scripts/lint.sh
