# Repository Guidelines

## Agent bootstrap

Read `.ai/rules/common.md`, `.ai/service.yaml`, `docs/README.md`, and the affected HTTP, WebSocket, event, database, attachment, access, or frontend contracts before changing files. Production code, migrations, tests, and repository-owned documentation are authoritative.

## Architecture invariants

- This service owns discussion spaces, opaque resource/thread bindings, comment trees, moderation state, attachment bindings, access grants, realtime tickets, sequences, and outbox records. It does not own users, host resources, or attachment bytes.
- Keep domain and use-case logic independent from chi, pgx, NATS, FileStorage, and WebSocket transports. Adapters implement ports; `cmd/ms-comment-service/main.go` is the composition root.
- The exact `space_key + resource_type + resource_id` tuple is a contract. Do not add host-database foreign keys or synchronous host-resource lookups.
- REST identity currently trusts gateway-replaced `X-User-ID` and `X-User-Role`; internal routes use `X-Internal-Token`. WebSocket authentication uses a short-lived single-use ticket in `Sec-WebSocket-Protocol`. Do not describe these boundaries as JWT verification.
- Private access grants are bound to issuer, user, resource tuple, permissions, and expiry. Raw grant and ticket secrets must never be persisted or placed in URLs or markup.
- A domain mutation, sequence allocation, and outbox insert commit atomically. NATS delivery is at least once, so producers and consumers must preserve idempotency and `event_id` deduplication.
- FileStorage owns bytes; this service owns authorization, attachment metadata, binding, activation/deletion retries, and signed-URL access.
- WebSocket and NATS are delivery projections. PostgreSQL and the REST changes feed remain authoritative when sequences have gaps.
- The browser SDK never sets actor headers or accepts an internal token. Render untrusted comment content as text and only project validated HTTP(S) or signed media URLs.

## Verification and delivery

- Use `.ai/commands.yaml`; run agent-policy, tracked-file formatting, `go vet`, lint, Go tests/build, contract validation, and browser SDK tests/build.
- Contract, lifecycle, schema, authorization, event, or frontend integration changes require focused regression tests and synchronized owned documentation.
- Do not start services, Docker/PostgreSQL/NATS, apply or roll back migrations, contact sibling services, or run live integration scenarios without approval. `docker compose down -v` is destructive.
