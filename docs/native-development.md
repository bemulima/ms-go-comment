# Native development

Comment runs as a local macOS process while Docker supplies its shared
infrastructure. The existing environment based application configuration stays
topology neutral. `scripts/native_config.py` provides only the native endpoints
and reads the approved infrastructure dotenv file without printing secret
values.

## Shared infrastructure

From `learning-platform-infrastructure`, start and verify the D6 services:

```sh
make native-infra-up
make native-infra-check
```

The D8 infrastructure owner provisions the isolated `lw_comment` database and
`lw_comment` non-superuser role before the native migration is run. Comment does
not reuse the PostgreSQL provisioning identity or another service database.

## Native Comment process

From this repository:

```sh
task migrate:native
task run:native
```

The default adapter values are:

| Setting | Native value |
| --- | --- |
| PostgreSQL | `127.0.0.1:5432`, database and role `lw_comment` |
| NATS | `nats://127.0.0.1:4222` |
| FileStorage | `http://127.0.0.1:18085` |
| Comment listener | `127.0.0.1:18087` |

Set `LW_INFRA_ENV_FILE` when the approved infrastructure dotenv file is not at
its standard relative path. The adapter accepts `COMMENT_NATIVE_*` endpoint
overrides, including `COMMENT_NATIVE_POSTGRES_HOST`,
`COMMENT_NATIVE_POSTGRES_PORT`, `COMMENT_NATIVE_DATABASE`,
`COMMENT_NATIVE_FILESTORAGE_URL`, and `COMMENT_NATIVE_HTTP_PORT`. Passwords and
tokens remain in the approved local secret source.

`HTTP_HOST` is an optional composition setting. It defaults to the existing
empty value, which preserves Docker's `:<port>` listener. The native adapter
sets `HTTP_HOST=127.0.0.1` so only the assigned loopback port is exposed.

`task migrate:native` starts a short lived PostgreSQL client beside the existing
shared PostgreSQL container. It invokes the same `scripts/migrate.sh` runner and
the same ordered SQL files as Docker Compose; only the connection endpoint
differs. The runner only applies a migration when its corresponding schema
state is absent. It does not drop, truncate, reset, or recreate a database.

Comment requires PostgreSQL and NATS at startup. In `all` mode it creates or
uses the `COMMENT_EVENTS` JetStream stream, publishes and subscribes to the
versioned `comment.*` lifecycle subjects, and uses Core NATS for
`comment.realtime.typing.<thread_uuid>`. FileStorage is a runtime HTTP
dependency for attachment operations; its native endpoint is supplied by the
adapter and is not contacted by the health endpoint.

Use `GET http://127.0.0.1:18087/healthz` for listener availability. Stop the
native process with `Ctrl-C`; stop shared infrastructure with
`make native-infra-down` in the infrastructure repository, which retains its
persistent state.

## Standalone Docker path

The service-local Docker path remains self-contained for PostgreSQL and NATS:

```sh
task up
curl -fsS http://127.0.0.1:8092/healthz
task down
```

`task up` starts PostgreSQL and NATS, applies migrations, and then starts
Comment so workers never begin against an empty schema.
`docker-compose.yml` keeps its own PostgreSQL `pgdata` volume and now provides
an internal JetStream-enabled NATS service with a service-specific `natsdata`
volume. Its host mappings are loopback-only. `task down` stops containers
without deleting either volume. The
standalone Compose service uses its configured FileStorage URL for attachment
operations; point that setting at a reachable FileStorage deployment when
testing attachments.
