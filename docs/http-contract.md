# HTTP contract

The service follows the route separation established by `ms-go-course`:

- `/api/v1`: authenticated user operations;
- `/admin/v1`: role-protected configuration and moderation;
- `/internal/v1`: shared-token service-to-service operations.

The new service does not expose legacy unversioned aliases.

Every implemented `/api/v1` route requires gateway-supplied `X-User-ID` as a
UUID and a non-empty, non-`GUEST` `X-User-Role`. Identity fields in request JSON
are unknown fields and are rejected. JSON bodies are limited to one MiB,
decoded with unknown-field rejection, and must contain exactly one object.
For a `context_grant` space the same request also carries the opaque
`X-Comment-Access-Grant`; the service matches it to that actor and exact
space/resource tuple.

## Authenticated API

```http
PUT    /api/v1/thread/ensure
GET    /api/v1/thread/get/{threadID}
GET    /api/v1/comment/list
GET    /api/v1/comment/get/{commentID}
GET    /api/v1/comment/changes
POST   /api/v1/comment/create
PUT    /api/v1/comment/update/{commentID}
DELETE /api/v1/comment/delete/{commentID}
POST   /api/v1/comment-attachment/upload
GET    /api/v1/comment-attachment/signed-url/{attachmentID}
DELETE /api/v1/comment-attachment/delete/{attachmentID}
POST   /api/v1/realtime/ticket
GET    /api/v1/ws
```

`comment/list` accepts `thread_id`, optional `parent_id`, `limit`, and opaque `cursor`. Omitting `parent_id` lists roots; providing it lists direct children. `comment/changes` accepts `thread_id`, `after_sequence`, and bounded `limit`.

Create accepts `thread_id`, optional `parent_id`, `body`, `attachment_ids`, and `idempotency_key`. Parent/root/path/depth/author/status/version/sequence are server-owned.

Update accepts `body` and expected `version`. Delete requires the expected version and returns a tombstone representation.

The implemented REST slice covers the eight thread/comment routes, all three
attachment routes, and realtime ticket minting. `GET /api/v1/ws` does not trust
gateway actor headers; it authenticates only through a short-lived single-use
ticket carried in `Sec-WebSocket-Protocol`.

### Thread DTOs

`PUT /thread/ensure` accepts:

```json
{
  "space_key": "course",
  "resource_type": "lesson",
  "resource_id": "01JEXTERNALRESOURCE"
}
```

It returns `200` for both create and replay. The thread response includes its
resource tuple, lifecycle status, counters, `last_sequence`, timestamps, and the
fully resolved effective policy. `GET /thread/get/{threadID}` returns the same
projection. Disabled/hidden ownership is reported as not found. A
`context_grant` space returns `comment_access_required` until a trusted grant is
present; knowing the resource tuple is never treated as authorization.

### Comment DTOs

`POST /comment/create` accepts only:

```json
{
  "thread_id": "00000000-0000-0000-0000-000000000001",
  "parent_id": null,
  "body": "Markdown subset with https://example.com links",
  "attachment_ids": [],
  "idempotency_key": "00000000-0000-0000-0000-000000000002"
}
```

A new comment returns `201`; an identical idempotent replay returns `200` and
the original representation. Reusing the key with a different thread, parent,
body, or attachment set returns `idempotency_conflict`. Non-empty attachment IDs
must already be pending, unexpired, same-thread objects owned by the actor; the
upload route that creates them is implemented in the attachment slice.

`PUT /comment/update/{commentID}` accepts `{"body":"...","version":1}`.
`DELETE /comment/delete/{commentID}` accepts `{"version":1}`. Both return `200`.
Only the author may mutate an active comment during the effective edit window.
Delete clears stored content and returns a tombstone while preserving placement
and descendants.

Comment responses expose server-owned `author_id`, placement, extracted links,
safe attachment metadata, lifecycle status, version, sequence, counters, and
timestamps. They never expose FileStorage ownership credentials or unrestricted
download URLs.

### Pagination and reconciliation

`comment/list` defaults to 20 and accepts at most 100 items. Its base64url cursor
is opaque and scoped to the requested `thread_id` plus nullable `parent_id`, so a
cursor cannot be reused for another tree edge. The response is:

```json
{"items": [], "next_cursor": null}
```

`comment/changes` defaults `after_sequence` to zero and returns:

```json
{"items": [], "next_after_sequence": 0, "has_more": false}
```

