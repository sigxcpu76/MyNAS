#!/bin/bash
# ==============================================================================
# mynas Appliance Interactive TUI Installer
# Debian 13 (Trixie) + Proxmox 7.0 Kernel + Root-on-ZFS + br0 Bridge
# ==============================================================================

set -o pipefail

# Ensure execution as root
if [ "$(id -u)" -ne 0 ]; then
    echo "This installer must be run as root." >&2
    exit 1
fi

TITLE="mynas Appliance Installer"

# ------------------------------------------------------------------------------
# Helper: Check dependencies
# ------------------------------------------------------------------------------
for cmd in whiptail zpool zfs sgdisk mkfs.vfat rsync lsblk; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "Error: Required utility '$cmd' is missing from installer environment." >&2
        exit 1
    fi
done

if ! modprobe zfs 2>/dev/null; then
    if [ ! -e /dev/zfs ] && [ ! -d /sys/module/zfs ]; then
        whiptail --title "$TITLE: Error" --msgbox \
"The ZFS kernel module could not be loaded!
Please ensure the installer booted with a ZFS-capable kernel (proxmox-kernel)." 10 65
        exit 1
    fi
fi

# ------------------------------------------------------------------------------
# Step 1: Welcome Dialog
# ------------------------------------------------------------------------------
whiptail --title "$TITLE" --msgbox \
"Welcome to the mynas Appliance Installer!

This wizard will guide you through installing the appliance onto dedicated OS drive(s) with:
  • Debian 13 (Trixie) + Proxmox 7.0 OpenZFS Kernel
  • Mirrored or Single Root-on-ZFS (rpool)
  • UEFI & Legacy BIOS Hybrid Bootloaders
  • Automatic 'br0' Bridge for Host, VMs, and Containers

Data storage drives (for bulk pools) should NOT be selected; they will be managed within the Web UI after boot." 16 72

# ------------------------------------------------------------------------------
# Step 2: Drive Discovery & Selection
# ------------------------------------------------------------------------------
get_live_media_devs() {
    # Find block devices that back the live installer environment or optical media
    local live_sources
    live_sources=$(findmnt -n -o SOURCE / 2>/dev/null; findmnt -n -o SOURCE /run/live/medium 2>/dev/null; findmnt -n -o SOURCE /lib/live/mount/medium 2>/dev/null)
    for src in $live_sources; do
        if [ -b "$src" ]; then
            lsblk -n -o PKNAME "$src" 2>/dev/null || basename "$src"
        fi
    done
}

LIVE_EXCLUDES=$(get_live_media_devs)

