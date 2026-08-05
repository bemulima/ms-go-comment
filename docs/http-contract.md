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

The first implemented REST slice covers the eight thread/comment routes above.
Attachment upload/signed URL/delete, realtime ticket, and WebSocket routes remain
reserved for their dedicated slices.

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

Space writes and moderation require `ADMIN` or `MODERATOR`. Read-only administration may also allow explicitly approved roles.

## Internal API

```http
POST /internal/v1/access-grant/create
POST /internal/v1/thread/ensure
GET  /internal/v1/thread/get-by-resource
```

All internal routes require the exact configured `X-Internal-Token`. Health remains `GET /healthz` outside these groups.
