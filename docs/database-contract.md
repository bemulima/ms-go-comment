# Database contract

PostgreSQL is owned exclusively by `ms-go-comment`. Migrations are ordered, reversible `.up.sql`/`.down.sql` pairs and must preserve existing data unless an approved contract change explicitly says otherwise.

## Planned tables

### `comment_space`

Owns stable integration key, lifecycle status, access mode, allowed origins, content limits, and timestamps.

### `comment_thread`

Owns the exact external-resource tuple, lifecycle status, nullable policy overrides, root/total counters, `last_sequence`, last activity, and timestamps. It has a unique constraint on `(space_id, resource_type, resource_id)`.

### `comment`

Owns identity, thread, author, parent/root/path/depth, Markdown source, extracted links, lifecycle status, optimistic version, latest sequence, direct reply count, idempotency key, and audit timestamps.

Required integrity includes:

- composite parent/root references scoped to the same thread;
- unique `(author_id, idempotency_key)`;
- non-negative depth and counters;
- GIN index on UUID path;
- cursor indexes on `(thread_id, parent_id, created_at, id)` and `(thread_id, sequence)`.

### `comment_attachment`

Owns thread/comment/uploader bindings, FileStorage identity, lifecycle status, safe media metadata, TTL, activation, and deletion timestamps. Bytes remain owned by FileStorage.

### `comment_outbox`

Owns event ID, aggregate identity, subject, versioned JSON payload, attempts, retry time, publication result, and error evidence. Dispatchers claim rows with `FOR UPDATE SKIP LOCKED` and publish with the event ID as the NATS deduplication ID.

### `comment_ws_ticket`

Owns only a cryptographic hash of the random ticket plus user, thread, permissions, and short expiry. Handshake consumption is atomic and destructive.

## Transaction invariants

- Thread sequence allocation, comment mutation, counters, attachment binding, and outbox insertion are atomic.
- NATS and FileStorage network calls do not execute inside a long-running database transaction.
- Attachment activation is retriable and represented explicitly as processing/ready/failed.
- Hard deletion is not part of the normal user API.
