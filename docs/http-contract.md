# HTTP contract

The service follows the route separation established by `ms-go-course`:

- `/api/v1`: authenticated user operations;
- `/admin/v1`: role-protected configuration and moderation;
- `/internal/v1`: shared-token service-to-service operations.

The new service does not expose legacy unversioned aliases.

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