while true; do
    # Enumerate disks (excluding loop, optical, ram, zvols, and live boot media)
    DISK_OPTIONS=()
    while read -r name size model tran type; do
        [ "$type" != "disk" ] && continue
        case "$name" in
            loop*|sr*|ram*|zd*|zvol*) continue ;;
        esac

        # Check against live installer media
        is_live=0
        for excl in $LIVE_EXCLUDES; do
            if [ "$name" = "$excl" ]; then
                is_live=1
                break
            fi
        done
        [ "$is_live" -eq 1 ] && continue

        dev_path="/dev/$name"
        label="$size | ${tran^^} | $model"
        # Truncate label if too long
        label=$(echo "$label" | cut -c1-45)
        DISK_OPTIONS+=("$dev_path" "$label" "OFF")
    done < <(lsblk -d -n -o NAME,SIZE,MODEL,TRAN,TYPE 2>/dev/null)

    if [ ${#DISK_OPTIONS[@]} -eq 0 ]; then
        whiptail --title "$TITLE: Error" --msgbox \
"No suitable installation drives were detected!
Please check virtual disk connections or SATA/NVMe controllers." 10 60
        exit 1
    fi

    SELECTED_DRIVES=$(whiptail --title "$TITLE: Select OS Drive(s)" --checklist \
"Select 1 or 2 drives for the Root Filesystem (rpool).
  • 1 drive  -> Single vdev root pool
  • 2 drives -> Mirrored root pool (mirror rpool)

Press SPACE to select, ENTER to confirm." 18 74 6 \
"${DISK_OPTIONS[@]}" 3>&1 1>&2 2>&3)

    if [ $? -ne 0 ]; then
        whiptail --title "$TITLE" --yesno "Abort installation and return to shell?" 8 50 && exit 0
        continue
    fi

    # Parse selected drives
    eval "DRIVE_ARRAY=($SELECTED_DRIVES)"
    NUM_DRIVES=${#DRIVE_ARRAY[@]}

    if [ "$NUM_DRIVES" -eq 0 ]; then
        whiptail --title "$TITLE: Warning" --msgbox "You must select at least one drive for the OS." 8 60
        continue
    elif [ "$NUM_DRIVES" -gt 2 ]; then
        whiptail --title "$TITLE: Warning" --msgbox \
"You selected $NUM_DRIVES drives. The root pool supports 1 drive (single) or 2 drives (mirror).
Bulk data pools should be created from the Web UI after installation." 10 65
        continue
    fi

    break
done

PRIMARY_DRIVE="${DRIVE_ARRAY[0]}"
SECONDARY_DRIVE=""
[ "$NUM_DRIVES" -eq 2 ] && SECONDARY_DRIVE="${DRIVE_ARRAY[1]}"

# ------------------------------------------------------------------------------
# Step 3: Hostname
# ------------------------------------------------------------------------------
HOSTNAME=$(whiptail --title "$TITLE: Hostname" --inputbox \
"Enter the appliance system hostname:" 10 60 "mynas" 3>&1 1>&2 2>&3)
if [ $? -ne 0 ] || [ -z "$HOSTNAME" ]; then
    HOSTNAME="mynas"
fi

# ------------------------------------------------------------------------------
# Step 4: Root Password
# ------------------------------------------------------------------------------
while true; do
    ROOT_PASS=$(whiptail --title "$TITLE: Root Password" --passwordbox \
"Enter the root administrator password for the appliance (SSH & Web UI):" 10 60 3>&1 1>&2 2>&3)
    if [ $? -ne 0 ]; then
        whiptail --title "$TITLE" --yesno "Abort installation?" 8 40 && exit 0
        continue
    fi

    if [ ${#ROOT_PASS} -lt 4 ]; then
        whiptail --title "$TITLE: Error" --msgbox "Password must be at least 4 characters." 8 50
        continue
    fi

    ROOT_PASS_CONFIRM=$(whiptail --title "$TITLE: Confirm Password" --passwordbox \
"Confirm the root administrator password:" 10 60 3>&1 1>&2 2>&3)

    if [ "$ROOT_PASS" != "$ROOT_PASS_CONFIRM" ]; then
        whiptail --title "$TITLE: Error" --msgbox "Passwords do not match. Please try again." 8 50
        continue
    fi
    break
done

# ------------------------------------------------------------------------------
# Step 5: Network Uplink & IP Configuration
# ------------------------------------------------------------------------------
NET_OPTIONS=()
for iface in /sys/class/net/*; do
    [ ! -d "$iface" ] && continue
    iname=$(basename "$iface")
    case "$iname" in
        lo|docker*|veth*|br*|virbr*|dummy*) continue ;;
    esac

    carrier=0
    [ -f "$iface/carrier" ] && carrier=$(cat "$iface/carrier" 2>/dev/null || echo 0)
    mac=$(cat "$iface/address" 2>/dev/null || echo "unknown")

    status="NO CARRIER"
    [ "$carrier" = "1" ] && status="CONNECTED"

    NET_OPTIONS+=("$iname" "[$status] MAC: $mac")
done

if [ ${#NET_OPTIONS[@]} -eq 0 ]; then
    whiptail --title "$TITLE: Error" --msgbox "No physical network interfaces found!" 8 50
    exit 1
fi

SELECTED_IFACE=$(whiptail --title "$TITLE: Primary Network Interface" --menu \
"Select the physical network interface to bind into the 'br0' bridge:
(Connected interfaces with an active link are marked [CONNECTED])" 16 70 6 \
"${NET_OPTIONS[@]}" 3>&1 1>&2 2>&3)

if [ $? -ne 0 ] || [ -z "$SELECTED_IFACE" ]; then
    SELECTED_IFACE="${NET_OPTIONS[0]}"
fi

IP_MODE=$(whiptail --title "$TITLE: Network IP Mode" --menu \
"Select IP addressing for bridge br0:" 12 60 2 \
"dhcp"   "DHCP (Automatic from router - Recommended)" \
"static" "Static IP (Manual configuration)" 3>&1 1>&2 2>&3)

STATIC_IP=""
STATIC_NETMASK="255.255.255.0"
STATIC_GW=""
STATIC_DNS="1.1.1.1"

if [ "$IP_MODE" = "static" ]; then
    STATIC_IP=$(whiptail --title "$TITLE: Static IP" --inputbox \
"Enter IPv4 Address (e.g. 192.168.1.50):" 10 60 "" 3>&1 1>&2 2>&3)
    STATIC_NETMASK=$(whiptail --title "$TITLE: Netmask" --inputbox \
"Enter Subnet Mask:" 10 60 "255.255.255.0" 3>&1 1>&2 2>&3)
    STATIC_GW=$(whiptail --title "$TITLE: Gateway" --inputbox \
"Enter Default Gateway (e.g. 192.168.1.1):" 10 60 "" 3>&1 1>&2 2>&3)
    STATIC_DNS=$(whiptail --title "$TITLE: DNS Nameserver" --inputbox \
"Enter Primary DNS Server:" 10 60 "1.1.1.1" 3>&1 1>&2 2>&3)
fi

# ------------------------------------------------------------------------------
# Step 6: Confirmation Screen
# ------------------------------------------------------------------------------
POOL_TYPE="Single Drive"
[ "$NUM_DRIVES" -eq 2 ] && POOL_TYPE="Mirror (2 Drives)"

SUMMARY_TEXT="Ready to install mynas appliance with the following configuration:

  • Hostname:        $HOSTNAME
  • Target Drives:   ${DRIVE_ARRAY[*]}
  • ZFS Root Pool:   rpool ($POOL_TYPE)
  • Network Uplink:  $SELECTED_IFACE -> br0 ($IP_MODE)
$( [ "$IP_MODE" = "static" ] && echo "  • Static IPv4:     $STATIC_IP / $STATIC_NETMASK (GW: $STATIC_GW)" )

WARNING: ALL EXISTING DATA ON ${DRIVE_ARRAY[*]} WILL BE PERMANENTLY DESTROYED!

Do you want to proceed?"

whiptail --title "$TITLE: CONFIRM INSTALLATION" --yesno "$SUMMARY_TEXT" 18 72 --defaultno
if [ $? -ne 0 ]; then
    whiptail --title "$TITLE" --msgbox "Installation canceled by user." 8 45
    exit 0
fi

# ------------------------------------------------------------------------------
# Step 7: Execution Phase with Progress Gauge
# ------------------------------------------------------------------------------
INSTALL_LOG="/tmp/mynas-install.log"
exec > >(tee -a "$INSTALL_LOG") 2>&1

TARGET="/target"
mkdir -p "$TARGET"

run_step() {
    local pct=$1
    local msg=$2
    echo "XXX"
    echo "$pct"
    echo "$msg"
    echo "XXX"
}

{
    run_step 5 "Wiping existing partition tables..."
    for drv in "${DRIVE_ARRAY[@]}"; do
        swapoff "$drv"* 2>/dev/null || true
        wipefs -a -f "$drv" || true
        sgdisk --zap-all "$drv" || true
        dd if=/dev/zero of="$drv" bs=1M count=10 conv=notrunc status=none || true
    done
    sleep 1

    run_step 15 "Partitioning target drive(s) with GPT layout..."
    # Partition layout:
    # 1: 1MB BIOS Boot partition (EF02)
    # 2: 1GB EFI System Partition (EF00)
    # 3: Remainder Solaris / ZFS root partition (BF00)
    for drv in "${DRIVE_ARRAY[@]}"; do
        sgdisk -n 1:2048:+1M   -c 1:"BIOS-BOOT" -t 1:EF02 "$drv"
        sgdisk -n 2:4096:+1G   -c 2:"EFI-SYSTEM" -t 2:EF00 "$drv"
        sgdisk -n 3:0:0        -c 3:"ZFS-ROOT"   -t 3:BF00 "$drv"
    done
    udevadm settle || sleep 2

    run_step 25 "Formatting EFI System Partition(s)..."
    get_part() {
        local disk=$1
        local num=$2
        if [[ "$disk" =~ [0-9]$ ]]; then
            echo "${disk}p${num}"
        else
            echo "${disk}${num}"
        fi
    }

    ESP1=$(get_part "$PRIMARY_DRIVE" 2)
    ZFS_PART1=$(get_part "$PRIMARY_DRIVE" 3)
    mkfs.vfat -F32 -n "EFI-SYSTEM" "$ESP1"

    ESP2=""
    ZFS_PART2=""
    if [ -n "$SECONDARY_DRIVE" ]; then
        ESP2=$(get_part "$SECONDARY_DRIVE" 2)
        ZFS_PART2=$(get_part "$SECONDARY_DRIVE" 3)
        mkfs.vfat -F32 -n "EFI-SYS-2" "$ESP2"
    fi

    run_step 35 "Creating Root-on-ZFS pool (rpool)..."
    modprobe zfs 2>/dev/null || true

    # Resolve stable by-id paths if available, otherwise use partition path
    resolve_by_id() {
        local part=$1
        local real_by_id
        real_by_id=$(find /dev/disk/by-id/ -type l -exec test "$(readlink -f "{}")" = "$part" \; -print 2>/dev/null | grep -v 'wwn-' | head -n 1)
        if [ -n "$real_by_id" ]; then
            echo "$real_by_id"
        else
            echo "$part"
        fi
    }

    PART1_ID=$(resolve_by_id "$ZFS_PART1")
    ZPOOL_DEV_SPEC="$PART1_ID"
    if [ -n "$SECONDARY_DRIVE" ]; then
        PART2_ID=$(resolve_by_id "$ZFS_PART2")
        ZPOOL_DEV_SPEC="mirror $PART1_ID $PART2_ID"
    fi

    # Create root pool with GRUB2 compatibility options
    if ! zpool create -f -o ashift=12 \
        -o compatibility=grub2 \
        -O acltype=posixacl \
        -O xattr=sa \
        -O relatime=on \
        -O canmount=off \
        -O mountpoint=none \
        -R "$TARGET" rpool $ZPOOL_DEV_SPEC; then
        echo "ERROR: Failed to create root ZFS pool 'rpool' on $ZPOOL_DEV_SPEC" >&2
        exit 1
    fi

    # Create dataset hierarchy
    zfs create -o canmount=off -o mountpoint=none rpool/ROOT
    zfs create -o mountpoint=/ rpool/ROOT/debian
    zpool set bootfs=rpool/ROOT/debian rpool

    run_step 50 "Extracting appliance root filesystem into /target..."
    # Copy from live rootfs to target
    rsync -aAX --info=progress2 \
        --exclude='/target' \
        --exclude='/proc/*' \
        --exclude='/sys/*' \
        --exclude='/dev/*' \
        --exclude='/run/*' \
        --exclude='/tmp/*' \
        --exclude='/mnt/*' \
        --exclude='/media/*' \
        --exclude='/var/tmp/*' \
        --exclude='/lost+found' \
        / "$TARGET/"

    mkdir -p "$TARGET/proc" "$TARGET/sys" "$TARGET/dev" "$TARGET/run" "$TARGET/tmp" "$TARGET/mnt" "$TARGET/media" "$TARGET/boot/efi"
    chmod 1777 "$TARGET/tmp"

    run_step 70 "Configuring hostname, networking, and hostid..."
    echo "$HOSTNAME" > "$TARGET/etc/hostname"
    cat <<EOF > "$TARGET/etc/hosts"
127.0.0.1   localhost $HOSTNAME
::1         localhost ip6-localhost ip6-loopback
EOF

    # Generate 8-digit HostID for ZFS
    zgenhostid -f -o "$TARGET/etc/hostid" 2>/dev/null || zgenhostid -f 2>/dev/null || true
    if [ -f "$TARGET/etc/hostid" ]; then
        cp "$TARGET/etc/hostid" /etc/hostid 2>/dev/null || true
    elif [ -f /etc/hostid ]; then
        cp /etc/hostid "$TARGET/etc/hostid" 2>/dev/null || true
    fi

    # Network configuration (br0 with ifupdown2)
    STATIC_BLOCK=""
    BOOTPROTO="dhcp"
    if [ "$IP_MODE" = "static" ]; then
        BOOTPROTO="static"
        STATIC_BLOCK="    address ${STATIC_IP}
    netmask ${STATIC_NETMASK}
    gateway ${STATIC_GW}
    dns-nameservers ${STATIC_DNS}"
    fi

    cat <<EOF > "$TARGET/etc/network/interfaces"
# Managed by mynas appliance installer
auto lo
iface lo inet loopback

# Physical interface attached to br0
auto ${SELECTED_IFACE}
iface ${SELECTED_IFACE} inet manual

# Primary Appliance Bridge for Host, VMs, and Containers
auto br0
iface br0 inet ${BOOTPROTO}
    bridge_ports ${SELECTED_IFACE}
    bridge_stp off
    bridge_fd 0
    bridge_maxwait 0
${STATIC_BLOCK}
EOF

    # Write /etc/fstab for EFI System Partition
    ESP_UUID=$(blkid -s UUID -o value "$ESP1")
    cat <<EOF > "$TARGET/etc/fstab"
# /etc/fstab: static file system information.
UUID=${ESP_UUID}  /boot/efi  vfat  umask=0077,fmask=0077,dmask=0077  0  1
EOF

    run_step 80 "Setting root password and enabling services..."
    # Set root password
    echo "root:$ROOT_PASS" | chroot "$TARGET" chpasswd

    # Clean up live-installer specific artifacts from target rootfs
    rm -f "$TARGET/etc/initramfs-tools/conf.d/live.conf"
    rm -f "$TARGET/etc/systemd/system/getty@tty1.service.d/override.conf"
    rm -f "$TARGET/etc/systemd/system/serial-getty@ttyS0.service.d/override.conf"
    rm -f "$TARGET/usr/local/bin/mynas-installer"
    sed -i '/MYNAS_INSTALLER_RAN/,/fi/d' "$TARGET/root/.bashrc" 2>/dev/null || true
    sed -i '/mynas-installer/d' "$TARGET/root/.bashrc" 2>/dev/null || true
    cp /etc/resolv.conf "$TARGET/etc/resolv.conf" 2>/dev/null || echo "nameserver 1.1.1.1" > "$TARGET/etc/resolv.conf"

    # Compatibility symlink: libcrypt.so.1 to libcrypt.so.2
    ln -sf libcrypt.so.1 "$TARGET/usr/lib/x86_64-linux-gnu/libcrypt.so.2"
    ln -sf /usr/lib/x86_64-linux-gnu/libcrypt.so.1 "$TARGET/usr/lib/libcrypt.so.2"
    ln -sf /usr/lib/x86_64-linux-gnu/libcrypt.so.1 "$TARGET/lib64/libcrypt.so.2"

    # Bind mount virtual filesystems for chroot operations
    mount -t proc proc "$TARGET/proc"
    mount -t sysfs sysfs "$TARGET/sys"
    mount --bind /dev "$TARGET/dev"
    mount --bind /dev/pts "$TARGET/dev/pts"
    mount "$ESP1" "$TARGET/boot/efi"

    # Update dynamic linker cache in target
    chroot "$TARGET" ldconfig 2>/dev/null || true

    # Pre-enable core systemd services
    mkdir -p "$TARGET/etc/systemd/system/multi-user.target.wants"
    ln -sf /etc/systemd/system/nasctl.service "$TARGET/etc/systemd/system/multi-user.target.wants/nasctl.service"
    ln -sf /etc/systemd/system/nas-banner.service "$TARGET/etc/systemd/system/multi-user.target.wants/nas-banner.service"
    chroot "$TARGET" systemctl enable nasctl.service 2>/dev/null || true
    chroot "$TARGET" systemctl enable nas-banner.service 2>/dev/null || true
    chroot "$TARGET" systemctl enable docker.service 2>/dev/null || true
    chroot "$TARGET" systemctl enable ssh.service 2>/dev/null || true
    chroot "$TARGET" systemctl enable zfs-import-cache.service 2>/dev/null || true
    chroot "$TARGET" systemctl enable zfs-mount.service 2>/dev/null || true

    # Create zpool.cache
    zpool set cachefile="$TARGET/etc/zfs/zpool.cache" rpool || true

    run_step 90 "Installing UEFI & Legacy BIOS bootloaders..."
    # 0. Generate /etc/default/grub with ZFS boot parameters and serial console
    cat << 'EOF' > "$TARGET/etc/default/grub"
# GRUB configuration for mynas appliance
GRUB_DEFAULT=0
GRUB_TIMEOUT=5
GRUB_DISTRIBUTOR="mynas"
GRUB_CMDLINE_LINUX_DEFAULT="quiet console=tty0 console=ttyS0,115200"
GRUB_CMDLINE_LINUX="root=ZFS=rpool/ROOT/debian boot=zfs"
GRUB_TERMINAL="console serial"
GRUB_SERIAL_COMMAND="serial --unit=0 --speed=115200"
EOF

    # 1. GRUB UEFI on primary EFI System Partition & Removable Fallback (/EFI/BOOT/BOOTX64.EFI)
    chroot "$TARGET" grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=debian --recheck --no-floppy 2>/dev/null || true
    mkdir -p "$TARGET/boot/efi/EFI/BOOT"
    if [ -f "$TARGET/boot/efi/EFI/debian/grubx64.efi" ]; then
        cp "$TARGET/boot/efi/EFI/debian/grubx64.efi" "$TARGET/boot/efi/EFI/BOOT/BOOTX64.EFI"
    fi

    # Optional secondary drive EFI mirror
    if [ -n "$ESP2" ]; then
        mkdir -p "$TARGET/boot/efi2"
        mount "$ESP2" "$TARGET/boot/efi2" 2>/dev/null || true
        chroot "$TARGET" grub-install --target=x86_64-efi --efi-directory=/boot/efi2 --bootloader-id=debian --recheck --no-floppy 2>/dev/null || true
        mkdir -p "$TARGET/boot/efi2/EFI/BOOT"
        if [ -f "$TARGET/boot/efi2/EFI/debian/grubx64.efi" ]; then
            cp "$TARGET/boot/efi2/EFI/debian/grubx64.efi" "$TARGET/boot/efi2/EFI/BOOT/BOOTX64.EFI"
        fi
        umount "$TARGET/boot/efi2" 2>/dev/null || true
        rmdir "$TARGET/boot/efi2" 2>/dev/null || true
    fi

    # 2. GRUB Legacy BIOS on MBR of all target drives
    for drv in "${DRIVE_ARRAY[@]}"; do
        chroot "$TARGET" grub-install --target=i386-pc --recheck "$drv"
    done

    # 3. Refresh initramfs and grub
    chroot "$TARGET" update-initramfs -u -k all
    chroot "$TARGET" update-grub

    run_step 98 "Unmounting and exporting root pool..."
    umount "$TARGET/boot/efi" 2>/dev/null || true
    umount -l "$TARGET/dev/pts" 2>/dev/null || true
    umount -l "$TARGET/dev" 2>/dev/null || true
    umount -l "$TARGET/sys" 2>/dev/null || true
    umount -l "$TARGET/proc" 2>/dev/null || true

    zpool export rpool 2>/dev/null || true

    run_step 100 "Installation completed successfully!"
    sleep 2
} | whiptail --title "$TITLE: Installing" --gauge "Beginning appliance deployment..." 8 70 0
PIPE_STATUS="${PIPESTATUS[0]}"

if [ "$PIPE_STATUS" -ne 0 ]; then
    whiptail --title "$TITLE: Installation Error" --msgbox \
"An error occurred during installation (exit code $PIPE_STATUS).
Please check /tmp/mynas-install.log for details." 10 60
    exit 1
fi

# ------------------------------------------------------------------------------
# Step 8: Completion Dialog
# ------------------------------------------------------------------------------
whiptail --title "$TITLE: Complete" --msgbox \
"Congratulations! The mynas appliance has been installed successfully.

Next Steps:
  1. Remove the installation media (USB/ISO).
  2. The system will now reboot into Debian 13 with OpenZFS.
  3. Once booted, the console will display the Web UI access URL (http://<ip>:8080).
  4. Log in via Web UI to configure bulk storage pools and file shares." 14 70

whiptail --title "$TITLE" --yesno "Reboot the system now?" 8 40
if [ $? -eq 0 ]; then
    reboot
fi
