#!/usr/bin/env bash
# Usage: ./scripts/multi-server-up.sh [--build] [--detach]
#
# Brings up the multi-server KaiVue environment with real mode separation:
#   1 Directory (control plane) + 2 Recorders (paired) + 4 fake RTSP cameras

set -euo pipefail
cd "$(dirname "$0")/.."

COMPOSE_FILE="docker-compose.multi-server.yml"
ARGS=()

for arg in "$@"; do
  case "$arg" in
    --build)  ARGS+=("--build") ;;
    --detach) ARGS+=("--detach") ;;
    *)        echo "Unknown arg: $arg"; exit 1 ;;
  esac
done

echo ""
echo "Multi-Server KaiVue Environment (Real Mode Separation)"
echo "======================================================="
echo ""
echo "Directory (control plane):"
echo "  Admin UI:  http://localhost:9997"
echo "  API:       http://localhost:9997/api/v1/"
echo "  Healthz:   http://localhost:9997/healthz"
echo "  RTSP:      rtsp://localhost:8554"
echo ""
echo "Recorder 1 (paired with Directory):"
echo "  API:       http://localhost:9998"
echo "  RTSP:      rtsp://localhost:8564"
echo "  Cameras:   cam1 (test pattern), cam2 (grid)"
echo ""
echo "Recorder 2 (paired with Directory):"
echo "  API:       http://localhost:9999"
echo "  RTSP:      rtsp://localhost:8574"
echo "  Cameras:   cam3 (color bars), cam4 (fractal)"
echo ""
echo "Boot sequence:"
echo "  1. Directory starts, generates pairing tokens"
echo "  2. Recorders pick up tokens, run 9-step join sequence"
echo "  3. Fake cameras connect to their assigned Recorders"
echo ""
echo "The Directory coordinates both Recorders:"
echo "  - Unified camera list at http://localhost:9997"
echo "  - Cross-Recorder playback via stream URL routing"
echo "  - Camera assignments streamed to each Recorder"
echo "  - Recording segments + AI events ingested from Recorders"
echo ""

docker compose -f "$COMPOSE_FILE" up "${ARGS[@]}"
