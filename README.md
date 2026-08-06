# ms-go-comment

`ms-go-comment` is an independent Go domain service for authenticated discussions attached to arbitrary external resources. It owns discussion spaces, threads, multi-level comments, attachment bindings, realtime delivery, and moderation state without foreign keys to Course, User, or any host product database.

## Status

The authenticated discussion, realtime, administration, moderation, and
private-resource access-grant backend slices are complete.
PostgreSQL persistence, thread/comment REST, validated FileStorage attachments,
transactional outbox delivery through NATS JetStream, single-use realtime
tickets, and the thread-scoped WebSocket projection are implemented. Start with the
[domain map](docs/domain-map.md) and the final
[agent contract map](docs/agent-contract-map.md).

Space/thread-policy administration and comment hide/restore moderation are
available under `/admin/v1`. Trusted host services use `/internal/v1` to ensure
private threads and mint short-lived grants; browsers present those grants as
`X-Comment-Access-Grant` on `/api/v1` calls.

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
task migrate
task run
task up
make validate-contracts
```

The service exposes `GET /healthz`. `SERVICE_MODE` supports `all`, `api`,
`realtime`, and `worker`; the default `all` process runs REST, WebSocket fan-out,
attachment cleanup, ticket cleanup, access-grant cleanup, and the outbox
dispatcher.

User REST and realtime-ticket requests have a bounded per-instance token-bucket
guard. Defaults are 20 requests/second, burst 40, at most 10,000 active actor
buckets, and five-minute idle eviction. Configure them with
`HTTP_USER_RATE_LIMIT_*`; gateway or distributed rate limiting remains the
cross-instance production authority. HTTP read/write/idle timeouts and maximum
header bytes are also configurable through `HTTP_*` environment variables.

## Security boundary

The service accepts `X-User-ID` and `X-User-Role` only from `ms-gateway`. Do not
publish the service container directly to untrusted networks. Every internal
route requires the exact configured `X-Internal-Token`. Set
`ACCESS_GRANT_MAX_TTL_SECONDS` (default 300, hard maximum 900) and
`ACCESS_GRANT_CLEANUP_SECONDS` (default 60) for private-resource grants. Browser
WebSocket connections use a short-lived, single-use ticket obtained through
authenticated REST; access tokens and access grants are not placed in a
WebSocket query string.
