# Realtime delivery

## Durable flow

Every durable mutation allocates a monotonic sequence inside its thread transaction and inserts a versioned outbox event in the same commit. A worker claims unpublished events, publishes them to NATS JetStream with `event_id` deduplication, and records success. Failures use bounded exponential backoff and remain inspectable.

The dispatcher atomically leases eligible rows by advancing `next_attempt_at`
in a short `FOR UPDATE SKIP LOCKED` statement, commits that claim, and only then
performs the JetStream network call. A publish uses `Nats-Msg-Id=event_id`.
Success records `published_at`; failure increments attempts, stores bounded
error evidence, and schedules exponential retry starting at five seconds and
capped at five minutes. A crashed worker leaves a finite lease rather than a
permanent lock.

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

An admin thread status/policy change increments the same thread sequence and
inserts `comment.thread.updated` in the configuration transaction. Its payload
contains the new status and effective policy, so connected clients can update
their controls without reconnecting.

## Fan-out

Every realtime instance subscribes to lifecycle subjects without a shared queue group so each instance can deliver an event to its own local connections. Ephemeral typing signals use non-durable NATS subjects scoped by thread and are never written to the outbox.

Typing uses `comment.realtime.typing.<thread_uuid>` over Core NATS. Realtime
instances subscribe to that wildcard and to every lifecycle subject with plain
subscriptions, never a shared queue group.

At-least-once delivery means clients and hubs deduplicate by `event_id` and apply state by sequence/version. WebSocket delivery can still be interrupted; `comment/changes` is the recovery source.
