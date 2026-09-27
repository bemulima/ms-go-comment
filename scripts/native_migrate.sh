#!/bin/sh
# Runs the common migration runner beside the shared PostgreSQL container.
set -eu

: "${COMMENT_NATIVE_POSTGRES_CONTAINER:=learning-workspace-local-platform-postgres-1}"
: "${PGUSER:?PGUSER is required}"
: "${PGPASSWORD:?PGPASSWORD is required}"
: "${PGDATABASE:?PGDATABASE is required}"

if ! docker inspect "$COMMENT_NATIVE_POSTGRES_CONTAINER" >/dev/null 2>&1; then
  echo "native Comment migration requires running shared PostgreSQL container $COMMENT_NATIVE_POSTGRES_CONTAINER" >&2
  exit 2
fi

exec docker run --rm \
  --network "container:${COMMENT_NATIVE_POSTGRES_CONTAINER}" \
  --env PGUSER --env PGPASSWORD --env PGDATABASE \
  -e PGHOST=127.0.0.1 -e PGPORT=5432 -e MIGRATIONS_DIR=/migrations \
  -v "$PWD/db/migrations:/migrations:ro" \
  -v "$PWD/scripts/migrate.sh:/usr/local/bin/ms-comment-migrate:ro" \
  postgres:16-alpine sh /usr/local/bin/ms-comment-migrate
