# Comment domain map

This document is the entrypoint for every agent and developer changing `ms-go-comment`.

## Service boundary

The service owns authenticated discussions attached to opaque external resources. It does not own users, courses, lessons, products, articles, or page authorization rules.

A discussion target is identified by the exact tuple:

```text
space_key + resource_type + resource_id
```

No foreign key or synchronous existence check is made against a host product database. `resource_id` is case-sensitive and must be stable for the lifetime of the host resource.

## Domain objects

```text
CommentSpace
└── CommentThread
    └── Comment (root)
        └── Comment (reply)
            └── Comment (reply at any allowed depth)

CommentThread
├── CommentAttachment
├── RealtimeTicket
└── monotonically increasing sequence

Database transaction
└── OutboxEvent
    └── NATS JetStream
        └── WebSocket hubs
```

- `CommentSpace` configures integration origin, access mode, and default content policy.
- `CommentThread` binds one space to one opaque resource and may override selected policy values.
- `Comment` is immutable in identity and placement. Editing changes content/version; replies are never moved.
- `CommentAttachment` stages and authorizes user media while FileStorage owns bytes.
- `OutboxEvent` makes committed mutations eventually publishable without losing them during a broker outage.
- `RealtimeTicket` authenticates one browser WebSocket handshake without exposing an access token in the URL.

## Business processes

1. Ensure or resolve a thread for an external resource.
2. Authorize read/write/upload for the authenticated user and target.
3. List roots or direct replies with cursor pagination.
4. Create a root or reply atomically with idempotency and sequence allocation.
5. Edit with optimistic locking.
6. Soft-delete to a tombstone so descendants retain their parent chain.
7. Stage, bind, activate, authorize, and eventually remove image attachments.
8. Publish committed lifecycle events through an outbox.
9. Fan events out over ticket-authenticated WebSockets.
10. Reconcile missed realtime frames through the REST changes feed.
11. Configure spaces/threads and moderate comments through admin APIs.
12. Grant access to private host resources through trusted internal APIs.

## Contract index

- [business-rules.md](business-rules.md)
- [database-contract.md](database-contract.md)
- [http-contract.md](http-contract.md)
- [websocket-contract.md](websocket-contract.md)
- [attachment-lifecycle.md](attachment-lifecycle.md)
- [realtime-delivery.md](realtime-delivery.md)
- [access-integration.md](access-integration.md)
- [error-contract.md](error-contract.md)

The matching machine-readable evidence lives in `.ai/contracts`.
