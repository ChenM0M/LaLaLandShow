#!/usr/bin/env sh
set -eu

backup_dir="${BACKUP_DIR:-/backups}"
retention_days="${BACKUP_RETENTION_DAYS:-14}"
timestamp="$(date +%Y%m%d-%H%M%S)"
file="${backup_dir}/lalaland-${timestamp}.sql.gz"

mkdir -p "${backup_dir}"
pg_dump "${DATABASE_URL}" | gzip > "${file}"
find "${backup_dir}" -name 'lalaland-*.sql.gz' -mtime "+${retention_days}" -delete
echo "${file}"
