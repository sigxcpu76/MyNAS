#!/usr/bin/env bash
set -euo pipefail

# Scripts directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

BINARY_PATH="${1:-${REPO_ROOT}/bin/nasctl}"
RAW_VERSION="${2:-}"
ARCH="${3:-amd64}"
OUTPUT_DIR="${4:-${REPO_ROOT}/dist}"

# If no version specified, attempt to infer from git or binary
if [ -z "${RAW_VERSION}" ]; then
    if git -C "${REPO_ROOT}" describe --tags --always 2>/dev/null; then
        RAW_VERSION="$(git -C "${REPO_ROOT}" describe --tags --always 2>/dev/null)"
    else
        RAW_VERSION="1.0.0"
    fi
fi

# Clean version for Debian package (must start with digit, no leading 'v')
DEB_VERSION="${RAW_VERSION#v}"
# Replace any dashes in pre-release/dirty with tilde for dpkg sorting (e.g., 1.0.0-rc1 -> 1.0.0~rc1)
DEB_VERSION="$(echo "${DEB_VERSION}" | sed -E 's/-([a-zA-Z])/~\1/g')"
# If version doesn't start with a digit, prefix with 0.
if ! [[ "${DEB_VERSION}" =~ ^[0-9] ]]; then
    DEB_VERSION="0.${DEB_VERSION}"
fi

# Normalize architecture names
case "${ARCH}" in
    x86_64|amd64)
        DEB_ARCH="amd64"
        QEMU_PKG="qemu-system-x86 | qemu-system"
        ;;
    aarch64|arm64)
        DEB_ARCH="arm64"
        QEMU_PKG="qemu-system-arm | qemu-system"
        ;;
    *)
        DEB_ARCH="${ARCH}"
        QEMU_PKG="qemu-system"
        ;;
esac

if [ ! -f "${BINARY_PATH}" ]; then
    echo "Error: Binary not found at '${BINARY_PATH}'" >&2
    exit 1
fi

STAGING_DIR="$(mktemp -d /tmp/nasctl-deb-build-XXXXXX)"
trap 'rm -rf "${STAGING_DIR}"' EXIT
chmod 0755 "${STAGING_DIR}"

mkdir -p \
    "${STAGING_DIR}/DEBIAN" \
    "${STAGING_DIR}/usr/bin" \
    "${STAGING_DIR}/lib/systemd/system" \
    "${STAGING_DIR}/etc/default"

chmod 0755 "${STAGING_DIR}/DEBIAN" "${STAGING_DIR}/usr" "${STAGING_DIR}/usr/bin" \
           "${STAGING_DIR}/lib" "${STAGING_DIR}/lib/systemd" "${STAGING_DIR}/lib/systemd/system" \
           "${STAGING_DIR}/etc" "${STAGING_DIR}/etc/default"

# 1. Install binary
cp -p "${BINARY_PATH}" "${STAGING_DIR}/usr/bin/nasctl"
chmod 0755 "${STAGING_DIR}/usr/bin/nasctl"

# 2. Systemd service & default environment
cp -p "${SCRIPT_DIR}/nasctl.service" "${STAGING_DIR}/lib/systemd/system/nasctl.service"
chmod 0644 "${STAGING_DIR}/lib/systemd/system/nasctl.service"

cp -p "${SCRIPT_DIR}/nasctl.default" "${STAGING_DIR}/etc/default/nasctl"
chmod 0644 "${STAGING_DIR}/etc/default/nasctl"

# 3. Maintainer scripts & conffiles
cp -p "${SCRIPT_DIR}/postinst" "${STAGING_DIR}/DEBIAN/postinst"
chmod 0755 "${STAGING_DIR}/DEBIAN/postinst"

cp -p "${SCRIPT_DIR}/prerm" "${STAGING_DIR}/DEBIAN/prerm"
chmod 0755 "${STAGING_DIR}/DEBIAN/prerm"

cp -p "${SCRIPT_DIR}/postrm" "${STAGING_DIR}/DEBIAN/postrm"
chmod 0755 "${STAGING_DIR}/DEBIAN/postrm"

cp -p "${SCRIPT_DIR}/conffiles" "${STAGING_DIR}/DEBIAN/conffiles"
chmod 0644 "${STAGING_DIR}/DEBIAN/conffiles"

# 4. Calculate installed size (in KB)
INSTALLED_SIZE="$(du -ks "${STAGING_DIR}" | cut -f1)"

# 5. Render control file
sed \
    -e "s#\${DEB_VERSION}#${DEB_VERSION}#g" \
    -e "s#\${DEB_ARCH}#${DEB_ARCH}#g" \
    -e "s#\${INSTALLED_SIZE}#${INSTALLED_SIZE}#g" \
    -e "s#\${QEMU_PKG}#${QEMU_PKG}#g" \
    "${SCRIPT_DIR}/control.in" > "${STAGING_DIR}/DEBIAN/control"
chmod 0644 "${STAGING_DIR}/DEBIAN/control"

# 6. Build package using dpkg-deb
mkdir -p "${OUTPUT_DIR}"
OUTPUT_DEB="${OUTPUT_DIR}/nasctl_${DEB_VERSION}_${DEB_ARCH}.deb"

echo "Building Debian package: ${OUTPUT_DEB}..."
dpkg-deb --root-owner-group --build "${STAGING_DIR}" "${OUTPUT_DEB}"

# 7. Generate sha256 checksum
(
    cd "${OUTPUT_DIR}"
    sha256sum "$(basename "${OUTPUT_DEB}")" > "$(basename "${OUTPUT_DEB}").sha256"
)

echo "Successfully built Debian package: ${OUTPUT_DEB}"
dpkg-deb --info "${OUTPUT_DEB}"
