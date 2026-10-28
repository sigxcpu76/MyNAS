#!/usr/bin/env bash
# ==============================================================================
# mynas Appliance ISO Master Build Script
# Creates a hybrid UEFI + Legacy BIOS bootable ISO with Debian 13 & OpenZFS
# ==============================================================================

set -euo pipefail

BASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ISO_DIR="$BASE_DIR/iso"
CONF_DIR="$ISO_DIR/conf"
BUILD_DIR="$ISO_DIR/build"
CHROOT_DIR="$BUILD_DIR/chroot"
ISO_ROOT="$BUILD_DIR/iso_root"
OUTPUT_DIR="$ISO_DIR/output"
OUTPUT_ISO="$OUTPUT_DIR/mynas-appliance-amd64.iso"

echo "=============================================================="
echo "          mynas Appliance ISO Build Pipeline                  "
echo "=============================================================="
echo "Base Directory: $BASE_DIR"
echo "Output ISO:     $OUTPUT_ISO"
echo "=============================================================="

# 1. Verify nasctl binary
if [ ! -f "$BASE_DIR/bin/nasctl" ]; then
    echo ">> [1/7] Building nasctl binary..."
    mkdir -p "$BASE_DIR/bin"
    (cd "$BASE_DIR" && CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/nasctl cmd/nasctl/main.go)
else
    echo ">> [1/7] Found existing nasctl binary at $BASE_DIR/bin/nasctl"
fi

mkdir -p "$OUTPUT_DIR" "$BUILD_DIR" "$ISO_ROOT/live" "$ISO_ROOT/boot/grub" "$ISO_ROOT/isolinux"

# Cleanup trap for chroot mounts
cleanup() {
    echo ">> Cleaning up chroot mounts..."
    if [ -d "$CHROOT_DIR" ]; then
        umount -l "$CHROOT_DIR/dev/pts" 2>/dev/null || true
        umount -l "$CHROOT_DIR/dev" 2>/dev/null || true
        umount -l "$CHROOT_DIR/sys" 2>/dev/null || true
        umount -l "$CHROOT_DIR/proc" 2>/dev/null || true
    fi
}
trap cleanup EXIT INT TERM

# 2. Debootstrap Debian 13 (Trixie) base system
if [ ! -f "$CHROOT_DIR/bin/bash" ]; then
    echo ">> [2/7] Running debootstrap for Debian 13 (Trixie) amd64..."
    rm -rf "$CHROOT_DIR"
    mkdir -p "$CHROOT_DIR"
    debootstrap --arch=amd64 \
        --components=main,contrib,non-free,non-free-firmware \
        --include=ca-certificates,curl,gnupg \
        trixie "$CHROOT_DIR" http://deb.debian.org/debian
else
    echo ">> [2/7] Chroot base system already exists at $CHROOT_DIR"
fi

# Mount essential filesystems in chroot
mount -t proc none "$CHROOT_DIR/proc"
mount -t sysfs none "$CHROOT_DIR/sys"
mount --bind /dev "$CHROOT_DIR/dev"
mount --bind /dev/pts "$CHROOT_DIR/dev/pts"

# 3. Configure repositories & install packages
echo ">> [3/7] Configuring APT sources and installing appliance packages..."
mkdir -p "$CHROOT_DIR/etc/apt/trusted.gpg.d" "$CHROOT_DIR/etc/apt/keyrings"
curl -fsSL https://enterprise.proxmox.com/debian/proxmox-release-trixie.gpg -o "$CHROOT_DIR/etc/apt/trusted.gpg.d/proxmox-release-trixie.gpg"
curl -fsSL https://download.docker.com/linux/debian/gpg -o "$CHROOT_DIR/etc/apt/keyrings/docker.asc"
cp "$CONF_DIR/apt/sources.list" "$CHROOT_DIR/etc/apt/sources.list"

# Set up policy to prevent daemons from starting inside chroot during build
cat << 'EOF' > "$CHROOT_DIR/usr/sbin/policy-rc.d"
#!/bin/sh
exit 101
EOF
chmod +x "$CHROOT_DIR/usr/sbin/policy-rc.d"

# Update package lists inside chroot
chroot "$CHROOT_DIR" apt-get update -y

