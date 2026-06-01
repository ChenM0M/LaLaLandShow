#!/usr/bin/env sh
set -eu

terminate() {
  if [ -n "${BACKEND_PID:-}" ]; then
    kill "$BACKEND_PID" 2>/dev/null || true
  fi
  exit 0
}

trap terminate INT TERM

/app/lalalandshow &
BACKEND_PID="$!"

nginx -g "daemon off;" &
NGINX_PID="$!"

while kill -0 "$BACKEND_PID" 2>/dev/null && kill -0 "$NGINX_PID" 2>/dev/null; do
  sleep 1
done

terminate
