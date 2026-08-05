# Realtime delivery

## Durable flow

Every durable mutation allocates a monotonic sequence inside its thread transaction and inserts a versioned outbox event in the same commit. A worker claims unpublished events, publishes them to NATS JetStream with `event_id` deduplication, and records success. Failures use bounded exponential backoff and remain inspectable.

Thread/comment REST mutations already implement the transaction and outbox-write
half of this flow. Broker publication, retries, and fan-out are implemented by
the realtime slice; an unpublished row is therefore expected until that worker
is running.

Subjects are:

```text
comment.created
comment.updated
comment.deleted
comment.hidden
comment.restored
comment.attachment.ready
comment.attachment.failed
comment.thread.updated
```

All lifecycle payloads include `schema_version`, `event_id`, `occurred_at`, `thread_id`, and `sequence`. Comment payloads also include comment, parent/root, actor/author, lifecycle status, version, safe content, and attachment projections required by the WebSocket client.

## Fan-out

Every realtime instance subscribes to lifecycle subjects without a shared queue group so each instance can deliver an event to its own local connections. Ephemeral typing signals use non-durable NATS subjects scoped by thread and are never written to the outbox.

At-least-once delivery means clients and hubs deduplicate by `event_id` and apply state by sequence/version. WebSocket delivery can still be interrupted; `comment/changes` is the recovery source.
