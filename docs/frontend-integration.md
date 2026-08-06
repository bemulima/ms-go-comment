# Frontend integration

`@bemulima/ms-comment-client` can be consumed as a headless SDK or through the
framework-independent `<ms-comment-thread>` Web Component. The host owns its
resource authorization and user-facing profile data; Comment owns discussion
policy, comment state, media authorization, sequencing, and realtime tickets.

## Gateway and identity

Expose the service only behind the trusted gateway. Rewrite a stable host path
such as `/api/comment/v1` to Comment `/api/v1`; keep browser cookies/session
credentials on the host origin. The browser package never accepts
`X-User-ID`, `X-User-Role`, or `X-Internal-Token`.

For a private resource, the host backend resolves authorization and calls
`POST /internal/v1/access-grant/create`. Return the short-lived opaque grant to
the current page and assign it to `element.accessGrant`. Never put the grant in
HTML, a query string, local storage, logs, analytics, or WebSocket protocols.
The component uses it only to call REST and mint a single-use realtime ticket.

## Vanilla host

Build the package, serve [vanilla.html](../web/examples/vanilla.html) from the
repository, and adapt its resource tuple. Registration is explicit. The
`realtime` boolean attribute is opt-in and can also be toggled with the
`realtime` property without reloading the thread or clearing a draft.

Resource changes abort stale REST/media work, stop the old socket, clear the old
projection, and resolve a new thread. Remove the element when its page is
unmounted. Framework wrappers should set `accessGrant` as a property and call
`refresh()` only for an intentional full reload.

## Realtime and network policy

Allow exact host origins in `comment_space.allowed_origins`. WebSocket connects
to the gateway-derived `ws:`/`wss:` path and carries only `comment.v1` plus
`ticket.<opaque>` subprotocols. CSP normally needs `connect-src 'self'` for a
same-origin gateway; add the exact API/WebSocket origin when it differs. Ready
image previews use backend-authorized HTTP(S) signed URLs, so include the exact
FileStorage delivery origin in `img-src`. The component sets `no-referrer` on
signed images.

## Theme contract

| Custom property | Default | Purpose |
| --- | --- | --- |
| `--ms-comment-color` | `#18212f` | Foreground text |
| `--ms-comment-font` | system font | Complete font shorthand |
| `--ms-comment-background` | `#fff` | Container background |
| `--ms-comment-border` | `#d9e0e8` | Borders and tree guides |
| `--ms-comment-radius` | `.75rem` | Container radius |
| `--ms-comment-space` | `1rem` | Container padding |
| `--ms-comment-muted` | `#5d6878` | Metadata and status text |
| `--ms-comment-action` | `#2457d6` | Controls and focus rings |
| `--ms-comment-image-max-height` | `24rem` | Ready image preview limit |

Stable parts are `container`, `heading`, `status`, `realtime-status`, `list`,
`comment`, `comment-content`, `comment-meta`, `comment-body`, `comment-links`,
`comment-attachments`, `attachment`, `attachment-image`, `comment-actions`,
`comment-select`, `reply`, `replies`, `replies-toggle`, `replies-list`,
`replies-load-more`, `load-more`, `composer`, `reply-context`, `cancel-reply`,
`composer-label`, `composer-input`, `composer-files`, `composer-hint`,
`composer-status`, and `submit`.

## Events and accessibility

All `ms-comment-*` events bubble and cross the shadow boundary. Treat
`ms-comment-error` as operational UI evidence; use `ms-comment-created`,
`ms-comment-reconciled`, `ms-comment-thread-updated`,
`ms-comment-realtime-state`, and `ms-comment-typing` for host integration, not
as a replacement for REST authority.

The component uses native form/button/details controls, a labelled textarea,
described file input, visible focus rings, polite atomic live regions, alerts
for load failures, deletion/moderation tombstones, and text-only projection.
Hosts remain responsible for page language, heading context, zoom/reflow,
surrounding landmark structure, and contrast when overriding theme colors.