# Read packages list (ignoring comments and empty lines)
PKG_LIST=$(grep -vE '^\s*#' "$CONF_DIR/packages.list" | grep -vE '^\s*$' | tr '\n' ' ')

# Also include live boot dependencies
LIVE_PKGS="live-boot live-boot-doc live-config live-config-systemd"

echo ">> Installing packages into appliance rootfs..."
chroot "$CHROOT_DIR" apt-get install -y --no-install-recommends $PKG_LIST $LIVE_PKGS

# Clean up apt caches to minimize ISO size
chroot "$CHROOT_DIR" apt-get clean
rm -rf "$CHROOT_DIR/var/lib/apt/lists/"*
rm -f "$CHROOT_DIR/usr/sbin/policy-rc.d"

# 4. Inject nasctl and system configurations
echo ">> [4/7] Ingesting appliance services, sysctls, and installer wizard..."
cp "$BASE_DIR/bin/nasctl" "$CHROOT_DIR/usr/local/bin/nasctl"
chmod +x "$CHROOT_DIR/usr/local/bin/nasctl"

# Systemd units
cp "$CONF_DIR/systemd/nasctl.service" "$CHROOT_DIR/etc/systemd/system/nasctl.service"
cp "$CONF_DIR/systemd/nas-banner.service" "$CHROOT_DIR/etc/systemd/system/nas-banner.service"
cp "$CONF_DIR/systemd/nas-banner.sh" "$CHROOT_DIR/usr/local/bin/nas-banner.sh"
chmod +x "$CHROOT_DIR/usr/local/bin/nas-banner.sh"

# Pre-enable nasctl and nas-banner systemd services
mkdir -p "$CHROOT_DIR/etc/systemd/system/multi-user.target.wants"
ln -sf /etc/systemd/system/nasctl.service "$CHROOT_DIR/etc/systemd/system/multi-user.target.wants/nasctl.service"
ln -sf /etc/systemd/system/nas-banner.service "$CHROOT_DIR/etc/systemd/system/multi-user.target.wants/nas-banner.service"
chroot "$CHROOT_DIR" systemctl enable nasctl.service 2>/dev/null || true
chroot "$CHROOT_DIR" systemctl enable nas-banner.service 2>/dev/null || true

# Ensure libcrypt.so.1 is symlinked to libcrypt.so.2
ln -sf libcrypt.so.1 "$CHROOT_DIR/usr/lib/x86_64-linux-gnu/libcrypt.so.2"
ln -sf /usr/lib/x86_64-linux-gnu/libcrypt.so.1 "$CHROOT_DIR/usr/lib/libcrypt.so.2"
ln -sf /usr/lib/x86_64-linux-gnu/libcrypt.so.1 "$CHROOT_DIR/lib64/libcrypt.so.2"
chroot "$CHROOT_DIR" ldconfig 2>/dev/null || true

# Sysctl configs
cp "$CONF_DIR/sysctl.d/"*.conf "$CHROOT_DIR/etc/sysctl.d/"

# Installer script
cp "$ISO_DIR/installer/install.sh" "$CHROOT_DIR/usr/local/bin/mynas-installer"
chmod +x "$CHROOT_DIR/usr/local/bin/mynas-installer"

# Autostart installer on console boot via tty1 autologin / service
mkdir -p "$CHROOT_DIR/etc/systemd/system/getty@tty1.service.d"
cat << 'EOF' > "$CHROOT_DIR/etc/systemd/system/getty@tty1.service.d/override.conf"
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM
EOF

mkdir -p "$CHROOT_DIR/etc/systemd/system/serial-getty@ttyS0.service.d"
cat << 'EOF' > "$CHROOT_DIR/etc/systemd/system/serial-getty@ttyS0.service.d/override.conf"
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --keep-baud 115200,38400,9600 %I $TERM
EOF

# Launch installer automatically when logging into root shell in live mode
cat << 'EOF' >> "$CHROOT_DIR/root/.bashrc"
if [ -z "$MYNAS_INSTALLER_RAN" ] && [ -x /usr/local/bin/mynas-installer ]; then
    export MYNAS_INSTALLER_RAN=1
    /usr/local/bin/mynas-installer
fi
EOF

# Ensure root password is set for rescue mode
echo "root:mynas" | chroot "$CHROOT_DIR" chpasswd

# Strip consumer GPU/WiFi firmware, sound/media drivers, and documentation to optimize ISO size
echo ">> [4b/7] Stripping consumer GPU/WiFi firmware and non-server kernel modules..."

