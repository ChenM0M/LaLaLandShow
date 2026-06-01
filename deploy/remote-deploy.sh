#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env}"
RELEASE_ENV_FILE="${RELEASE_ENV_FILE:-.release.env}"

if [ ! -f "$ENV_FILE" ]; then
  echo "Missing $ENV_FILE. Copy .env.prod.example to .env and fill production secrets first." >&2
  exit 1
fi

if [ ! -f "$RELEASE_ENV_FILE" ]; then
  echo "Missing $RELEASE_ENV_FILE. It should contain APP_IMAGE and IMAGE_TAG." >&2
  exit 1
fi

if [ "${SKIP_PULL:-false}" != "true" ]; then
  docker compose --env-file "$ENV_FILE" --env-file "$RELEASE_ENV_FILE" -f "$COMPOSE_FILE" pull
fi
docker compose --env-file "$ENV_FILE" --env-file "$RELEASE_ENV_FILE" -f "$COMPOSE_FILE" up -d --remove-orphans
docker image prune -f
docker compose --env-file "$ENV_FILE" --env-file "$RELEASE_ENV_FILE" -f "$COMPOSE_FILE" ps
