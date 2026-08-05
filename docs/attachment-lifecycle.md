# Attachment lifecycle

Comment authorizes metadata and ownership; `ms-go-filestorage` owns bytes.

## Upload

1. The authenticated uploader selects an existing thread.
2. Comment resolves effective policy and rejects upload when images are disabled.
3. It validates size, content signature, supported MIME, and image dimensions before accepting the media.
4. Comment allocates an attachment UUID and uploads the blob to FileStorage as temporary `USER_MEDIA` with that UUID as `owner_id` and a finite TTL.
5. It stores a pending attachment bound to uploader and thread.

## Bind and activate

- Comment creation may reference only pending attachments owned by the actor and same thread.
- Binding, comment creation, sequence allocation, and outbox insertion are one database transaction.
- A worker activates the FileStorage object idempotently after commit, marks the attachment ready, advances the owner comment/thread sequence when required, and emits `attachment.ready`.
- Activation failures are retried and eventually become explicit `failed` state; they never silently report ready.

## Read and delete

- Clients receive attachment IDs, safe metadata, and status, not unrestricted FileStorage ownership.
- Signed URL requests first authorize the comment/thread, then request a short-lived GET URL from FileStorage.
- Unbound temporary objects expire automatically. Bound deletion is an asynchronous, idempotent lifecycle operation.
- Disabling future images does not revoke already ready media; hiding a comment denies its regular attachment URL path.
