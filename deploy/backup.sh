#!/usr/bin/env sh
set -eu

: "${DATABASE_URL:?set DATABASE_URL}"

backup_dir="${BACKUP_DIR:-/backups}"
retention_days="${BACKUP_RETENTION_DAYS:-14}"
timestamp="$(date +%Y%m%d-%H%M%S)"
file="${backup_dir}/lalaland-${timestamp}.sql.gz"
plain_tmp="${backup_dir}/.${timestamp}.sql.tmp"
gzip_tmp="${file}.tmp"

cleanup() {
  rm -f "${plain_tmp}" "${gzip_tmp}"
}
trap cleanup EXIT

mkdir -p "${backup_dir}"
pg_dump "${DATABASE_URL}" > "${plain_tmp}"
gzip -c "${plain_tmp}" > "${gzip_tmp}"
mv "${gzip_tmp}" "${file}"
find "${backup_dir}" -name 'lalaland-*.sql.gz' -mtime "+${retention_days}" -delete
echo "${file}"
