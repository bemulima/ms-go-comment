# Attachment lifecycle

Comment authorizes metadata and ownership; `ms-go-filestorage` owns bytes.

## Upload

1. The authenticated uploader selects an existing thread.
2. Comment resolves effective policy and rejects upload when images are disabled.
3. It validates size, content signature, supported MIME, and image dimensions before accepting the media.
4. Comment allocates an attachment UUID and uploads the blob to FileStorage as temporary `USER_MEDIA` with that UUID as `owner_id` and a finite TTL.
5. It stores a pending attachment bound to uploader and thread.

The implemented upload endpoint is multipart
`POST /api/v1/comment-attachment/upload` with `thread_id` and one `file` part.
The whole request is hard-limited to 26 MiB. The effective thread policy may be
smaller. Comment decodes the actual header and dimensions before FileStorage;
the browser filename, extension, and Content-Type are not trusted. JPEG, PNG,
and WebP are the only accepted formats. A user may have at most the effective
`max_attachments` unexpired pending uploads in one thread.

## Bind and activate

- Comment creation may reference only pending attachments owned by the actor and same thread.
- Binding, comment creation, sequence allocation, and outbox insertion are one database transaction.
- A worker activates the FileStorage object idempotently after commit, marks the attachment ready, advances the owner comment/thread sequence/version, and emits `comment.attachment.ready`.
- Activation failures are retried and eventually become explicit `failed` state; they never silently report ready.

The worker stores attempt count, next retry, and bounded error evidence in
PostgreSQL. Backoff starts at five seconds, caps at five minutes, and defaults to
five activation attempts. A `ready`/`failed` transition and its comment sequence
plus outbox event are one transaction. FileStorage calls occur before that short
transaction and are safe to repeat.

## Read and delete

- Clients receive attachment IDs, safe metadata, and status, not unrestricted FileStorage ownership.
- Signed URL requests first authorize the comment/thread, then request a short-lived GET URL from FileStorage.
- Unbound temporary objects expire automatically. Bound deletion is an asynchronous, idempotent lifecycle operation.
- Disabling future images does not revoke already ready media; hiding a comment denies its regular attachment URL path.

`GET /api/v1/comment-attachment/signed-url/{attachmentID}` accepts no URL/TTL
from the caller. It authorizes a ready attachment through its active comment and
returns a server-bounded GET URL, five minutes by default.

`DELETE /api/v1/comment-attachment/delete/{attachmentID}` first commits local
`deleted` state. For a bound attachment it requires uploader/comment authorship,
a writable thread, and the edit window; it refuses to leave a comment with
neither text nor another attachment. The worker then deletes FileStorage bytes;
404 is treated as idempotent success. Expired unbound pending rows enter the same
cleanup path, while terminal failed rows remain as operational evidence.
