#!/bin/bash
#
# Build a macOS installer package (.pkg) for MediaMTX NVR.
#
# Usage: ./build-pkg.sh [version]
#   version  - Semantic version (default: 0.0.0)
#
# Prerequisites:
#   - macOS with Xcode command line tools (pkgbuild, productbuild)
#   - Pre-built mediamtx binary at ../../tmp/mediamtx
#
set -euo pipefail

VERSION="${1:-0.0.0}"
PKG_ID="com.mediamtx.nvr"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORK_DIR="$(mktemp -d)"
OUTPUT_DIR="$PROJECT_ROOT/binaries"

cleanup() {
    rm -rf "$WORK_DIR"
}
trap cleanup EXIT

echo "==> Building MediaMTX NVR macOS package v${VERSION}"

# ---- Create payload structure ----
PAYLOAD="$WORK_DIR/payload"
mkdir -p "$PAYLOAD/usr/local/bin"
mkdir -p "$PAYLOAD/usr/local/etc/mediamtx"
mkdir -p "$PAYLOAD/usr/local/var/lib/mediamtx/recordings"
mkdir -p "$PAYLOAD/usr/local/var/lib/mediamtx/thumbnails"
mkdir -p "$PAYLOAD/usr/local/var/log/mediamtx"
mkdir -p "$PAYLOAD/Library/LaunchDaemons"

# Copy binary
cp "$PROJECT_ROOT/tmp/mediamtx" "$PAYLOAD/usr/local/bin/mediamtx"
chmod 0755 "$PAYLOAD/usr/local/bin/mediamtx"

# Copy config (only installed if not already present, handled by postinstall)
cp "$PROJECT_ROOT/mediamtx.yml" "$PAYLOAD/usr/local/etc/mediamtx/mediamtx.yml.default"

# Copy launchd plist
cp "$SCRIPT_DIR/com.mediamtx.nvr.plist" "$PAYLOAD/Library/LaunchDaemons/com.mediamtx.nvr.plist"

# ---- Create install scripts ----
SCRIPTS="$WORK_DIR/scripts"
mkdir -p "$SCRIPTS"

cat > "$SCRIPTS/preinstall" << 'PREINSTALL'
#!/bin/bash
# Stop existing service if running
launchctl bootout system/com.mediamtx.nvr 2>/dev/null || true
exit 0
PREINSTALL

cat > "$SCRIPTS/postinstall" << 'POSTINSTALL'
#!/bin/bash
set -e

CONFIG_DIR="/usr/local/etc/mediamtx"
DATA_DIR="/usr/local/var/lib/mediamtx"
LOG_DIR="/usr/local/var/log/mediamtx"

# Install default config if none exists
if [ ! -f "$CONFIG_DIR/mediamtx.yml" ]; then
    cp "$CONFIG_DIR/mediamtx.yml.default" "$CONFIG_DIR/mediamtx.yml"
fi

# Create mediamtx user if it doesn't exist
if ! dscl . -read /Users/mediamtx >/dev/null 2>&1; then
    # Find an unused UID above 400
    LAST_UID=$(dscl . -list /Users UniqueID | awk '{print $2}' | sort -n | tail -1)
    NEW_UID=$((LAST_UID + 1))
    if [ "$NEW_UID" -lt 400 ]; then
        NEW_UID=400
    fi

    dscl . -create /Users/mediamtx
    dscl . -create /Users/mediamtx UserShell /usr/bin/false
    dscl . -create /Users/mediamtx UniqueID "$NEW_UID"
    dscl . -create /Users/mediamtx PrimaryGroupID 20
    dscl . -create /Users/mediamtx RealName "MediaMTX NVR"
    dscl . -create /Users/mediamtx NFSHomeDirectory "$DATA_DIR"
    dscl . -create /Users/mediamtx IsHidden 1
fi

# Set ownership
chown -R mediamtx:staff "$DATA_DIR"
chown -R mediamtx:staff "$LOG_DIR"

# Load and start the service
launchctl bootstrap system /Library/LaunchDaemons/com.mediamtx.nvr.plist || true

exit 0
POSTINSTALL

chmod 0755 "$SCRIPTS/preinstall" "$SCRIPTS/postinstall"

# ---- Build component package ----
mkdir -p "$OUTPUT_DIR"
COMPONENT_PKG="$WORK_DIR/mediamtx-component.pkg"

pkgbuild \
    --root "$PAYLOAD" \
    --identifier "$PKG_ID" \
    --version "$VERSION" \
    --scripts "$SCRIPTS" \
    --install-location "/" \
    "$COMPONENT_PKG"

# ---- Build distribution (product) package ----
DIST_XML="$WORK_DIR/distribution.xml"
cat > "$DIST_XML" << DISTXML
<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="2">
    <title>MediaMTX NVR ${VERSION}</title>
    <organization>${PKG_ID}</organization>
    <domains enable_localSystem="true"/>
    <options customize="never" require-scripts="true" rootVolumeOnly="true"/>
    <choices-outline>
        <line choice="default">
            <line choice="${PKG_ID}"/>
        </line>
    </choices-outline>
    <choice id="default"/>
    <choice id="${PKG_ID}" visible="false">
        <pkg-ref id="${PKG_ID}"/>
    </choice>
    <pkg-ref id="${PKG_ID}" version="${VERSION}" onConclusion="none">mediamtx-component.pkg</pkg-ref>
</installer-gui-script>
DISTXML

productbuild \
    --distribution "$DIST_XML" \
    --package-path "$WORK_DIR" \
    "$OUTPUT_DIR/mediamtx-nvr-${VERSION}-macos.pkg"

echo "==> Package built: $OUTPUT_DIR/mediamtx-nvr-${VERSION}-macos.pkg"
