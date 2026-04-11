#!/usr/bin/env bash
# --------------------------------------------------------------------------
# build-installer.sh — Build signed Windows installer or Linux packages.
#
# Usage:
#   ./scripts/build-installer.sh [--platform windows|linux] [--brand <name>]
#                                [--channel stable|beta] [--skip-sign]
#
# Environment (Windows code signing):
#   CODESIGN_CERT_THUMBPRINT — SHA-1 thumbprint of the EV certificate
#   CODESIGN_TIMESTAMP_URL  — RFC 3161 timestamp server (default: DigiCert)
#   AZURE_TENANT_ID         — Azure Key Vault tenant  (HSM-backed signing)
#   AZURE_CLIENT_ID         — Azure Key Vault client
#   AZURE_CLIENT_SECRET     — Azure Key Vault secret
#   AZURE_VAULT_URI         — Azure Key Vault URI
#   AZURE_CERT_NAME         — Certificate name in vault
#
# Prerequisites (CI runner):
#   - Qt 6.6+ with Qt IFW installed (binarycreator on PATH)
#   - signtool.exe (Windows SDK) or AzureSignTool for cloud HSM signing
#   - rpmbuild, dpkg-deb (Linux)
# --------------------------------------------------------------------------
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ── defaults ──────────────────────────────────────────────────────────────
PLATFORM="${PLATFORM:-windows}"
BRAND="kaivue"
CHANNEL="stable"
SKIP_SIGN=0
BUILD_DIR="${PROJECT_DIR}/build"
VERSION=""

# ── arg parsing ───────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --platform)  PLATFORM="$2";   shift 2 ;;
        --brand)     BRAND="$2";      shift 2 ;;
        --channel)   CHANNEL="$2";    shift 2 ;;
        --skip-sign) SKIP_SIGN=1;     shift   ;;
        *)           echo "Unknown arg: $1"; exit 1 ;;
    esac
done

