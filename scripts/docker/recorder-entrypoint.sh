#!/bin/sh
# recorder-entrypoint.sh — waits for its pairing token to appear on the
# shared /pairing volume, then boots the Recorder with MTX_PAIRING_TOKEN set.
set -e

RECORDER_ID="${RECORDER_ID:-1}"

echo "[recorder-entrypoint] Recorder $RECORDER_ID waiting for pairing token..."
attempts=0
while [ "$attempts" -lt 60 ]; do
    if [ -f "/pairing/token-$RECORDER_ID" ]; then
        TOKEN=$(cat "/pairing/token-$RECORDER_ID")
        if [ -n "$TOKEN" ]; then
            echo "[recorder-entrypoint] got pairing token for Recorder $RECORDER_ID"
            break
        fi
    fi
    attempts=$((attempts + 1))
    sleep 1
done

if [ -z "$TOKEN" ]; then
    echo "[recorder-entrypoint] ERROR: no pairing token found after 60s"
    exit 1
fi

export MTX_PAIRING_TOKEN="$TOKEN"

echo "[recorder-entrypoint] starting Recorder $RECORDER_ID..."
exec /mediamtx /config/mediamtx.yml
