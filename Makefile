# Common tasks. `make help` lists them.

SHELL := /bin/bash
.DEFAULT_GOAL := help

.PHONY: help setup dev dev-server dev-console build test check lint fmt db migrate keygen brand clean

help: ## Show this list
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-14s %s\n", $$1, $$2}'

setup: ## Install JavaScript dependencies and download Go modules
	pnpm install
	cd server && go mod download

dev: ## Run the server and console at https://api.veto.localhost and https://veto.localhost
	pnpm dev

dev-server: ## Run only vetod, through portless
	cd server && pnpm exec portless api.veto sh ./scripts/dev.sh

dev-console: ## Run only the console, through portless
	cd apps/console && pnpm exec portless veto next dev

build: ## Build vetod, the console and the TypeScript SDK
	cd server && go build -trimpath -o bin/vetod ./cmd/vetod
	pnpm --filter @veto/console build
	pnpm --filter @shvgroups/veto build

test: ## Run every test suite (Go tests need VETO_TEST_DATABASE_URL for the database ones)
	cd server && go test ./...
	pnpm --filter @veto/console test
	pnpm --filter @shvgroups/veto test
	cd sdks/python && python3 -m pytest -q

lint: ## Vet Go code and lint the console
	cd server && go vet ./... && test -z "$$(gofmt -l .)"
	pnpm --filter @veto/console lint
	pnpm --filter @veto/console typecheck
	pnpm --filter @shvgroups/veto typecheck

check: lint test ## Everything CI runs, except image builds

fmt: ## Format Go and TypeScript code
	cd server && gofmt -w .
	pnpm format

db: ## Start a local PostgreSQL in Docker on port 5432
	docker run -d --name veto-postgres -e POSTGRES_USER=veto -e POSTGRES_PASSWORD=veto -e POSTGRES_DB=veto \
		-p 5432:5432 postgres:16-alpine

migrate: ## Apply database migrations using VETO_DATABASE_URL
	cd server && go run ./cmd/vetod migrate

keygen: ## Print a new master key
	cd server && go run ./cmd/vetod keygen

brand: ## Regenerate brand SVGs and PNGs
	pnpm brand

clean: ## Remove build output
	rm -rf server/bin apps/console/.next sdks/typescript/dist
