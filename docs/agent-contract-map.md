# Agent contract map

This is the final navigation map for Backend v2. Agents must use it
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

## Implemented in Backend v2

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
| HTTP guardrails | global middleware + actor token bucket | Server request ID/security headers/recovery; bounded per-instance user REST/ticket rate limit |

Gateway-facing routes are `/api/comment/v1/*`. Student REST rewrites to
`/api/v1/*`; the exact `/api/comment/v1/ws` route preserves Origin, upgrade, and
subprotocol headers without bearer authentication. The admin gateway rewrite to
`/admin/v1/*` exposes configuration administration and comment moderation.

## Role and access matrix

| Surface | Identity | Additional proof | Allowed operations |
| --- | --- | --- | --- |
| Authenticated user space | Gateway UUID + non-guest role | None | Read; write while thread open; upload while open and images allowed |
| Private-resource space | Gateway UUID + non-guest role | Bound `X-Comment-Access-Grant` | Exact intersection of grant `read/write/upload`, thread state, and policy |
| Admin configuration write | Gateway role `ADMIN` | None | Create/update/disable spaces; update thread status/policy |
| Admin configuration read | `ADMIN` or `MODERATOR` | None | List/get spaces and list threads |
| Moderation | `ADMIN` or `MODERATOR` | None | Hide/restore comments through explicit routes |
| Internal host service | Exact `X-Internal-Token` | Trusted service network | Ensure/get private thread and mint user/resource grant |
| WebSocket browser | Single-use `ticket.<opaque>` subprotocol | Exact allowed Origin | Read projection; typing only when ticket includes write |

Request bodies never select the actor. Admin roles do not implicitly bypass
private-resource checks on user routes, and an internal token is never accepted
as a browser identity or WebSocket credential.

## End-to-end private integration

```text
Host authorizes user/resource
  -> POST /internal/v1/thread/ensure
  -> POST /internal/v1/access-grant/create
  -> browser REST with gateway identity + X-Comment-Access-Grant
  -> POST /api/v1/realtime/ticket with the same proof
  -> GET /api/v1/ws with comment.v1 + ticket.<opaque>
  -> sequence gap returns browser to GET /api/v1/comment/changes
```

The host is authoritative for resource authorization. Comment is authoritative
for grant binding, comment policy/state, attachment authorization, sequences,
and ticket consumption. PostgreSQL is authoritative for durable state; NATS and
WebSocket are delivery projections.

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

## Deferred after Backend v2

These items are not registered in the current runtime:

- distributed cross-instance rate limiting beyond the implemented bounded
  per-instance REST/ticket and WebSocket connection guards;
- production observability dashboards and multi-instance load validation.

Frontend implementation, gateway route rollout, deployment-specific secret
provisioning, dashboards, and distributed/load validation remain separate
tasks. They must consume this map without moving business authorization into
the browser.

## Frontend v1 status

The headless browser SDK and embeddable Web Component tree/composer are
implemented under `web/`. The SDK owns typed REST calls, private-grant
forwarding, single-use WebSocket ticket handshakes, reconnect, and REST
reconciliation. The explicitly registered `<ms-comment-thread>` resolves the
resource tuple, paginates roots and direct-child branches independently, creates
roots/replies, stages policy-allowed images, and projects authorized links/media
safely. Ambiguous network/5xx create outcomes retain the exact attachment IDs
and idempotency key for a deliberate retry; only explicit rejection cleans up
staged media. Bodies, filenames, tombstones, and status messages use `textContent`;
only checked HTTP(S) values become link/image attributes. It never sets gateway
identity headers, accepts no internal token, and accepts private grants only
through a JavaScript property, never through markup. Browser policy checks are
UX guidance; backend policy and authorization remain authoritative. Optional
realtime starts only after REST state, uses single-use subprotocol tickets,
reports connection/reconnect state, answers application ping/pong, applies
comment/policy reconciliation in place, and preserves drafts plus loaded
branches. Edit/delete controls remain outside the current widget slice.

The deployable host contract is [frontend-integration.md](frontend-integration.md).
It fixes the gateway/private-grant boundary, exact Origin and CSP requirements,
component lifecycle, events, theme tokens, shadow parts, and host accessibility
responsibilities. `web/examples/vanilla.html` is the package-relative reference
integration; grants remain property-only in every example.

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
| Web Component | `web/src/element.ts` | `.ai/contracts/frontend.yaml`, per-branch cursors, composer, staged media, in-place realtime, safe projection and DOM tests |
| Host integration | `docs/frontend-integration.md` | gateway rewrite, grant property, Origin/CSP, lifecycle, tokens/parts, accessibility and vanilla example |

Stable error codes are in [error-contract.md](error-contract.md). Machine-readable
contracts under `.ai/contracts` must stay synchronized; `make validate-contracts`
and `go test ./...` enforce the checked-in map.
