# WebSocket contract

WebSocket is a realtime projection of REST-owned state. Durable comment create/update/delete commands remain REST operations in the first slice.

## Handshake

1. An authenticated client requests `POST /api/v1/realtime/ticket` for one authorized thread and optional `last_sequence`.
2. The service returns a random opaque ticket with a maximum 30-second lifetime.
3. A browser opens `/api/v1/ws` with subprotocols `comment.v1` and `ticket.<opaque>`.
4. The WebSocket adapter validates `Origin`, atomically consumes the hashed ticket, binds the connection to its user/thread/permissions, and selects `comment.v1`.

Access tokens must not be put in WebSocket URLs. A ticket is single-use and thread-scoped.

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

Ephemeral types are `connection.ready`, `typing.started`, `typing.stopped`, `resync_required`, `error`, and `ping`. Client messages are limited to `typing.start`, `typing.stop`, and `pong` in v1.

## Reconnect

`connection.ready` contains the current thread sequence. When a supplied last sequence is behind or the server detects a gap, it sends `resync_required`. The client calls `GET /api/v1/comment/changes` until it reaches the advertised current sequence before trusting subsequent frames.

The adapter enforces bounded frame size, read/write deadlines, ping/pong liveness, per-user connection limits, and graceful close on shutdown.
