# Agent contract map

This is the final navigation map for the first backend slice. Agents must use it
to locate a rule, then follow the linked source-of-truth document and code. It
describes the current runtime, not an aspirational API.

## Boundary and ownership

| Concern | Owner | Contract |
| --- | --- | --- |
| Discussion spaces, thread/resource binding, comment tree and moderation state | `ms-go-comment` | No foreign keys or synchronous resource lookup in a host database |
| User identity and role | `ms-gateway` | REST trusts only gateway-replaced `X-User-ID` and `X-User-Role` |
| Host-resource authorization | Host service + `ms-go-comment` grant verification | Host authorizes first; Comment enforces a short-lived user/resource grant |
| Attachment bytes | `ms-go-filestorage` | Comment owns binding/authorization metadata and requests temporary/active/signed operations |
| Durable realtime delivery | PostgreSQL outbox + NATS JetStream | Domain mutation and outbox insert commit atomically; delivery is at least once |
| Browser realtime session | `ms-go-comment` WebSocket | One thread per single-use ticket; REST remains the source of truth |

The integration key is the exact, opaque tuple
`space_key + resource_type + resource_id`. The service never owns Course,
Lesson, Product, Article, Page, or User rows.

## Implemented in backend v1

| Process | Entry point | Invariants and outcome |
| --- | --- | --- |
| Ensure thread | `PUT /api/v1/thread/ensure` | Idempotent on space/resource tuple; resolves effective policy |
| Read thread | `GET /api/v1/thread/get/{threadID}` | Hidden/disabled ownership is reported as not found |
| Read tree edge | `GET /api/v1/comment/list` | Roots or direct children only; opaque scoped cursor; maximum 100 |
| Reconcile changes | `GET /api/v1/comment/changes` | Monotonic per-thread sequence; repeat until `has_more=false` |
| Create comment/reply | `POST /api/v1/comment/create` | Server computes placement; UUID idempotency key is per author; state + outbox are atomic |
| Edit/delete | `PUT .../update/{id}`, `DELETE .../delete/{id}` | Author, edit window, expected version; delete preserves a tombstone and descendants |
| Attachment lifecycle | upload, signed-url, delete routes | JPEG/PNG/WebP only; policy and ownership checks; FileStorage bytes never become Comment-owned |
| Mint realtime ticket | `POST /api/v1/realtime/ticket` | Maximum 30 seconds, stored hashed, single-use and thread-scoped |
| Realtime projection | `GET /api/v1/ws` | `comment.v1` + `ticket.<opaque>` subprotocols; exact Origin allowlist; bounded connections/frames/queue |
| Durable delivery | background outbox dispatcher | Finite lease, JetStream `event_id` deduplication, bounded retry evidence |
| Attachment cleanup | background worker | Idempotent activation/deletion and explicit ready/failed/deleted states |
| Admin configuration | `/admin/v1/space/*`, `/admin/v1/thread/*` | ADMIN writes; ADMIN/MODERATOR reads; space delete is soft-disable |
| Comment moderation | `/admin/v1/comment/hide/*`, `/admin/v1/comment/restore/*` | ADMIN/MODERATOR; idempotent transitions; redacted hide event |
| Private-resource grant | `POST /internal/v1/access-grant/create` | Exact internal token; hash-only bounded grant tied to issuer/user/resource/permissions |
| Trusted private thread | `/internal/v1/thread/ensure`, `/internal/v1/thread/get-by-resource` | Active `context_grant` spaces only; opaque host tuple remains the integration key |

Gateway-facing routes are `/api/comment/v1/*`. Student REST rewrites to
`/api/v1/*`; the exact `/api/comment/v1/ws` route preserves Origin, upgrade, and
subprotocol headers without bearer authentication. The admin gateway rewrite to
`/admin/v1/*` exposes configuration administration and comment moderation.

## Business rules

- An actor is a non-guest UUID identity supplied by the trusted gateway; bodies
  cannot select `author_id` or `uploader_id`.
- A reply parent must be active, visible, in the same thread, and below the
  effective maximum depth. Placement never changes after creation.
- Content requires non-blank text or an attachment. Raw HTML is rejected. Only
  HTTP(S) links are allowed and space/thread policy may disable links or images.
- Create is idempotent. Mutations use optimistic versioning. Every durable
  mutation advances `comment_thread.last_sequence` in its database transaction.
- WebSocket is a projection. A sequence gap always returns the client to
  `GET /api/v1/comment/changes`; clients deduplicate events by `event_id`.

Detailed rule sources are [business-rules.md](business-rules.md),
[attachment-lifecycle.md](attachment-lifecycle.md), and
[websocket-contract.md](websocket-contract.md).

## Storage and delivery contracts

| Table | Responsibility |
| --- | --- |
| `comment_space` | Stable integration key, lifecycle, Origin allowlist and default policy |
| `comment_thread` | Opaque resource tuple, overrides, counters and sequence |
| `comment` | Immutable tree placement, content, optimistic version and tombstone state |
| `comment_attachment` | FileStorage identity, binding, media evidence and retry state |
| `comment_outbox` | Versioned durable event, finite lease, attempts and publication evidence |
| `comment_ws_ticket` | SHA-256 ticket hash, actor/thread/permissions and short expiry |
| `comment_access_grant` | SHA-256 grant hash, issuer/user/resource/permissions and bounded expiry |

Lifecycle subjects are `comment.created`, `comment.updated`, `comment.deleted`,
`comment.hidden`, `comment.restored`, `comment.attachment.ready`,
`comment.attachment.failed`, and `comment.thread.updated`. Public WebSocket event
names shorten the last three to `attachment.ready`, `attachment.failed`, and
`thread.updated`.

## Deferred after backend v1

These items are not registered in the current runtime:

- rate limiting beyond the implemented per-user WebSocket connection bound;
- production observability dashboards and multi-instance load validation.

An agent must create a new issue and implement the use case, adapter, tests, docs,
and `.ai/contracts` status together before changing any item above to
implemented.

## Change routing for agents

| Change | Start in | Also verify |
| --- | --- | --- |
| Entity/invariant | `internal/domain` | migration, repository port, error and event contracts |
| Business process | `internal/usecase` | transaction boundary, handler, tests and outbox payload |
| REST shape | `internal/adapters/http` | `docs/http-contract.md`, `.ai/contracts/http.yaml`, gateway rewrite |
| Persistence | `db/migrations` + `internal/adapters/postgres` | reversible pair and migration contract tests |
| WebSocket | `internal/adapters/websocket` | ticket use case, Origin/subprotocol rules and reconciliation |
| NATS event | domain outbox + NATS adapter | `.ai/contracts/events.yaml`, consumer mapping and at-least-once behavior |
| Attachment | attachment use case + FileStorage adapter | policy, cleanup retries and signed authorization |

Stable error codes are in [error-contract.md](error-contract.md). Machine-readable
contracts under `.ai/contracts` must stay synchronized; `make validate-contracts`
and `go test ./...` enforce the checked-in map.
