# WebSocket contract

WebSocket is a realtime projection of REST-owned state. Durable comment create/update/delete commands remain REST operations in the first slice.

## Handshake

1. An authenticated client requests `POST /api/v1/realtime/ticket` for one authorized thread and optional `last_sequence`; the value is stored with the hashed ticket.
2. The service returns a random opaque ticket with a maximum 30-second lifetime.
3. A browser opens `/api/v1/ws` with subprotocols `comment.v1` and `ticket.<opaque>`.
4. The WebSocket adapter validates `Origin`, atomically consumes the hashed ticket, binds the connection to its user/thread/permissions/requested sequence, and selects `comment.v1`.

Access tokens must not be put in WebSocket URLs. A ticket is single-use and thread-scoped.
For a `context_grant` space, ticket minting also requires
`X-Comment-Access-Grant`. Stored ticket permissions are the intersection of the
grant, current thread state, and effective image policy. The WebSocket handshake
does not accept or re-resolve the access grant; the consumed ticket is
authoritative for that connection and is attenuated again if the thread closes
or image policy is disabled before consumption.
The exact `Origin` must be present in the owning space allowlist. Missing or
different origins fail the handshake. Query-string `ticket` and `access_token`
credentials are rejected before upgrade.

## Envelope

```json
{
  "v": 1,
  "type": "comment.created",
  "event_id": "uuid",
  "thread_id": "uuid",
  "sequence": 43,
  "occurred_at": "2026-08-05T10:00:00Z",
  "data": {}
}
```

Durable server event types are `comment.created`, `comment.updated`, `comment.deleted`, `comment.hidden`, `comment.restored`, `attachment.ready`, `attachment.failed`, and `thread.updated`.

`comment.hidden` is redacted and never carries the moderated body, links, or
attachments. `comment.restored` carries the safe restored projection.

Ephemeral types are `connection.ready`, `typing.started`, `typing.stopped`, `resync_required`, `error`, and `ping`. Client messages are limited to `typing.start`, `typing.stop`, and `pong` in v1.

## Reconnect

`connection.ready` contains the current thread sequence. When a supplied last sequence is behind or the server detects a gap, it sends `resync_required`. The client calls `GET /api/v1/comment/changes` until it reaches the advertised current sequence before trusting subsequent frames.

During handshake the hub registers the connection in a pre-ready state,
buffers matching fan-out, re-reads `last_sequence` from PostgreSQL, and then
activates the queue with `connection.ready` first. This closes the race where a
commit could otherwise land between ticket consumption and hub registration.

The adapter enforces bounded frame size, read/write deadlines, ping/pong liveness, per-user connection limits, and graceful close on shutdown.

The implemented hub is process-local and partitions connections by thread ID.
Each connection has a bounded outbound queue; a slow consumer is disconnected
instead of allowing unbounded memory growth. The v1 server defaults are a 16
KiB client frame, five connections per user, and 64 queued outbound frames.
Application `ping`/`pong` frames coexist with WebSocket control-frame liveness.

Lifecycle NATS subjects are mapped to the public names above. In particular,
`comment.attachment.ready`, `comment.attachment.failed`, and
`comment.thread.updated` become `attachment.ready`, `attachment.failed`, and
`thread.updated`. Subject-specific fields are placed under `data`.
