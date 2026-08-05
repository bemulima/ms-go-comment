# Database contract

PostgreSQL is owned exclusively by `ms-go-comment`. Migrations are ordered, reversible `.up.sql`/`.down.sql` pairs and must preserve existing data unless an approved contract change explicitly says otherwise.

## Implemented schema

The initial schema is implemented by the reversible pair
`db/migrations/001_init.up.sql` / `001_init.down.sql`. The down migration removes
tables in reverse dependency order and deliberately does not use `CASCADE`.
`002_attachment_delivery.up.sql` / `.down.sql` adds only retry/delivery evidence
to `comment_attachment` plus partial worker indexes.

### `comment_space`

Owns stable integration key, lifecycle status, access mode, allowed origins,
content limits, and timestamps. Status values are `0 disabled`, `1 active`;
access modes are `1 authenticated`, `2 context_grant`.

### `comment_thread`

Owns the exact external-resource tuple, lifecycle status, nullable policy
overrides, root/total counters, `last_sequence`, last activity, and timestamps.
It has a unique constraint on `(space_id, resource_type, resource_id)`. Status
values are `1 open`, `2 read_only`, `3 closed`, `4 hidden`.

### `comment`

Owns identity, thread, author, parent/root/path/depth, Markdown source, extracted links, lifecycle status, optimistic version, latest sequence, direct reply count, idempotency key, and audit timestamps.

Status values are `1 active`, `2 deleted`, `3 hidden`. The self-referencing root
constraint is deferred so a root row can refer to itself at transaction commit.

Required integrity includes:

- composite parent/root references scoped to the same thread;
- unique `(author_id, idempotency_key)`;
- non-negative depth and counters;
- GIN index on UUID path;
- cursor indexes on `(thread_id, parent_id, created_at, id)` and `(thread_id, sequence)`.

### `comment_attachment`

Owns thread/comment/uploader bindings, FileStorage identity, lifecycle status,
safe media metadata, TTL, activation, and deletion timestamps. Bytes remain
owned by FileStorage. Status values are `1 pending`, `2 processing`, `3 ready`,
`4 failed`, `5 deleted`; MIME types are limited to JPEG, PNG, and WebP.
Activation/delete attempts, next retry timestamps, bounded failure evidence, and
`storage_deleted_at` make worker outcomes observable and retriable.

### `comment_outbox`

Owns event ID, aggregate identity, subject, versioned JSON payload, attempts, retry time, publication result, and error evidence. Dispatchers claim rows with `FOR UPDATE SKIP LOCKED` and publish with the event ID as the NATS deduplication ID.

### `comment_ws_ticket`

Owns only a cryptographic hash of the random ticket plus user, thread, permissions, the client's optional requested `last_sequence`, and short expiry. Handshake consumption returns that sequence context atomically while deleting the ticket, so reconnect gap detection does not depend on WebSocket query parameters or untrusted post-upgrade state.

Permissions use a bit mask: `read=1`, `write=2`, `upload=4`; every ticket must
include `read`. Only the 32-byte SHA-256 ticket hash is persisted.

## Transaction invariants

- Thread sequence allocation, comment mutation, counters, attachment binding, and outbox insertion are atomic.
- NATS and FileStorage network calls do not execute inside a long-running database transaction.
- Attachment activation is retriable and represented explicitly as processing/ready/failed.
- Hard deletion is not part of the normal user API.

## Code boundary

The matching entities, value objects, validation rules, and stable errors live
in `internal/domain`. Persistence contracts live in `internal/domain/repository`
and expose no pgx or SQL types. Concrete pgx implementations live in
`internal/adapters/postgres`; a context-bound transaction manager makes sequence,
counters, comment state, attachment binding, and outbox insertion one commit.
