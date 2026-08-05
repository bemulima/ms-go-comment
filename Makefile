.PHONY: fmt deps tidy test lint validate-contracts up down

fmt:
	gofmt -w cmd internal

deps:
	XDG_CACHE_HOME=$(CURDIR)/.cache GOMODCACHE=$(CURDIR)/.cache/gomod go mod download all

tidy:
	XDG_CACHE_HOME=$(CURDIR)/.cache GOMODCACHE=$(CURDIR)/.cache/gomod go mod tidy

test:
	XDG_CACHE_HOME=$(CURDIR)/.cache GOCACHE=$(CURDIR)/.cache/go-build GOMODCACHE=$(CURDIR)/.cache/gomod go test ./...

lint:
	$(CURDIR)/.cache/bin/golangci-lint run ./...

validate-contracts:
	@set -eu; \
	for file in .ai/service.yaml .ai/architecture.yaml .ai/commands.yaml .ai/contracts/database.yaml .ai/contracts/http.yaml .ai/contracts/websocket.yaml .ai/contracts/events.yaml; do \
		test -s "$$file"; \
		rg -q '^schema_version: 1$$' "$$file"; \
	done

up:
	docker compose up -d --build

down:
	docker compose down -v
