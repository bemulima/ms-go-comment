# Frontend v1 agent contract map

This is the release map for the browser SDK and `<ms-comment-thread>`. Read it
with [agent-contract-map.md](agent-contract-map.md),
[frontend-integration.md](frontend-integration.md), and the machine-readable
`.ai/contracts/frontend.yaml` contract before changing frontend behavior.

## Ownership and boundaries

| Concern | Authority | Frontend responsibility |
| --- | --- | --- |
| Host resource access | Integrating host | Resolve access server-side; request a bounded grant for private resources |
| Browser identity | Gateway session | Send `credentials: include`; never create identity headers |
| Space/thread policy | Comment backend | Display effective-policy guidance and controls; never authorize locally |
| Comment/tree state | Comment backend | Project DTOs, paginate each edge, reconcile by sequence/version |
| Attachment bytes | FileStorage behind Comment | Upload through Comment; render only Comment-authorized signed URLs |
| Durable realtime | PostgreSQL/outbox/NATS | Treat WebSocket as a projection and REST changes as recovery authority |
| User profiles | Host/User service | The widget exposes author UUIDs; host profile composition stays external |

The frontend accepts no internal token. `X-Comment-Access-Grant` is the only
optional package-owned header. A grant is assigned through the `accessGrant`
JavaScript property and must never enter markup, URLs, storage, logs, or
analytics. WebSocket authentication uses only `comment.v1` and
`ticket.<opaque>` subprotocols.

## Implemented modules

| Module | Source | Contract |
| --- | --- | --- |
| Typed DTOs | `web/src/contracts.ts` | Thread policy, comments, attachments, pages, changes and realtime envelopes |
| REST client | `web/src/client.ts` | Gateway credentials, grants, stable API errors and all user endpoints |
| Realtime client | `web/src/realtime.ts` | Ticket minting, socket lifecycle, ping/pong, gap recovery and reconnect |
| Web Component | `web/src/element.ts` | Resource projection, tree, composer, media, accessibility, theme and live UX |
| Package entry | `web/src/index.ts` | Headless exports only; DOM component remains the explicit `./element` subpath |
| Reference host | `web/examples/vanilla.html` | Property-only grant and opt-in realtime integration |

Importing the headless entry has no DOM side effects. Importing `./element`
still requires explicit `defineCommentThreadElement()` registration.

## End-to-end processes

### Public resource

1. Host mounts the element with gateway base path plus exact
   `(space_key, resource_type, resource_id)` tuple.
2. Component calls thread ensure, then lists root comments with an opaque cursor.
3. Each expanded comment lists only its direct children with a cursor scoped to
   that parent. Every level is independent and lazy.
4. With `realtime` enabled, the component mints a single-use ticket after REST
   state exists and opens the derived `ws:`/`wss:` endpoint.

### Private resource

1. Trusted host authorizes the current user/resource server-side.
2. Host calls Comment internal grant create with its internal credential.
3. Browser receives only the short-lived opaque grant and assigns
   `element.accessGrant`.
4. REST and ticket minting carry the grant header. The socket URL, protocols,
   events, and DOM never carry the grant.
5. Resource or grant changes abort old requests, stop the old socket, clear the
   old projection, and ensure the new thread.

### Create root or reply

1. UI reads effective thread status/policy and provides advisory limits.
2. Backend remains authoritative for writability, depth, content, link, media,
   ownership, idempotency, and access decisions.
3. Allowed images are uploaded first and return pending attachment IDs.
4. Create sends server-owned placement/author fields nowhere; it sends thread,
   optional parent, body, attachment IDs, and one UUID idempotency key.
5. Explicit 4xx rejection deletes known staged attachments. A network/5xx
   outcome is ambiguous: lock the draft and retry the exact payload/key without
   re-uploading or destructive cleanup.
6. A successful response is projected immediately and later realtime/REST
   copies deduplicate by comment ID.

### Realtime and recovery

1. SDK reports connecting, connected, reconnecting, and stopped states.
2. Application `ping` receives `pong`; textarea activity emits bounded
   `typing.start`/`typing.stop` only while the socket is ready.
3. Sequence gaps and `resync_required` paginate REST changes until `has_more`
   is false. Reconnect always mints a new single-use ticket at the latest known
   sequence.
4. Durable comment/attachment events re-read the authorized comment projection.
   Reply events also refresh a visible parent counter.
5. `thread.updated` re-reads status/effective policy and updates controls.
6. Reconciliation updates content in place. It must preserve draft text,
   selected files, expanded branches, and per-parent cursors.

## REST and WebSocket consumption

