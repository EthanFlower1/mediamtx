#!/usr/bin/env bash
# Usage: ./scripts/multi-server-down.sh
#
# Tears down the multi-server KaiVue test environment and removes volumes.

set -euo pipefail
cd "$(dirname "$0")/.."

echo "Stopping multi-server KaiVue environment..."
docker compose -f docker-compose.multi-server.yml down -v
echo "Done."
