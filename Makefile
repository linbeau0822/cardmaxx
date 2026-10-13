# CardMaxx development tasks.
#
# Run `make` (or `make help`) to list every target.

BACKEND  := backend
FRONTEND := frontend

# golang-migrate is run through `go run` at a pinned version rather than being
# installed separately or added to go.mod: contributors and CI then use the
# exact same migration tool with no extra setup step.
MIGRATE_VERSION := v4.20.1
MIGRATE := go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)
MIGRATIONS := migrations

# The application reads plain environment variables (12-factor); .env is a
# developer convenience that the Makefile sources for targets that need it.
ENV_FILE := .env
load_env = set -a; . $(CURDIR)/$(ENV_FILE); set +a;

.DEFAULT_GOAL := help
.PHONY: help env-check setup dev dev-backend dev-frontend build build-backend build-frontend \
        test test-backend format format-check lint vet sqlc tidy \
        db-up db-down db-reset db-shell migrate-up migrate-down migrate-create clean

help: ## List available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

env-check:
	@test -f $(ENV_FILE) || { \
		echo "error: $(ENV_FILE) not found. Run: cp .env.example .env"; exit 1; }

# --- setup -------------------------------------------------------------------

setup: ## Install dependencies for backend and frontend
	cd $(BACKEND) && go mod download
	cd $(FRONTEND) && npm install

# --- development -------------------------------------------------------------

dev: ## Start PostgreSQL, apply migrations, then run the API server
	$(MAKE) db-up
	$(MAKE) migrate-up
	$(MAKE) dev-backend

dev-backend: env-check ## Run the Go API server
	cd $(BACKEND) && $(load_env) go run ./cmd/server

dev-frontend: ## Run the Vite dev server
	cd $(FRONTEND) && npm run dev

# --- build -------------------------------------------------------------------

build: build-backend build-frontend ## Build backend and frontend

build-backend: ## Compile the API server to backend/bin/server
	cd $(BACKEND) && go build -o bin/server ./cmd/server

build-frontend: ## Type-check and bundle the frontend
	cd $(FRONTEND) && npm run build

# --- quality -----------------------------------------------------------------

test: test-backend ## Run all tests

test-backend: ## Run Go tests with the race detector
	cd $(BACKEND) && go test -race ./...

format: ## Format Go code
	cd $(BACKEND) && gofmt -w .

format-check: ## Fail if any Go file is not gofmt-clean (used by CI)
	@cd $(BACKEND) && unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "these files are not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi

vet: ## Run go vet
	cd $(BACKEND) && go vet ./...

lint: format-check vet ## Run all static checks
	cd $(FRONTEND) && npm run lint

tidy: ## Tidy Go module dependencies
	cd $(BACKEND) && go mod tidy

sqlc: ## Regenerate type-safe query code from sql/queries
	cd $(BACKEND) && sqlc generate

# --- database ----------------------------------------------------------------

db-up: ## Start PostgreSQL and wait until it is accepting connections
	docker compose up -d --wait postgres

db-down: ## Stop PostgreSQL (data is preserved)
	docker compose down

db-reset: ## Destroy the database volume and recreate an empty, migrated schema
	docker compose down -v
	$(MAKE) db-up
	$(MAKE) migrate-up

db-shell: ## Open a psql shell on the development database
	docker compose exec postgres psql -U cardmaxx -d cardmaxx

migrate-up: env-check ## Apply all pending migrations
	cd $(BACKEND) && $(load_env) $(MIGRATE) -path $(MIGRATIONS) -database "$$DATABASE_URL" up

migrate-down: env-check ## Roll back the most recent migration
	cd $(BACKEND) && $(load_env) $(MIGRATE) -path $(MIGRATIONS) -database "$$DATABASE_URL" down 1

migrate-create: env-check ## Create a migration pair: make migrate-create name=add_x
	@test -n "$(name)" || { echo "usage: make migrate-create name=add_something"; exit 1; }
	cd $(BACKEND) && $(MIGRATE) create -ext sql -dir $(MIGRATIONS) -seq $(name)

# --- housekeeping ------------------------------------------------------------

clean: ## Remove build output
	rm -rf $(BACKEND)/bin $(FRONTEND)/dist
