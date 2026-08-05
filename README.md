# ms-go-comment

`ms-go-comment` is an independent Go domain service for authenticated discussions attached to arbitrary external resources. It owns discussion spaces, threads, multi-level comments, attachment bindings, realtime delivery, and moderation state without foreign keys to Course, User, or any host product database.

## Status

The repository is in the backend foundation phase. The versioned design map is in [docs/domain-map.md](docs/domain-map.md). The initial PostgreSQL schema, domain invariants, and repository ports are implemented; REST, adapters, attachment orchestration, outbox delivery, and WebSocket runtime are tracked by GitHub issues linked to the backend epic.

## Architecture

- `cmd/ms-comment-service`: composition root and graceful process lifecycle.
- `internal/domain`: domain entities, invariants, events, and repository ports.
- `internal/usecase`: transport-independent business processes.
- `internal/adapters/http`: Chi REST adapters and gateway identity boundary.
- `internal/adapters/websocket`: ticket-authenticated realtime transport.
- `internal/adapters/postgres`: pgx repositories and transaction manager.
- `internal/adapters/nats`: outbox delivery and realtime fan-out.
- `internal/adapters/filestorage`: staged user-media lifecycle.
- `db/migrations`: reversible PostgreSQL contracts.
- `.ai`: machine-readable service, architecture, command, workflow, and API contracts for agents.

## Local commands

```sh
task test
task run
task up
make validate-contracts
```

The service exposes `GET /healthz`. Business routes will be mounted under `/api/v1`, `/admin/v1`, and `/internal/v1`.

## Security boundary

The service accepts `X-User-ID` and `X-User-Role` only from `ms-gateway`. Do not publish the service container directly to untrusted networks. Browser WebSocket connections use a short-lived, single-use ticket obtained through authenticated REST; access tokens are not placed in a WebSocket query string.
