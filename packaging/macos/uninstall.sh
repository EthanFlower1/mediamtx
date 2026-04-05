#!/bin/bash
#
# Uninstall MediaMTX NVR from macOS.
#
# Usage: sudo ./uninstall.sh [--purge]
#   --purge  Remove config, data, and log directories as well
#
set -euo pipefail

PURGE=false
if [ "${1:-}" = "--purge" ]; then
    PURGE=true
fi

echo "==> Uninstalling MediaMTX NVR..."

# Stop and unload the service
launchctl bootout system/com.mediamtx.nvr 2>/dev/null || true

# Remove binary and launchd plist
rm -f /usr/local/bin/mediamtx
rm -f /Library/LaunchDaemons/com.mediamtx.nvr.plist

# Forget the package receipt
pkgutil --forget com.mediamtx.nvr 2>/dev/null || true

if [ "$PURGE" = true ]; then
    echo "==> Purging config, data, and log directories..."
    rm -rf /usr/local/etc/mediamtx
    rm -rf /usr/local/var/lib/mediamtx
    rm -rf /usr/local/var/log/mediamtx

    # Remove mediamtx user
    if dscl . -read /Users/mediamtx >/dev/null 2>&1; then
        dscl . -delete /Users/mediamtx
    fi
else
    echo "==> Config and data preserved in /usr/local/etc/mediamtx and /usr/local/var/lib/mediamtx"
    echo "    Run with --purge to remove everything."
fi

echo "==> MediaMTX NVR uninstalled."
