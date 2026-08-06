# Comment business rules

## Authentication and ownership

- Every student-facing read or write requires a non-empty UUID `X-User-ID` and a non-guest role supplied by the trusted gateway.
- Request bodies never select the acting user.
- `author_id` and `uploader_id` are immutable and have no foreign key to another service database.
- Administrators and moderators may perform explicitly registered moderation actions; elevated role does not bypass route registration or target access checks implicitly.
- In `context_grant` spaces, gateway identity is necessary but insufficient.
  The request also needs a non-expired grant bound to that actor and exact
  space/resource tuple.
- Private grants always include `read`; `upload` requires `write`. Reads require
  `read`, mutations require `write`, and new image uploads require `upload`.
- Knowing a private resource tuple or thread UUID never grants access. Only a
  trusted internal caller may ensure/resolve its thread and mint a grant.

## Threads and policy

- A space key is globally unique, lowercase, and stable.
- `(space_id, resource_type, resource_id)` uniquely identifies one thread.
- Effective policy is a thread override followed by the space default.
- `open` permits reads and writes, `read_only` and `closed` permit reads only, and `hidden` denies regular reads.
- Disabling new images does not make already active attachments disappear.

Recommended first-slice defaults are: links enabled, images disabled, maximum depth 10, hard maximum depth 32, body length 10,000 characters, four images, five MiB per image, and a 15-minute author edit/delete window.

## Comment tree

- A new comment is either a root (`parent_id = null`, depth 0) or a reply to one active/visible comment in the same thread.
- Parent, root, path, and depth are computed server-side in one transaction and never accepted as authoritative request fields.
- Comments cannot be moved to a different parent or thread.
- Reads paginate roots or direct children. Clients construct the visible tree lazily instead of requesting an unbounded recursive response.
- Soft deletion produces a tombstone and preserves the row, identity, path, counters, and descendants.

## Moderation

- `ADMIN` and `MODERATOR` may invoke the explicit hide/restore routes; all other roles are denied.
- Hide changes only `active` to `hidden`; restore changes only `hidden` to `active`. Repeating the achieved state is idempotent.
- Deleted comments cannot be hidden or restored. Hiding preserves content, attachments, placement, and counters in storage.
- Regular reads and attachment signed URLs never expose hidden content. Change reconciliation returns only a redacted hidden tombstone with identity, status, version, and sequence.
- Every actual transition increments comment version and thread sequence and inserts its lifecycle event in the same transaction.

## Content

- A comment must contain non-blank text or at least one valid attachment.
- Raw HTML is rejected. The stored source format is a documented Markdown subset.
- Only `http` and `https` links are accepted. When links are disabled, content containing a link is rejected.
- Renderers must sanitize output and add `rel="ugc noopener noreferrer"` to external anchors.
- SVG and executable media are outside the first slice. Allowed images are JPEG, PNG, and WebP after signature, size, and dimension checks.

## Concurrency and delivery

- Create accepts a UUID idempotency key unique per author. Repeating it returns the original outcome.
- Edit/delete accepts the current integer version and returns conflict on stale input.
- Every durable thread mutation increments `thread.last_sequence` and stores the allocated sequence on the changed aggregate.
- Domain state and its outbox event commit in the same PostgreSQL transaction.
- WebSocket delivery is a low-latency projection, not the source of truth. REST reconciliation is required after gaps.
- Authenticated REST and realtime-ticket requests pass a bounded per-instance
  actor token bucket. Exhaustion returns `rate_limited` with `Retry-After: 1`.
  WebSocket upgrades use their separate per-user connection bound. Gateway or
  distributed limits remain required for coordinated multi-instance abuse
  control.