It projects the latest durable state of comment aggregates changed after the
sequence, ordered by sequence. Clients keep reconciling while `has_more=true`.

### Attachment DTOs

Upload uses `multipart/form-data` with a UUID `thread_id` field and one `file`
part. It returns `201` with safe attachment metadata in `pending` state. The
response MIME and dimensions come from server-side decoding, not multipart
headers. The later comment create call binds returned attachment IDs.

Signed URL returns `200`:

```json
{"url":"https://storage.example/...","expires_at":"2026-08-05T12:05:00Z"}
```

Only `ready` attachments belonging to active, authorized comments qualify.
Delete returns `200` with attachment status `deleted`; physical FileStorage
cleanup is asynchronous and repeat requests return the same local outcome.

### Realtime ticket DTO

`POST /realtime/ticket` accepts only:

```json
{"thread_id":"00000000-0000-0000-0000-000000000001","last_sequence":42}
```

`last_sequence` is optional and non-negative. The endpoint authorizes the
thread using the gateway actor and returns `201` with an opaque ticket,
`expires_at`, and protocol `comment.v1`. TTL is at most 30 seconds. The ticket
is never accepted in a query string and can complete only one WebSocket
handshake.

## Admin API

```http
POST   /admin/v1/space/create
GET    /admin/v1/space/get/{spaceID}
GET    /admin/v1/space/list
PUT    /admin/v1/space/update/{spaceID}
DELETE /admin/v1/space/delete/{spaceID}
GET    /admin/v1/thread/list
PUT    /admin/v1/thread/update/{threadID}
PUT    /admin/v1/comment/hide/{commentID}
PUT    /admin/v1/comment/restore/{commentID}
```

Space/thread configuration routes are implemented. `ADMIN` may create, update,
and soft-disable spaces and update thread status/policy. `ADMIN` and `MODERATOR`
may read/list spaces and threads; `MODERATOR` cannot change configuration.
Space keys and creators are immutable. Delete is an idempotent soft-disable.
List routes accept `limit` (default 20, maximum 100) and non-negative `offset`;
thread list additionally accepts optional `space_id` and status filters.
Create and update requests contain the complete space policy. Thread updates
contain `status` plus `policy_overrides`; every override field is nullable and a
null/missing field inherits the current space policy. Responses expose the
effective policy. A successful thread update increments its durable sequence and
atomically emits `comment.thread.updated` with status and effective policy.
Comment hide/restore routes are implemented for both `ADMIN` and `MODERATOR`;
all other roles receive `comment_forbidden`. They accept no request body. Hide
transitions active comments to hidden and restore transitions hidden comments
to active; repeating the achieved state is idempotent, while deleted comments
return `comment_moderation_conflict`. The transition increments comment version
and thread sequence and atomically emits `comment.hidden` or `comment.restored`.
Hidden content and attachments remain stored but are excluded from regular user
reads and signed URLs. Change reconciliation includes a redacted hidden
tombstone so clients can advance through the moderation sequence.

## Internal API

```http
POST /internal/v1/access-grant/create
POST /internal/v1/thread/ensure
GET  /internal/v1/thread/get-by-resource
```

All internal routes require the exact configured `X-Internal-Token`. Health remains `GET /healthz` outside these groups.

Internal routes are implemented and callable only on the trusted service
network. They do not accept gateway actor headers as authorization.

`POST /internal/v1/access-grant/create` accepts:

```json
{
  "issuer": "ms-go-course",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "space_key": "course.private",
  "resource_type": "lesson",
  "resource_id": "lesson-1",
  "permissions": {"read": true, "write": true, "upload": false},
  "expires_in_seconds": 300
}
```

It returns `201` with `grant`, `expires_at`, and the effective permission object.
The active space must use `access_mode=context_grant`. TTL is from one second to
`ACCESS_GRANT_MAX_TTL_SECONDS`; every valid permission set contains `read`, and
`upload` also requires `write`.

`POST /internal/v1/thread/ensure` accepts the same `space_key`,
`resource_type`, and `resource_id` tuple and returns the normal thread
projection with its effective policy. It is idempotent. The lookup equivalent is:

```http
GET /internal/v1/thread/get-by-resource?space_key=course.private&resource_type=lesson&resource_id=lesson-1
```

Both thread operations work only for active `context_grant` spaces. The grant
is presented by the browser only on `/api/v1` REST calls and realtime-ticket
minting; it is never accepted on `/api/v1/ws`.
