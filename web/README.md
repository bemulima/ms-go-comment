# `@bemulima/ms-comment-client`

Framework-agnostic browser SDK for `ms-go-comment`. It owns transport and
reconciliation only; rendering and host-resource authorization stay outside the
package.

```ts
import { CommentClient, CommentRealtimeClient } from "@bemulima/ms-comment-client";

const client = new CommentClient({
  baseURL: "/api/comment/v1",
  accessGrant: () => currentPrivateResourceGrant,
});

const thread = await client.ensureThread("course.private", "lesson", "lesson-1");
const realtime = new CommentRealtimeClient({
  client,
  threadID: thread.id,
  onEvent: (event) => console.log(event),
  onChanges: (comments) => reconcileStore(comments),
});
await realtime.start(thread.last_sequence);
```

The SDK always uses `credentials: include`. It never accepts or sends
`X-User-ID`, `X-User-Role`, or internal tokens. An optional private-resource
grant is sent only as `X-Comment-Access-Grant`. WebSocket tickets are carried
only in the `Sec-WebSocket-Protocol` values `comment.v1` and `ticket.<opaque>`.

Commands:

```sh
npm install
npm run typecheck
npm test
npm run build
```

## Embeddable Web Component

The optional `ms-comment-thread` renders a safe discussion tree and composer on
any host page. Registration is explicit, so importing the headless SDK has no
DOM side effects.

```ts
import { defineCommentThreadElement } from "@bemulima/ms-comment-client/element";

defineCommentThreadElement();

const comments = document.createElement("ms-comment-thread");
comments.spaceKey = "course.private";
comments.resourceType = "lesson";
comments.resourceID = "lesson-1";
comments.accessGrant = currentPrivateResourceGrant; // property only; never markup
comments.realtime = true; // opt in to ticket-authenticated live projection
document.querySelector("main")?.append(comments);
```

Public attributes are `base-url`, `space-key`, `resource-type`, `resource-id`,
`heading`, `page-size`, and boolean `realtime`. The private grant is intentionally available only
through the `accessGrant` property. `refresh()` reloads the thread and first
page; `loadMore()` advances root pagination; `loadReplies(commentID)` opens a
branch and advances its independent direct-child cursor.

The composer creates roots and replies with a fresh UUID idempotency key. It
uses the effective thread policy to expose character, link, depth, image count,
type, and size guidance, while the backend remains authoritative. Allowed
JPEG/PNG/WebP files are staged before comment creation and their IDs are bound
by the create request. Staged files are deleted after an explicit rejected
create. A network/5xx ambiguity retains the exact attachment IDs and idempotency
key behind a locked retry action, preventing a blind duplicate or destructive
cleanup. The composer is hidden for non-open threads.

The element emits bubbling, composed `ms-comment-ready`, `ms-comment-error`,
`ms-comment-select`, `ms-comment-reply-start`,
`ms-comment-replies-loaded`, `ms-comment-attachment-uploaded`, and
`ms-comment-created` events. With realtime enabled it also emits
`ms-comment-realtime-state`, `ms-comment-reconciled`, `ms-comment-typing`, and
`ms-comment-thread-updated`.

Realtime starts after the initial REST projection and stops on refresh,
disconnect, resource change, or grant change. The SDK exposes
connecting/connected/reconnecting/stopped states, remints single-use tickets,
answers application ping/pong, and reconciles sequence gaps through REST. The
component applies comment and policy changes in place, preserving the current
draft, selected files, and loaded branches. Textarea activity sends bounded
typing start/stop signals.

Comment bodies and filenames use `textContent`.
Only server-extracted HTTP(S) links become hardened anchors; ready images use
authorized signed URLs after a second HTTP(S) check. Deleted and hidden content
is shown as a tombstone and never interpreted as HTML.

Host applications may theme it with `--ms-comment-*` custom properties and
stable shadow parts including `container`, `heading`, `status`, `realtime-status`, `list`,
`comment`, `comment-content`, `comment-actions`, `comment-select`, `reply`,
`replies`, `replies-toggle`, `comment-meta`, `comment-body`, `comment-links`,
`comment-attachments`, `attachment-image`, `composer`, `composer-input`, and
`load-more`. Native buttons, details/summary, labels, and live regions preserve
keyboard and screen-reader semantics.

Edit/delete controls remain outside the current widget slice.

The complete gateway, private-resource, CSP/Origin, lifecycle, event, theme,
part, and accessibility contract is in
[`docs/frontend-integration.md`](../docs/frontend-integration.md). A runnable
package-relative example is in [`examples/vanilla.html`](examples/vanilla.html).
