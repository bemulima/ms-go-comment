.PHONY: fmt deps tidy test lint web-test web-build validate-contracts migrate migrate-native up down

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
		grep -Eq '^schema_version: 1$$' "$$file"; \
	done

migrate:
	docker compose --profile migration run --rm migrate

migrate-native:
	python3 scripts/native_config.py -- scripts/native_migrate.sh

up:
	docker compose up -d postgres nats
	$(MAKE) migrate
	docker compose up -d --build ms-comment-service

down:
	docker compose down