# 1. Strip consumer desktop GPU firmware
rm -rf "$CHROOT_DIR/usr/lib/firmware/nvidia" \
       "$CHROOT_DIR/usr/lib/firmware/amdgpu" \
       "$CHROOT_DIR/usr/lib/firmware/radeon" \
       "$CHROOT_DIR/usr/lib/firmware/xe" \
       "$CHROOT_DIR/usr/lib/firmware/cirrus" \
       "$CHROOT_DIR/usr/lib/firmware/amdnpu"

# 2. Strip consumer WiFi and Bluetooth firmware (servers use wired Ethernet / SFP+)
rm -rf "$CHROOT_DIR/usr/lib/firmware/ath10k" \
       "$CHROOT_DIR/usr/lib/firmware/ath11k" \
       "$CHROOT_DIR/usr/lib/firmware/ath12k" \
       "$CHROOT_DIR/usr/lib/firmware/brcm" \
       "$CHROOT_DIR/usr/lib/firmware/cypress" \
       "$CHROOT_DIR/usr/lib/firmware/mediatek" \
       "$CHROOT_DIR/usr/lib/firmware/rtw89" \
       "$CHROOT_DIR/usr/lib/firmware/rtl_bt" \
       "$CHROOT_DIR/usr/lib/firmware/libertas" \
       "$CHROOT_DIR/usr/lib/firmware/ti-connectivity" \
       "$CHROOT_DIR/usr/lib/firmware/mrvl"

# 3. Strip non-server kernel modules (audio, TV tuners, consumer sensors, wireless)
PVE_KVER=$(ls "$CHROOT_DIR/usr/lib/modules" | grep pve | head -n 1)
if [ -n "$PVE_KVER" ]; then
    KMOD_DIR="$CHROOT_DIR/usr/lib/modules/$PVE_KVER/kernel"
    rm -rf "$KMOD_DIR/sound" \
           "$KMOD_DIR/drivers/media" \
           "$KMOD_DIR/drivers/iio" \
           "$KMOD_DIR/drivers/net/wireless" \
           "$KMOD_DIR/drivers/comedi" \
           "$KMOD_DIR/drivers/isdn" \
           "$KMOD_DIR/drivers/net/can" \
           "$KMOD_DIR/net/wireless" \
           "$KMOD_DIR/net/mac80211" \
           "$KMOD_DIR/net/bluetooth"
    # Rebuild module dependency maps
    chroot "$CHROOT_DIR" depmod -a "$PVE_KVER"
fi

# 4. Strip documentation, man pages, locales, and dev headers
rm -rf "$CHROOT_DIR/usr/share/doc/"* \
       "$CHROOT_DIR/usr/share/man/"* \
       "$CHROOT_DIR/usr/share/info/"* \
       "$CHROOT_DIR/usr/include/"*

# Configure initramfs for Live ISO boot
echo ">> [4c/7] Configuring initramfs for Live boot..."
mkdir -p "$CHROOT_DIR/etc/initramfs-tools/conf.d"
cat << 'EOF' > "$CHROOT_DIR/etc/initramfs-tools/conf.d/live.conf"
BOOT=live
UNIONTYPE=overlay
EOF

# Ensure all essential block and CD-ROM drivers are included in initramfs
cat << 'EOF' > "$CHROOT_DIR/etc/initramfs-tools/modules"
# Filesystems & live overlay
isofs
cdrom
sr_mod
overlay
squashfs
loop
# Virtualization & storage controllers
virtio_blk
virtio_scsi
virtio_pci
ahci
ata_piix
sd_mod
EOF

# Rebuild initramfs with live-boot hooks and storage modules
echo ">> Updating initramfs with storage and live-boot drivers..."
chroot "$CHROOT_DIR" update-initramfs -u -k all

# Unmount virtual filesystems from chroot before packaging
umount -l "$CHROOT_DIR/dev/pts" 2>/dev/null || true
umount -l "$CHROOT_DIR/dev" 2>/dev/null || true
umount -l "$CHROOT_DIR/sys" 2>/dev/null || true
umount -l "$CHROOT_DIR/proc" 2>/dev/null || true

