#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

: "${DEPLOY_HOST:?set DEPLOY_HOST}"
: "${DEPLOY_USER:?set DEPLOY_USER}"
: "${DEPLOY_PATH:?set DEPLOY_PATH}"

DEPLOY_PORT="${DEPLOY_PORT:-22}"
IMAGE_TAG="${IMAGE_TAG:-local-$(git -C "$PROJECT_ROOT" rev-parse --short HEAD)}"
BACKEND_IMAGE="${BACKEND_IMAGE:-lalalandshow-backend}"
FRONTEND_IMAGE="${FRONTEND_IMAGE:-lalalandshow-frontend}"
SSH_TARGET="${DEPLOY_USER}@${DEPLOY_HOST}"
SSH_OPTS="${SSH_OPTS:-}"

docker build -t "${BACKEND_IMAGE}:${IMAGE_TAG}" -f "$PROJECT_ROOT/backend/Dockerfile" "$PROJECT_ROOT"
docker build -t "${FRONTEND_IMAGE}:${IMAGE_TAG}" -f "$PROJECT_ROOT/frontend/Dockerfile" "$PROJECT_ROOT"

ssh $SSH_OPTS -p "$DEPLOY_PORT" "$SSH_TARGET" "mkdir -p '$DEPLOY_PATH'"
scp $SSH_OPTS -P "$DEPLOY_PORT" \
  "$SCRIPT_DIR/docker-compose.prod.yml" \
  "$SCRIPT_DIR/nginx.conf" \
  "$SCRIPT_DIR/.env.prod.example" \
  "$SCRIPT_DIR/remote-deploy.sh" \
  "$SSH_TARGET:$DEPLOY_PATH/"

printf 'BACKEND_IMAGE=%s\nFRONTEND_IMAGE=%s\nIMAGE_TAG=%s\n' "$BACKEND_IMAGE" "$FRONTEND_IMAGE" "$IMAGE_TAG" > /tmp/lalalandshow-release.env
scp $SSH_OPTS -P "$DEPLOY_PORT" /tmp/lalalandshow-release.env "$SSH_TARGET:$DEPLOY_PATH/.release.env"

docker save "${BACKEND_IMAGE}:${IMAGE_TAG}" "${FRONTEND_IMAGE}:${IMAGE_TAG}" | gzip | ssh $SSH_OPTS -p "$DEPLOY_PORT" "$SSH_TARGET" "gunzip | docker load"
ssh $SSH_OPTS -p "$DEPLOY_PORT" "$SSH_TARGET" "cd '$DEPLOY_PATH' && chmod +x remote-deploy.sh && SKIP_PULL=true ./remote-deploy.sh"
