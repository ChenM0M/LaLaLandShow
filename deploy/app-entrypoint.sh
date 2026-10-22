#!/usr/bin/env sh
set -eu

PGDATA="${PGDATA:-/data/postgres}"
REDIS_DIR="${REDIS_DIR:-/data/redis}"
POSTGRES_DB="${POSTGRES_DB:-lalaland}"
POSTGRES_USER="${POSTGRES_USER:-lalaland}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-lalaland}"
export DATABASE_URL="${DATABASE_URL:-postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:5432/${POSTGRES_DB}?sslmode=disable}"
export REDIS_URL="${REDIS_URL:-redis://127.0.0.1:6379/0}"
export JWT_SECRET="${JWT_SECRET:-standalone-jwt-secret-not-for-production}"
export HMAC_SECRET="${HMAC_SECRET:-standalone-hmac-secret-not-for-production}"
export ADMIN_USERS="${ADMIN_USERS:-admin}"
export ADMIN_PASSWORDS="${ADMIN_PASSWORDS:-admin}"
export CHECKER_USER="${CHECKER_USER:-checker}"
export CHECKER_PASSWORD="${CHECKER_PASSWORD:-checker}"

terminate() {
  if [ -n "${REDIS_PID:-}" ]; then
    kill "$REDIS_PID" 2>/dev/null || true
  fi
  if [ -n "${POSTGRES_PID:-}" ]; then
    kill "$POSTGRES_PID" 2>/dev/null || true
  fi
  exit 0
}

trap terminate INT TERM

mkdir -p "$PGDATA" "$REDIS_DIR" /run/postgresql
chown -R postgres:postgres "$PGDATA" /run/postgresql

if [ ! -s "$PGDATA/PG_VERSION" ]; then
  su postgres -c "initdb -D '$PGDATA' --auth=trust --no-locale --encoding=UTF8"
  cat >> "$PGDATA/postgresql.conf" <<'EOF'
listen_addresses = '127.0.0.1'
port = 5432
EOF
  cat >> "$PGDATA/pg_hba.conf" <<'EOF'
host all all 127.0.0.1/32 trust
EOF
fi

su postgres -c "postgres -D '$PGDATA'" &
POSTGRES_PID="$!"

until pg_isready -h 127.0.0.1 -p 5432 >/dev/null 2>&1; do
  sleep 1
done

if ! psql -h 127.0.0.1 -U postgres -tAc "SELECT 1 FROM pg_roles WHERE rolname='${POSTGRES_USER}'" | grep -q 1; then
  psql -h 127.0.0.1 -U postgres -c "CREATE ROLE ${POSTGRES_USER} LOGIN PASSWORD '${POSTGRES_PASSWORD}'"
fi

if ! psql -h 127.0.0.1 -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname='${POSTGRES_DB}'" | grep -q 1; then
  createdb -h 127.0.0.1 -U postgres -O "$POSTGRES_USER" "$POSTGRES_DB"
fi

redis-server --appendonly yes --dir "$REDIS_DIR" --bind 127.0.0.1 --port 6379 &
REDIS_PID="$!"

until [ "$(redis-cli -h 127.0.0.1 -p 6379 ping 2>/dev/null)" = PONG ]; do sleep 0.5; done

exec /app/lalalandshow