# 5. Extract kernel and initrd, build squashfs
echo ">> [5/7] Preparing kernel, initramfs, and SquashFS image..."
KERNEL_IMG=$(ls -1t "$CHROOT_DIR/boot/vmlinuz"* 2>/dev/null | head -n 1)
INITRD_IMG=$(ls -1t "$CHROOT_DIR/boot/initrd.img"* 2>/dev/null | head -n 1)

if [ -z "$KERNEL_IMG" ] || [ -z "$INITRD_IMG" ]; then
    echo "Error: Could not locate vmlinuz or initrd.img in chroot /boot!" >&2
    exit 1
fi

cp "$KERNEL_IMG" "$ISO_ROOT/live/vmlinuz"
cp "$INITRD_IMG" "$ISO_ROOT/live/initrd.img"

SQUASHFS_COMP="${SQUASHFS_COMP:-lz4}"
NPROC=$(nproc 2>/dev/null || echo 4)
echo ">> Generating filesystem.squashfs (compression: $SQUASHFS_COMP, processors: $NPROC)..."
rm -f "$ISO_ROOT/live/filesystem.squashfs"

if [ "$SQUASHFS_COMP" = "lz4" ]; then
    COMP_ARGS=(-comp lz4 -b 256K -processors "$NPROC")
elif [ "$SQUASHFS_COMP" = "zstd" ]; then
    COMP_ARGS=(-comp zstd -Xcompression-level 1 -b 256K -processors "$NPROC")
else
    COMP_ARGS=(-comp "$SQUASHFS_COMP" -processors "$NPROC")
fi

# Ensure empty mount point directories exist in chroot
mkdir -p "$CHROOT_DIR/proc" "$CHROOT_DIR/sys" "$CHROOT_DIR/dev" "$CHROOT_DIR/run" \
         "$CHROOT_DIR/tmp" "$CHROOT_DIR/mnt" "$CHROOT_DIR/media" "$CHROOT_DIR/boot/efi"
chmod 1777 "$CHROOT_DIR/tmp"

mksquashfs "$CHROOT_DIR" "$ISO_ROOT/live/filesystem.squashfs" \
    "${COMP_ARGS[@]}" \
    -e boot/vmlinuz* boot/initrd.img*

# 6. Setup bootloaders in ISO root
echo ">> [6/7] Configuring bootloader menus for UEFI and BIOS..."
mkdir -p "$ISO_ROOT/.disk"
echo "mynas Appliance Live Installer" > "$ISO_ROOT/.disk/info"
touch "$ISO_ROOT/.disk/base_installable"

cp "$ISO_DIR/boot/grub/grub.cfg" "$ISO_ROOT/boot/grub/grub.cfg"
cp "$ISO_DIR/boot/isolinux/isolinux.cfg" "$ISO_ROOT/isolinux/isolinux.cfg"

# Copy syslinux/isolinux binaries if available
for bin in /usr/lib/ISOLINUX/isolinux.bin /usr/lib/syslinux/modules/bios/vesamenu.c32 /usr/lib/syslinux/modules/bios/ldlinux.c32 /usr/lib/syslinux/modules/bios/libcom32.c32 /usr/lib/syslinux/modules/bios/libutil.c32; do
    if [ -f "$bin" ]; then
        cp "$bin" "$ISO_ROOT/isolinux/"
    fi
done

# 7. Create Hybrid ISO
echo ">> [7/7] Generating hybrid bootable ISO image with grub-mkrescue / xorriso..."
if command -v grub-mkrescue >/dev/null 2>&1; then
    grub-mkrescue -o "$OUTPUT_ISO" "$ISO_ROOT" \
        -- -volid "MYNAS_INSTALLER"
elif command -v xorriso >/dev/null 2>&1; then
    xorriso -as mkisofs \
        -r -V "MYNAS_INSTALLER" \
        -o "$OUTPUT_ISO" \
        -graft-points \
        "$ISO_ROOT"
else
    echo "Error: Neither grub-mkrescue nor xorriso is available." >&2
    exit 1
fi

echo "=============================================================="
echo ">> SUCCESS! mynas appliance ISO successfully built:"
echo "   Path:   $OUTPUT_ISO"
echo "   Size:   $(du -h "$OUTPUT_ISO" | awk '{print $1}')"
echo "   SHA256: $(sha256sum "$OUTPUT_ISO" | awk '{print $1}')"
echo "=============================================================="
