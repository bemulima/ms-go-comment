# Repository Guidelines

<!-- codex-agent-bootstrap:start -->
## Agent Bootstrap
- Before planning or changing files, read all available project instructions:
  - `docs/*`
  - `prompts/*`
- Treat `prompts/git-workflow.md` as the required workflow for issue, branch, commit, push, Pull Request, merge, and issue-closing behavior.
- If a listed directory does not exist, continue with the instructions that are present.
<!-- codex-agent-bootstrap:end -->

<!-- codex-shared-policy:start -->
## Codex Shared Policy (Managed)

Source of truth: `prompts/codex-shared-agents.md`

This file is the canonical shared policy for repo-level `AGENTS.md` files in
git-backed repositories under `/Users/marat/Developments/microservices`.

## Cache policy
- Use only repo-local `.cache` for temporary build and tool artifacts.
- Do not create or rely on repo-local `.gocache`.
- For Go commands, prefer these locations:
  - `XDG_CACHE_HOME=$PWD/.cache`
  - `GOCACHE=$PWD/.cache/go-build`
  - `GOMODCACHE=$PWD/.cache/gomod`
  - `GOBIN=$PWD/.cache/bin`
- Put disposable local binaries in `.cache/bin`.
- Treat `.cache` as disposable local state. Do not commit it.

## Workspace hygiene
- Do not introduce extra cache directories when `.cache` can be used instead.
- Keep temporary logs, generated reports, and ad-hoc tooling output under
  `.cache` when practical.
- Do not store persistent project data in `.cache`.
- If a repo has stricter local requirements, document them in that repo's
  `AGENTS.md` below the managed shared block.

## Scope
- This policy is synced into repo-level `AGENTS.md` files by
  `prompts/scripts/sync_agents.py`.
- The canonical source of truth is this file, not the generated copies.
<!-- codex-shared-policy:end -->

## Project Structure & Module Organization
- `cmd/ms-comment-service`: composition root for API, realtime hub, and background workers.
- `internal/usecase`: business logic for spaces, threads, comments, attachments, access grants, and realtime tickets.
- `internal/adapters/http`: chi router, request validation, auth/token checks, and API/admin/internal routes.
- `internal/adapters/websocket`: authenticated WebSocket transport and local connection hub; it must call shared use cases rather than duplicate business rules.
- `internal/adapters/postgres`: pgx repositories; keep SQL and migrations aligned.
- `internal/adapters/nats`: outbox-backed lifecycle publication and ephemeral realtime fan-out.
- `internal/adapters/filestorage`: temporary upload, activation, deletion, and signed-URL integration.
- `internal/domain`: entities, repository contracts, shared errors/events.
- `db/migrations`: reversible versioned schema SQL applied to the service-owned PostgreSQL database.
- `test`: integration-style HTTP/router tests; add more alongside features.

## Build, Test, and Development Commands
- `task up` or `docker-compose up --build`: start service + Postgres + NATS locally.
- `make up`: same as above but detached.
- `task migrate` or `make migrate`: apply SQL migrations inside the Postgres container (sorted order).
- `task test` or `go test ./...`: run Go unit/integration tests with local build cache `.cache/go-build`.
- `task down` or `docker-compose down -v`: stop stack and clean volumes.

## Coding Style & Naming Conventions
- Follow standard Go style; run `gofmt`/`goimports` before committing.
- Prefer clear package names mirroring folders (`usecase`, `adapter/http`, `adapter/postgres`).
- Keep handlers thin; business logic belongs in `usecase`, persistence in `adapter/postgres`.
- Use context-aware functions (`ctx` first param) and `Err*` naming for sentinel errors.
- Filename pattern: feature oriented (`comment_service.go`, `thread_repository.go`); tests end with `_test.go`.

## Testing Guidelines
- Framework: Go testing package with testify where needed; keep table-driven cases for handlers/usecases.
- Place tests near code (`*_test.go`) and add router/integration cases under `test/`.
- Name tests `<Type>_<Behavior>` for clarity (e.g., `TestCreateComment_RejectsCrossThreadParent`).
- Aim to cover auth, access grants, tree depth, idempotency, optimistic locking, and reconnect error paths.

## Commit & Pull Request Guidelines
- Follow existing history: conventional-ish prefixes (`chore:`, `feat:`, `fix:`) plus imperative summaries.
- One logical change per commit; include rationale in body if behavior shifts.
- PRs should describe scope, testing performed (`go test ./...`), and any migration or NATS subject impacts.
- Link issues if applicable; add screenshots or curl examples when altering HTTP endpoints.

## Security & Configuration Tips
- Keep `INTERNAL_API_TOKEN` secret; required for `/internal/*` routes.
- Trust `X-User-ID` and `X-User-Role` only behind the configured gateway; never expose the service container directly to untrusted traffic.
- Never accept raw HTML. Treat comment Markdown and URLs as untrusted input and keep FileStorage downloads behind comment authorization.
- Default envs are in README; avoid committing overrides. Provide sample `.env` snippets in PRs when relevant.
- Migrations are ordered and reversible; avoid out-of-band schema changes.

<!-- agent-orchestrator:start -->
## Agent Orchestrator (Managed)

Read `.ai/service.yaml` before repository work. Follow every linked repository instruction and prompt. Do not edit files outside an explicitly approved write scope.
<!-- agent-orchestrator:end -->
