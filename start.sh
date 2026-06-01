#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$ROOT_DIR/deploy"
FRONTEND_DIR="$ROOT_DIR/frontend"

START_FRONTEND=true
FOLLOW_LOGS=false
BUILD_ARGS=(--build)

usage() {
    cat <<'USAGE'
Usage: ./start.sh [options]

Options:
  --no-frontend   Start only Docker services, then exit after health checks.
  --logs          Start Docker services and follow their logs instead of Vite.
  --no-build      Start Docker services without rebuilding the backend image.
  -h, --help      Show this help.
USAGE
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --no-frontend)
            START_FRONTEND=false
            ;;
        --logs)
            START_FRONTEND=false
            FOLLOW_LOGS=true
            ;;
        --no-build)
            BUILD_ARGS=()
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
    shift
done

compose() {
    if command -v docker-compose >/dev/null 2>&1; then
        docker-compose "$@"
    elif docker compose version >/dev/null 2>&1; then
        docker compose "$@"
    else
        echo "Docker Compose was not found. Install Docker Desktop, then try again." >&2
        exit 1
    fi
}

env_value() {
    local key="$1"
    local default_value="$2"
    local env_file="$DEPLOY_DIR/.env"
    local line=""

    if [ -f "$env_file" ]; then
        line="$(grep -E "^${key}=" "$env_file" | tail -n 1 || true)"
    fi

    if [ -n "$line" ]; then
        printf '%s\n' "${line#*=}" | sed -e 's/^"//' -e 's/"$//'
    else
        printf '%s\n' "$default_value"
    fi
}

wait_for_backend() {
    local url="$1"
    local max_attempts=45
    local attempt=1

    if ! command -v curl >/dev/null 2>&1; then
        echo "curl is not installed; skipping backend health probe."
        return 0
    fi

    printf "Waiting for backend health check at %s" "$url"
    while [ "$attempt" -le "$max_attempts" ]; do
        if curl -fsS "$url" >/dev/null 2>&1; then
            echo ""
            return 0
        fi
        printf "."
        sleep 1
        attempt=$((attempt + 1))
    done

    echo ""
    echo "Backend did not become healthy in ${max_attempts}s. Recent backend logs:" >&2
    compose logs --tail=80 backend >&2 || true
    return 1
}

echo "======================================"
echo "✨ Starting La La Land Show Servers ✨"
echo "======================================"

cd "$DEPLOY_DIR"

# Check if .env exists, if not, copy example
if [ ! -f .env ] && [ -f .env.example ]; then
    echo "Creating default .env from .env.example..."
    cp .env.example .env
fi

APP_PORT="$(env_value APP_PORT 8080)"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
BACKEND_URL="http://localhost:${APP_PORT}"
FRONTEND_URL="http://localhost:${FRONTEND_PORT}"

# Build and start all containers in detached mode
echo "Bringing up containers (Postgres, Redis, Backend)..."
if [ "${#BUILD_ARGS[@]}" -gt 0 ]; then
    compose up -d "${BUILD_ARGS[@]}"
else
    compose up -d
fi

echo ""
compose ps

echo ""
wait_for_backend "${BACKEND_URL}/healthz"

echo ""
echo "✅ Backend stack is running."
echo "📡 Backend API: ${BACKEND_URL}"
echo "🛑 Stop containers with: ./stop.sh"

if [ "$FOLLOW_LOGS" = true ]; then
    echo ""
    echo "Following Docker logs. Press Ctrl+C to stop watching logs; containers will keep running."
    compose logs -f
    exit 0
fi

if [ "$START_FRONTEND" = false ]; then
    echo "Frontend was not started. Run ./start.sh without --no-frontend to start Vite too."
    exit 0
fi

if [ ! -d "$FRONTEND_DIR" ]; then
    echo "Frontend directory not found: $FRONTEND_DIR"
    exit 0
fi

if ! command -v npm >/dev/null 2>&1; then
    echo "npm is not installed, so the frontend dev server was not started."
    echo "Install Node.js/npm, then run: cd frontend && npm run dev -- --host 0.0.0.0 --port ${FRONTEND_PORT}"
    exit 0
fi

if [ ! -d "$FRONTEND_DIR/node_modules" ]; then
    echo "Frontend dependencies are missing, so Vite was not started."
    echo "Run: cd frontend && npm install"
    exit 0
fi

echo ""
echo "Starting frontend dev server in the foreground..."
echo "🌐 Frontend: ${FRONTEND_URL}"
echo "Press Ctrl+C to stop Vite. Docker services will keep running until ./stop.sh."
echo ""
cd "$FRONTEND_DIR"
exec npm run dev -- --host 0.0.0.0 --port "$FRONTEND_PORT"
