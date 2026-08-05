# Access integration

## Authenticated mode

For a space with `access_mode=authenticated`, every gateway-authenticated non-guest user may read and participate in an open thread, subject to thread state and content policy.

## Private-resource mode

For private host resources, knowing a resource ID must not grant access. The host service first evaluates its own page/resource permissions, then requests a short-lived comment access grant through the internal API. The grant binds:

```text
issuer + user_id + space/resource tuple + read/write/upload permissions + expiry
```

The browser presents the grant to authenticated Comment REST calls. A realtime ticket may be minted only within the grant's permissions and lifetime.

The first implementation may use the platform shared internal token because that is the existing service-to-service pattern. The contract must preserve an upgrade path to per-service credentials or asymmetric grants.

## Gateway boundary

- Student gateway maps `/api/comment/v1/*` to `/api/v1/*` and replaces client-supplied identity headers with verified values.
- WebSocket proxying preserves upgrade headers and forwards the requested subprotocol and Origin. Upstream ticket validation remains authoritative.
- Admin gateway maps its Comment namespace to `/admin/v1/*`.
- The Comment container is reachable only on trusted service networks.
