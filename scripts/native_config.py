#!/usr/bin/env python3
"""Run Comment with native-development endpoints and approved local secrets."""

from __future__ import annotations

import argparse
import os
import re
import sys
from pathlib import Path
from urllib.parse import quote


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_INFRA_ENV_FILE = ROOT.parents[1] / "learning-platform-infrastructure" / ".env"
KEY = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


def load_dotenv(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for number, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].lstrip()
        key, separator, value = line.partition("=")
        if separator != "=" or not KEY.fullmatch(key.strip()):
            raise ValueError(f"unsupported dotenv syntax at {path}:{number}")
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '\"'}:
            value = value[1:-1]
        values[key.strip()] = value
    return values


def required(values: dict[str, str], name: str) -> str:
    value = values.get(name, "")
    if not value:
        raise ValueError(f"{name} is required by the approved infrastructure environment")
    return value


def native_environment() -> dict[str, str]:
    configured_path = os.environ.get("LW_INFRA_ENV_FILE")
    infra_env_file = Path(configured_path).expanduser() if configured_path else DEFAULT_INFRA_ENV_FILE
    values = load_dotenv(infra_env_file)
    values.update({name: value for name, value in os.environ.items() if name.startswith("COMMENT_NATIVE_")})

    database = values.get("COMMENT_NATIVE_DATABASE", "lw_comment")
    postgres_host = values.get("COMMENT_NATIVE_POSTGRES_HOST", "127.0.0.1")
    postgres_port = values.get("COMMENT_NATIVE_POSTGRES_PORT", "5432")
    http_port = values.get("COMMENT_NATIVE_HTTP_PORT", "18087")
    db_user = values.get("LW_COMMENT_DB_USER", "lw_comment")
    db_password = values.get("LW_COMMENT_DB_PASSWORD") or required(values, "LW_POSTGRES_PASSWORD")
    db_dsn = "postgres://{}:{}@{}:{}/{}?sslmode=disable".format(
        quote(db_user, safe=""), quote(db_password, safe=""), postgres_host, postgres_port, database
    )

    environment = dict(os.environ)
    environment.update(
        {
            "HTTP_HOST": values.get("COMMENT_NATIVE_HTTP_HOST", "127.0.0.1"),
            "HTTP_PORT": http_port,
            "DATABASE_URL": db_dsn,
            "NATS_URL": values.get("COMMENT_NATIVE_NATS_URL", "nats://127.0.0.1:4222"),
            "FILESTORAGE_SERVICE_BASE_URL": values.get(
                "COMMENT_NATIVE_FILESTORAGE_URL", "http://127.0.0.1:18085"
            ),
            "INTERNAL_API_TOKEN": values.get("COMMENT_NATIVE_INTERNAL_API_TOKEN")
            or required(values, "LW_INTERNAL_TOKEN"),
            "PGUSER": db_user,
            "PGPASSWORD": db_password,
            "PGDATABASE": database,
            "COMMENT_NATIVE_POSTGRES_CONTAINER": values.get(
                "COMMENT_NATIVE_POSTGRES_CONTAINER", "learning-workspace-local-platform-postgres-1"
            ),
        }
    )
    print(
        "native Comment config: database={} postgres={}:{} nats={} filestorage={} http={}:{}".format(
            database,
            postgres_host,
            postgres_port,
            environment["NATS_URL"],
            environment["FILESTORAGE_SERVICE_BASE_URL"],
            environment["HTTP_HOST"],
            http_port,
        ),
        flush=True,
    )
    return environment


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ["--"] else args.command
    if not command:
        parser.error("a command is required after --")
    try:
        environment = native_environment()
    except (OSError, ValueError) as error:
        print(f"native Comment configuration failed: {error}", file=sys.stderr)
        return 2
    os.execvpe(command[0], command, environment)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