| Frontend operation | Backend contract |
| --- | --- |
| Resolve resource | `PUT /api/v1/thread/ensure` |
| Load roots/children | `GET /api/v1/comment/list` with nullable `parent_id` and opaque cursor |
| Re-read live aggregate | `GET /api/v1/comment/get/{commentID}` |
| Gap recovery | `GET /api/v1/comment/changes` by monotonic sequence |
| Create | `POST /api/v1/comment/create` with UUID idempotency key |
| Stage image | `POST /api/v1/comment-attachment/upload` multipart |
| Render ready image | `GET /api/v1/comment-attachment/signed-url/{attachmentID}` |
| Cleanup known rejected stage | `DELETE /api/v1/comment-attachment/delete/{attachmentID}` |
| Mint live credential | `POST /api/v1/realtime/ticket` |
| Live projection | `GET /api/v1/ws` via subprotocol ticket |

The SDK also exposes get/update/delete APIs, but edit/delete UI is not part of
the current component. Admin configuration and moderation remain trusted
backoffice concerns, not arbitrary host-page controls.

## Component contract

- Attributes: `base-url`, `space-key`, `resource-type`, `resource-id`,
  `heading`, `page-size`, and boolean `realtime`.
- Property-only secret: `accessGrant`.
- Methods: `refresh()`, `loadMore()`, `loadReplies(commentID)`.
- Lifecycle events: `ms-comment-ready`, `ms-comment-error`,
  `ms-comment-realtime-state`, `ms-comment-reconciled`,
  `ms-comment-thread-updated`.
- Interaction events: `ms-comment-select`, `ms-comment-reply-start`,
  `ms-comment-replies-loaded`, `ms-comment-attachment-uploaded`,
  `ms-comment-created`, `ms-comment-typing`.

All events bubble and are composed. Theme tokens and stable shadow parts are
listed in [frontend-integration.md](frontend-integration.md).

## Rendering, accessibility, and security invariants

- Bodies, filenames, metadata, errors, and tombstones use `textContent`.
- Raw HTML is never interpreted. Deleted/hidden content never leaks through
  links, attachments, attributes, events generated by the renderer, or labels.
- Only server-extracted absolute HTTP(S) links become anchors, with
  `noopener noreferrer nofollow ugc`.
- Only authorized signed HTTP(S) URLs become image sources; images use
  `no-referrer`, lazy loading, bounded layout, and safe filename alt text.
- Native form, button, details/summary, label, focus, busy, atomic live-region,
  and alert semantics are part of the stable accessibility contract.
- Host theme overrides must retain contrast and visible focus. Host owns page
  language, heading hierarchy, surrounding landmarks, zoom, and reflow.
- Exact Origin allowlisting and CSP `connect-src`/`img-src` are deployment
  requirements; they are not weakened by browser code.

## State and race rules

- Every resource refresh increments a generation and aborts the old controller.
- Root, reply, media, mutation, thread, and realtime results check current
  generation/ownership before touching live DOM.
- Root and every parent keep separate cursors and loading flags.
- Comment IDs deduplicate REST pages, create responses, gap changes, and events.
- Realtime toggle starts/stops transport in place and never reloads a resource.
- Disconnect stops timers, typing, socket, and outstanding REST/media work.
- Reconnect is projection recovery, never a durable mutation retry.

## Validation and agent change routing

Run `npm --prefix web test`, `npm --prefix web run typecheck`,
`npm --prefix web run build`, `npm audit --prefix web`, `npm pack --dry-run`,
`go test ./...`, `make validate-contracts`, `golangci-lint run ./...`, and
`git diff --check` before publishing frontend contract changes.

| Change | Start in | Synchronize |
| --- | --- | --- |
| DTO/REST method | `web/src/contracts.ts` / `client.ts` | HTTP docs/YAML, SDK tests, this map |
| Ticket/reconnect/recovery | `web/src/realtime.ts` | WebSocket docs/YAML, SDK tests, component states |
| Tree/composer/media UX | `web/src/element.ts` | frontend YAML, DOM tests, integration guide |
| Attribute/property/event | component + `web/README.md` | example, integration guide, contract tests |
| Token/shadow part/a11y | component styles/DOM | integration guide and DOM tests |
| Host security flow | integration guide | backend access/WebSocket contracts and this map |

## Deferred scope

- Author edit/delete controls in the Web Component.
- Admin space/thread configuration and moderation UI.
- Host-specific profile resolution, localization, analytics, and framework
  wrappers.
- Deployment-specific gateway manifests, secrets, dashboards, multi-instance
  rate limiting, and browser/load matrices.

These items require new issues and must not move backend authorization or policy
authority into the browser.
