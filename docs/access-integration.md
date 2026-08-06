# Access integration

## Authenticated mode

For a space with `access_mode=authenticated`, every gateway-authenticated non-guest user may read and participate in an open thread, subject to thread state and content policy.

## Private-resource mode

For private host resources, knowing a resource ID must not grant access. The host service first evaluates its own page/resource permissions, then requests a short-lived comment access grant through the internal API. The grant binds:

```text
issuer + user_id + space/resource tuple + read/write/upload permissions + expiry
```

The browser presents the grant as `X-Comment-Access-Grant` together with its
gateway-authenticated identity. Comment stores only the SHA-256 grant hash. A
grant is reusable until expiry but cannot move to another user, space, resource
type, or resource ID. A realtime ticket may be minted only within the grant's
permissions; the WebSocket handshake then consumes the single-use ticket and
does not accept the grant again.

The implemented internal API uses the platform shared token because that is the
existing service-to-service pattern. Every `/internal/v1/*` request must carry
the exact configured `X-Internal-Token`. The contract preserves an upgrade path
to per-service credentials or asymmetric grants through the explicit `issuer`
binding.

## Host-service flow

1. The host authorizes its own resource for the authenticated user.
2. It calls `POST /internal/v1/thread/ensure` for the exact opaque resource tuple.
3. It calls `POST /internal/v1/access-grant/create` with issuer, user, tuple,
   permissions, and bounded TTL.
4. The browser supplies `X-Comment-Access-Grant` on Comment REST calls and on
   `POST /api/v1/realtime/ticket`.
5. The browser connects to WebSocket with the returned `ticket.<opaque>`
   subprotocol; the grant never enters a URL or WebSocket frame.

Permission mapping is exact: thread/comment reads and signed attachment URLs
require `read`; create/update/delete require `write`; image upload requires
`upload`. Every grant includes `read`, and `upload` is valid only together with
`write`. Ticket permissions are the intersection of the grant, current thread
state, and effective image policy.

Grant TTL is at least one second and at most
`ACCESS_GRANT_MAX_TTL_SECONDS` (default 300, configuration hard-capped at 900).
Expired rows are removed by a bounded worker every
`ACCESS_GRANT_CLEANUP_SECONDS` (default 60).

## Gateway boundary

- Student gateway maps `/api/comment/v1/*` to `/api/v1/*` and replaces client-supplied identity headers with verified values.
- WebSocket proxying preserves upgrade headers and forwards the requested subprotocol and Origin. Upstream ticket validation remains authoritative.
- Admin gateway maps its Comment namespace to `/admin/v1/*`.
- The Comment container is reachable only on trusted service networks.
