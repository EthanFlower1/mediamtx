#!/bin/sh
# directory-entrypoint.sh — boots the Directory, waits for health, then
# generates one pairing token per expected Recorder and writes each to the
# shared /pairing volume so Recorder containers can pick them up.
set -e

# Start the Directory in the background.
/mediamtx /config/mediamtx.yml &
DIR_PID=$!

# Wait for the Directory HTTP server to become healthy.
echo "[directory-entrypoint] waiting for Directory to become healthy..."
attempts=0
while [ "$attempts" -lt 30 ]; do
    if wget -q --spider http://localhost:${DIR_PORT:-9997}/healthz 2>/dev/null; then
        echo "[directory-entrypoint] Directory is healthy"
        break
    fi
    attempts=$((attempts + 1))
    sleep 1
done

if [ "$attempts" -ge 30 ]; then
    echo "[directory-entrypoint] ERROR: Directory did not become healthy after 30s"
    kill "$DIR_PID" 2>/dev/null || true
    exit 1
fi

# Number of Recorder tokens to generate (default: 2).
NUM_RECORDERS="${NUM_RECORDERS:-2}"

echo "[directory-entrypoint] generating $NUM_RECORDERS pairing token(s)..."
i=1
while [ "$i" -le "$NUM_RECORDERS" ]; do
    # The GenerateHandler requires X-User-ID (see boot.go line ~300).
    # POST with an empty JSON body — default roles ["recorder"] apply.
    RESPONSE=$(wget -q -O - \
        --header="Content-Type: application/json" \
        --header="X-User-ID: system:docker-compose" \
        --post-data='{}' \
        http://localhost:${DIR_PORT:-9997}/api/v1/pairing/tokens 2>/dev/null) || true

    # Extract the "token" field from the JSON response.
    TOKEN=$(echo "$RESPONSE" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')

    if [ -z "$TOKEN" ]; then
        echo "[directory-entrypoint] WARNING: failed to generate token $i (response: $RESPONSE)"
    else
        echo "$TOKEN" > "/pairing/token-$i"
        echo "[directory-entrypoint] wrote /pairing/token-$i"
    fi

    i=$((i + 1))
done

echo "[directory-entrypoint] pairing bootstrap complete, waiting on Directory process"
wait "$DIR_PID"
