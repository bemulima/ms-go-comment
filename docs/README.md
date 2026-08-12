# Documentation index

Repository-owned code, migrations, tests, and the documents below are the source of truth for `ms-go-comment`. Start with the domain and release maps, then read the contracts affected by a change.

## Architecture and ownership

- [Domain map](domain-map.md): bounded context, domain objects, processes, and change routing.
- [Agent contract map](agent-contract-map.md): current backend capabilities, ownership, access matrix, and implementation status.
- [Business rules](business-rules.md): comment tree, content, policy, mutation, moderation, and idempotency invariants.

## Interfaces and persistence

- [HTTP contract](http-contract.md) and [error contract](error-contract.md): REST routes, authentication boundaries, validation, pagination, and stable errors.
- [WebSocket contract](websocket-contract.md) and [realtime delivery](realtime-delivery.md): tickets, protocol, sequence recovery, outbox, NATS, and retry semantics.
- [Database contract](database-contract.md): owned tables, constraints, repositories, and reversible migrations.
- [Attachment lifecycle](attachment-lifecycle.md): FileStorage ownership, staging, activation, signed access, and cleanup.
- [Access integration](access-integration.md): trusted host flows and private resource grants.

## Browser integration

- [Frontend contract map](frontend-contract-map.md): headless SDK and Web Component runtime contract.
- [Frontend integration](frontend-integration.md): gateway routing, host lifecycle, Origin/CSP, theming, events, and accessibility responsibilities.

Machine-readable mirrors live under `.ai/contracts`. Update an owned document, its machine-readable mirror, implementation, and tests together when the observable contract changes.
