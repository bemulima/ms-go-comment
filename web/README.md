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

The optional `ms-comment-thread` shell renders a safe, read-only first slice on
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
document.querySelector("main")?.append(comments);
```

Public attributes are `base-url`, `space-key`, `resource-type`, `resource-id`,
`heading`, and `page-size`. The private grant is intentionally available only
through the `accessGrant` property. `refresh()` reloads the thread and first
page; `loadMore()` advances cursor pagination.

The element emits bubbling, composed `ms-comment-ready`, `ms-comment-error`,
and `ms-comment-select` events. Comment content is projected with
`textContent`, including deletion tombstones, and is never interpreted as HTML.
Host applications may theme it with `--ms-comment-*` custom properties and the
stable `container`, `heading`, `status`, `list`, `comment`, `comment-select`,
`comment-meta`, `comment-body`, and `load-more` shadow parts. Comment selection
uses native buttons and remains keyboard accessible.

Nested tree rendering, composing, attachments, and live WebSocket binding are
deliberately deferred to later frontend slices.
