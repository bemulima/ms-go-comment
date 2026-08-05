# Error contract

HTTP errors use a stable JSON object:

```json
{
  "error": "comment_edit_conflict",
  "message": "comment was changed by another request",
  "details": {}
}
```

Initial stable codes include:

| Code | Status | Meaning |
| --- | --- | --- |
| `authentication_required` | 401 | Missing authenticated actor |
| `comment_access_required` | 403 | Missing/insufficient private-resource grant |
| `comment_forbidden` | 403 | Actor cannot perform the operation |
| `space_not_found` | 404 | Space does not exist or is unavailable |
| `thread_not_found` | 404 | Thread does not exist or is unavailable |
| `comment_not_found` | 404 | Comment does not exist or is unavailable |
| `attachment_not_found` | 404 | Attachment does not exist or is unavailable |
| `attachment_not_ready` | 409 | Attachment cannot receive a signed URL yet |
| `thread_not_writable` | 409 | Thread is read-only/closed/hidden |
| `comment_edit_conflict` | 409 | Expected version is stale |
| `idempotency_conflict` | 409 | Key was reused with different input |
| `parent_not_found` | 422 | Reply parent is invalid or unavailable |
| `max_depth_exceeded` | 422 | Reply would exceed effective depth |
| `images_disabled` | 422 | Effective policy forbids new images |
| `links_disabled` | 422 | Effective policy forbids links |
| `invalid_comment_content` | 422 | Content/attachments violate the contract |
| `rate_limited` | 429 | Actor or connection limit exceeded |
| `realtime_ticket_invalid` | 401 | Ticket is absent, expired, or already consumed |
| `internal_error` | 500 | Unexpected server-side failure; details are not exposed |

WebSocket `error` frames use the same code vocabulary and include a client request ID when applicable. Internal error text and SQL/FileStorage/NATS details are logged but not exposed to untrusted clients.
