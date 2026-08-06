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
