SHELL := /bin/sh

# Database name selection: use CLICKHOUSE_DB env if set, else default to 'wallets'
CH_DB ?= $(CLICKHOUSE_DB)
ifeq ($(strip $(CH_DB)),)
  CH_DB := wallets
endif

.PHONY: schema-dev go-test api-test db-dev ingest-dev go-cover api-cover tools-test

db-dev:
	@echo "Ensuring database exists: $(CH_DB)"
	@which clickhouse-client >/dev/null 2>&1 || { echo "clickhouse-client not found"; exit 1; }
	@clickhouse-client -q "CREATE DATABASE IF NOT EXISTS $(CH_DB)"

schema-dev: db-dev
	@echo "Applying dev schema (sql/schema_dev.sql) to DB=$(CH_DB)"
	@which clickhouse-client >/dev/null 2>&1 || { echo "clickhouse-client not found"; exit 1; }
	@clickhouse-client --database $(CH_DB) --queries-file sql/schema_dev.sql

go-test:
	GOCACHE=$(PWD)/.gocache GOMODCACHE=$(PWD)/.gocache/mod GOPATH=$(PWD)/.gocache/gopath go test -race ./...

go-cover:
	GOCACHE=$(PWD)/.gocache GOMODCACHE=$(PWD)/.gocache/mod GOPATH=$(PWD)/.gocache/gopath go test ./... -coverprofile=coverage.out -covermode=count
	GOCACHE=$(PWD)/.gocache GOMODCACHE=$(PWD)/.gocache/mod GOPATH=$(PWD)/.gocache/gopath go tool cover -func=coverage.out

api-test:
	cd api && npm run test:threads

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
	@test -n "$(ADDRESS)" || { echo "ADDRESS is required (0x...)"; exit 1; }
	@echo "Running ingester for $(ADDRESS) mode=$(MODE) range=$(FROM)..$(TO) batch=$(BATCH)"
	GOCACHE=$(PWD)/.gocache GOMODCACHE=$(PWD)/.gocache/mod GOPATH=$(PWD)/.gocache/gopath \
		go run ./cmd/ingester --address $(ADDRESS) --mode $(MODE) --from-block $(FROM) --to-block $(TO) --batch $(BATCH)
