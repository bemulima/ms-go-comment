#!/bin/sh
# Applies the repository-owned SQL migrations through standard libpq variables.
set -eu

migrations_dir="${MIGRATIONS_DIR:-db/migrations}"

has_schema="$(psql -v ON_ERROR_STOP=1 -tAc "SELECT to_regclass('public.comment_space') IS NOT NULL AND to_regclass('public.comment_thread') IS NOT NULL AND to_regclass('public.comment') IS NOT NULL AND to_regclass('public.comment_attachment') IS NOT NULL AND to_regclass('public.comment_outbox') IS NOT NULL AND to_regclass('public.comment_ws_ticket') IS NOT NULL;")"
if [ "$has_schema" = "t" ]; then
  echo "Initial comment schema already exists"
else
  psql -v ON_ERROR_STOP=1 -f "$migrations_dir/001_init.up.sql"
fi

has_attachment_delivery="$(psql -v ON_ERROR_STOP=1 -tAc "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='comment_attachment' AND column_name='activation_attempts');")"
if [ "$has_attachment_delivery" = "t" ]; then
  echo "Attachment delivery migration already applied"
else
  psql -v ON_ERROR_STOP=1 -f "$migrations_dir/002_attachment_delivery.up.sql"
fi

has_access_grants="$(psql -v ON_ERROR_STOP=1 -tAc "SELECT to_regclass('public.comment_access_grant') IS NOT NULL;")"
if [ "$has_access_grants" = "t" ]; then
  echo "Access grants migration already applied"
else
  psql -v ON_ERROR_STOP=1 -f "$migrations_dir/003_access_grants.up.sql"
fi
