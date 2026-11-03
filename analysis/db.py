"""Подключение к PostgreSQL из docker compose и загрузка запроса в DataFrame.

Параметры берутся из переменных окружения PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE (так
настроен CI), а если их нет — из .env в корне репозитория (так работает локально после make up).
"""

from __future__ import annotations

import os
from pathlib import Path

import pandas as pd
import psycopg

REPO_ROOT = Path(__file__).resolve().parent.parent


def read_env_file(path: Path) -> dict[str, str]:
    """Простой разбор .env: строки KEY=VALUE, комментарии и пустые строки пропускаются."""
    values = {}
    if not path.exists():
        return values
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip("'\"")
    return values


def connection_params() -> dict[str, str]:
    env = read_env_file(REPO_ROOT / ".env")
    return {
        "host": os.environ.get("PGHOST", "localhost"),
        "port": os.environ.get("PGPORT", "5432"),
        "user": os.environ.get("PGUSER", env.get("POSTGRES_USER", "vetcare")),
        "password": os.environ.get("PGPASSWORD", env.get("POSTGRES_PASSWORD", "")),
        "dbname": os.environ.get("PGDATABASE", env.get("POSTGRES_DB", "vetcare")),
    }


def query(sql: str) -> pd.DataFrame:
    with psycopg.connect(**connection_params()) as conn, conn.cursor() as cur:
        cur.execute(sql)
        columns = [c.name for c in cur.description]
        return pd.DataFrame(cur.fetchall(), columns=columns)
