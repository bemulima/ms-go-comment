.PHONY: fmt deps tidy test lint web-test web-build validate-contracts migrate up down

fmt:
	gofmt -w cmd internal test

deps:
	XDG_CACHE_HOME=$(CURDIR)/.cache GOMODCACHE=$(CURDIR)/.cache/gomod go mod download all

tidy:
	XDG_CACHE_HOME=$(CURDIR)/.cache GOMODCACHE=$(CURDIR)/.cache/gomod go mod tidy

test:
	XDG_CACHE_HOME=$(CURDIR)/.cache GOCACHE=$(CURDIR)/.cache/go-build GOMODCACHE=$(CURDIR)/.cache/gomod go test ./...

lint:
	$(CURDIR)/.cache/bin/golangci-lint run ./...

web-test:
	npm --prefix web test

web-build:
	npm --prefix web run build

validate-contracts:
	@set -eu; \
	for file in .ai/service.yaml .ai/architecture.yaml .ai/commands.yaml .ai/contracts/database.yaml .ai/contracts/http.yaml .ai/contracts/websocket.yaml .ai/contracts/events.yaml .ai/contracts/frontend.yaml; do \
		test -s "$$file"; \
		rg -q '^schema_version: 1$$' "$$file"; \
	done

migrate:
	@set -eu; \
	has_schema=$$(docker compose exec -T postgres psql -U "$${POSTGRES_USER:-postgres}" -d "$${POSTGRES_DB:-ms_comment}" -tAc "SELECT to_regclass('public.comment_space') IS NOT NULL AND to_regclass('public.comment_thread') IS NOT NULL AND to_regclass('public.comment') IS NOT NULL AND to_regclass('public.comment_attachment') IS NOT NULL AND to_regclass('public.comment_outbox') IS NOT NULL AND to_regclass('public.comment_ws_ticket') IS NOT NULL;"); \
	if [ "$$has_schema" = "t" ]; then \
		echo "Initial comment schema already exists"; \
	else \
		docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "$${POSTGRES_USER:-postgres}" -d "$${POSTGRES_DB:-ms_comment}" < db/migrations/001_init.up.sql; \
	fi; \
	has_attachment_delivery=$$(docker compose exec -T postgres psql -U "$${POSTGRES_USER:-postgres}" -d "$${POSTGRES_DB:-ms_comment}" -tAc "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='comment_attachment' AND column_name='activation_attempts');"); \
	if [ "$$has_attachment_delivery" = "t" ]; then \
		echo "Attachment delivery migration already applied"; \
	else \
		docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "$${POSTGRES_USER:-postgres}" -d "$${POSTGRES_DB:-ms_comment}" < db/migrations/002_attachment_delivery.up.sql; \
	fi; \
	has_access_grants=$$(docker compose exec -T postgres psql -U "$${POSTGRES_USER:-postgres}" -d "$${POSTGRES_DB:-ms_comment}" -tAc "SELECT to_regclass('public.comment_access_grant') IS NOT NULL;"); \
	if [ "$$has_access_grants" = "t" ]; then \
		echo "Access grant migration already applied"; \
	else \
		docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "$${POSTGRES_USER:-postgres}" -d "$${POSTGRES_DB:-ms_comment}" < db/migrations/003_access_grants.up.sql; \
	fi

up:
	docker compose up -d --build

down:
	docker compose down -v