# ── helpers ───────────────────────────────────────────────────────────────
log() { echo "==> $*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

read_version() {
    VERSION=$(grep 'VERSION' "${PROJECT_DIR}/CMakeLists.txt" \
        | head -1 \
        | sed -E 's/.*VERSION ([0-9]+\.[0-9]+\.[0-9]+).*/\1/')
    [[ -n "$VERSION" ]] || die "Could not read version from CMakeLists.txt"
    log "Version: $VERSION"
}

# ── load branding ─────────────────────────────────────────────────────────
load_branding() {
    local brand_file="${PROJECT_DIR}/branding/${BRAND}.json"
    if [[ ! -f "$brand_file" ]]; then
        die "Branding file not found: $brand_file"
    fi
    BRAND_NAME=$(python3 -c "import json,sys; d=json.load(open('$brand_file')); print(d['name'])")
    BRAND_PUBLISHER=$(python3 -c "import json,sys; d=json.load(open('$brand_file')); print(d['publisher'])")
    BRAND_ID=$(python3 -c "import json,sys; d=json.load(open('$brand_file')); print(d['id'])")
    log "Brand: $BRAND_NAME ($BRAND_ID) by $BRAND_PUBLISHER"
}

# ── token replacement in IFW configs ──────────────────────────────────────
stamp_templates() {
    local staging="$1"
    find "$staging" -type f \( -name '*.xml' -o -name '*.qs' -o -name '*.txt' \) \
        -exec sed -i.bak \
            -e "s|%BRAND_NAME%|${BRAND_NAME}|g" \
            -e "s|%BRAND_PUBLISHER%|${BRAND_PUBLISHER}|g" \
            -e "s|%BRAND_ID%|${BRAND_ID}|g" \
            -e "s|@PROJECT_VERSION@|${VERSION}|g" \
            -e "s|@RELEASE_DATE@|$(date +%Y-%m-%d)|g" \
            -e "s|@CHANNEL@|${CHANNEL}|g" \
            {} +
    find "$staging" -name '*.bak' -delete
}

# ══════════════════════════════════════════════════════════════════════════
# WINDOWS — Qt IFW installer + EV code signing
# ══════════════════════════════════════════════════════════════════════════
build_windows() {
    log "Building Windows installer..."
    local staging="${BUILD_DIR}/installer-staging"
    rm -rf "$staging"
    cp -r "${PROJECT_DIR}/installer/windows" "$staging"

    # Copy built binaries into the data directory.
    local data_dir="${staging}/packages/com.kaivue.videowall/data"
    if [[ -d "${BUILD_DIR}/install" ]]; then
        cp -r "${BUILD_DIR}/install/"* "$data_dir/"
    else
        log "WARNING: No build/install dir found — creating empty data package."
    fi

    stamp_templates "$staging"

    # ── Sign all EXE / DLL before packaging ───────────────────────────────
    if [[ "$SKIP_SIGN" -eq 0 ]]; then
        sign_windows_binaries "$data_dir"
    fi

    # ── Build offline installer ───────────────────────────────────────────
    local out_name="${BRAND_NAME// /_}-${VERSION}-Setup.exe"
    local out_path="${BUILD_DIR}/${out_name}"

    command -v binarycreator >/dev/null 2>&1 \
        || die "binarycreator not found — install Qt Installer Framework."

    binarycreator \
        --offline-only \
        -c "${staging}/config/config.xml" \
        -p "${staging}/packages" \
        "$out_path"

    log "Installer created: $out_path"

    # ── Sign the installer itself ─────────────────────────────────────────
    if [[ "$SKIP_SIGN" -eq 0 ]]; then
        sign_single_file "$out_path"
    fi

    # ── Build online-only repository (for in-app updater) ─────────────────
    local repo_dir="${BUILD_DIR}/repository"
    rm -rf "$repo_dir"

    command -v repogen >/dev/null 2>&1 \
        || die "repogen not found — install Qt Installer Framework."

    repogen \
        -p "${staging}/packages" \
        "$repo_dir"

    log "Update repository created: $repo_dir"
    log "Upload $repo_dir to https://updates.kaivue.com/videowall/windows/${CHANNEL}/"
}

# ── EV code signing (supports both local USB token and Azure Key Vault) ──
sign_windows_binaries() {
    local dir="$1"
    log "Signing Windows binaries in $dir ..."
    local count=0
    while IFS= read -r -d '' f; do
        sign_single_file "$f"
        ((count++))
    done < <(find "$dir" -type f \( -iname '*.exe' -o -iname '*.dll' \) -print0)
    log "Signed $count files."
}

sign_single_file() {
    local file="$1"

    # Prefer Azure Key Vault (HSM-backed) signing if configured.
    if [[ -n "${AZURE_VAULT_URI:-}" ]]; then
        log "  AzureSignTool: $(basename "$file")"
        AzureSignTool sign \
            -kvu "$AZURE_VAULT_URI" \
            -kvc "$AZURE_CERT_NAME" \
            -kvt "$AZURE_TENANT_ID" \
            -kvi "$AZURE_CLIENT_ID" \
            -kvs "$AZURE_CLIENT_SECRET" \
            -tr "${CODESIGN_TIMESTAMP_URL:-http://timestamp.digicert.com}" \
            -td sha256 \
            "$file"
    elif [[ -n "${CODESIGN_CERT_THUMBPRINT:-}" ]]; then
        # Local USB EV token via signtool.
        log "  signtool: $(basename "$file")"
        signtool sign \
            /sha1 "$CODESIGN_CERT_THUMBPRINT" \
            /fd sha256 \
            /tr "${CODESIGN_TIMESTAMP_URL:-http://timestamp.digicert.com}" \
            /td sha256 \
            /v \
            "$file"
    else
        die "No code-signing credentials found. Set AZURE_VAULT_URI or CODESIGN_CERT_THUMBPRINT."
    fi
}

# ══════════════════════════════════════════════════════════════════════════
# LINUX — .deb (Ubuntu 22.04+) and .rpm (RHEL 9 / Fedora)
# ══════════════════════════════════════════════════════════════════════════
build_linux() {
    log "Building Linux packages..."
    build_deb
    build_rpm
}

build_deb() {
    log "Building .deb package..."
    local pkg_name
    pkg_name=$(echo "${BRAND_NAME}" | tr '[:upper:]' '[:lower:]' | tr ' ' '-')
    local deb_root="${BUILD_DIR}/deb-staging/${pkg_name}-${VERSION}"
    rm -rf "$deb_root"

    # Directory layout.
    mkdir -p "$deb_root/DEBIAN"
    mkdir -p "$deb_root/usr/bin"
    mkdir -p "$deb_root/usr/lib/${pkg_name}"
    mkdir -p "$deb_root/usr/share/applications"
    mkdir -p "$deb_root/usr/share/icons/hicolor/256x256/apps"
    mkdir -p "$deb_root/usr/share/doc/${pkg_name}"

    # Control file.
    cat > "$deb_root/DEBIAN/control" <<CTRL
Package: ${pkg_name}
Version: ${VERSION}
Section: video
Priority: optional
Architecture: amd64
Depends: libqt6core6 (>= 6.6), libqt6gui6 (>= 6.6), libqt6quick6 (>= 6.6), libqt6multimedia6 (>= 6.6), libvulkan1
Maintainer: ${BRAND_PUBLISHER} <support@kaivue.com>
Description: ${BRAND_NAME} — SOC video wall client
 Multi-monitor live camera view, map overlays, tour management,
 and PTZ control for security operations centers.
Homepage: https://kaivue.com
CTRL

    # Post-install: update desktop database.
    cat > "$deb_root/DEBIAN/postinst" <<'POSTINST'
#!/bin/sh
set -e
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database /usr/share/applications || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f /usr/share/icons/hicolor || true
fi
POSTINST
    chmod 0755 "$deb_root/DEBIAN/postinst"

    # Desktop entry.
    cat > "$deb_root/usr/share/applications/${pkg_name}.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=${BRAND_NAME}
Comment=SOC video wall client
Exec=/usr/bin/${pkg_name}
Icon=${pkg_name}
Categories=Video;Security;
Terminal=false
StartupWMClass=${pkg_name}
DESKTOP

    # Copyright.
    cp "${PROJECT_DIR}/installer/windows/packages/com.kaivue.videowall/meta/eula.txt" \
       "$deb_root/usr/share/doc/${pkg_name}/copyright"

    # Copy built binaries (if available).
    if [[ -d "${BUILD_DIR}/install" ]]; then
        cp -r "${BUILD_DIR}/install/"* "$deb_root/usr/lib/${pkg_name}/"
        # Symlink the main executable into /usr/bin.
        ln -sf "/usr/lib/${pkg_name}/KaivueVideoWall" "$deb_root/usr/bin/${pkg_name}"
    else
        log "WARNING: No build/install dir — .deb will have no binaries."
        touch "$deb_root/usr/bin/${pkg_name}"
        chmod +x "$deb_root/usr/bin/${pkg_name}"
    fi

    # Build.
    dpkg-deb --build --root-owner-group "$deb_root" "${BUILD_DIR}/${pkg_name}_${VERSION}_amd64.deb"
    log "Created: ${BUILD_DIR}/${pkg_name}_${VERSION}_amd64.deb"
}

build_rpm() {
    log "Building .rpm package..."
    local pkg_name
    pkg_name=$(echo "${BRAND_NAME}" | tr '[:upper:]' '[:lower:]' | tr ' ' '-')
    local rpm_root="${BUILD_DIR}/rpm-staging"
    rm -rf "$rpm_root"

    mkdir -p "$rpm_root"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

    # Create tarball from install dir.
    local src_tar="${rpm_root}/SOURCES/${pkg_name}-${VERSION}.tar.gz"
    if [[ -d "${BUILD_DIR}/install" ]]; then
        tar czf "$src_tar" -C "${BUILD_DIR}/install" .
    else
        log "WARNING: No build/install dir — tarball will be empty."
        mkdir -p /tmp/empty-rpm-src && tar czf "$src_tar" -C /tmp/empty-rpm-src .
    fi

    # Spec file.
    cat > "$rpm_root/SPECS/${pkg_name}.spec" <<SPEC
Name:           ${pkg_name}
Version:        ${VERSION}
Release:        1%{?dist}
Summary:        ${BRAND_NAME} — SOC video wall client
License:        Proprietary
URL:            https://kaivue.com
Source0:        ${pkg_name}-${VERSION}.tar.gz

Requires:       qt6-qtbase >= 6.6
Requires:       qt6-qtdeclarative >= 6.6
Requires:       qt6-qtmultimedia >= 6.6
Requires:       vulkan-loader

%description
Multi-monitor live camera view, map overlays, tour management,
and PTZ control for security operations centres.

%prep
%setup -c -T
tar xzf %{SOURCE0}

%install
mkdir -p %{buildroot}/usr/lib/${pkg_name}
cp -a * %{buildroot}/usr/lib/${pkg_name}/
mkdir -p %{buildroot}/usr/bin
ln -sf /usr/lib/${pkg_name}/KaivueVideoWall %{buildroot}/usr/bin/${pkg_name}

mkdir -p %{buildroot}/usr/share/applications
cat > %{buildroot}/usr/share/applications/${pkg_name}.desktop <<EOF
[Desktop Entry]
Type=Application
Name=${BRAND_NAME}
Comment=SOC video wall client
Exec=/usr/bin/${pkg_name}
Icon=${pkg_name}
Categories=Video;Security;
Terminal=false
EOF

%files
/usr/lib/${pkg_name}/
/usr/bin/${pkg_name}
/usr/share/applications/${pkg_name}.desktop

%changelog
* $(date '+%a %b %d %Y') ${BRAND_PUBLISHER} <support@kaivue.com> - ${VERSION}-1
- Initial package release for KAI-340.
SPEC

    rpmbuild \
        --define "_topdir $rpm_root" \
        -bb "$rpm_root/SPECS/${pkg_name}.spec"

    log "RPM created in: $rpm_root/RPMS/"
}

# ══════════════════════════════════════════════════════════════════════════
# MAIN
# ══════════════════════════════════════════════════════════════════════════
read_version
load_branding

case "$PLATFORM" in
    windows) build_windows ;;
    linux)   build_linux   ;;
    all)     build_windows; build_linux ;;
    *)       die "Unknown platform: $PLATFORM. Use windows, linux, or all." ;;
esac

log "Done."
