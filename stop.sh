#!/usr/bin/env bash
set -e

echo "======================================"
echo "🛑 Stopping La La Land Show Servers 🛑"
echo "======================================"

cd "$(dirname "$0")/deploy"
docker-compose down

echo ""
echo "✅ All containers stopped successfully."
