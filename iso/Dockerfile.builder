FROM debian:trixie-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    debootstrap \
    squashfs-tools \
    xorriso \
    grub-common \
    grub-pc-bin \
    grub-efi-amd64-bin \
    mtools \
    dosfstools \
    curl \
    ca-certificates \
    rsync \
    git \
    make \
    bc \
    kmod \
    isolinux \
    syslinux-common \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

ENTRYPOINT ["/workspace/iso/build.sh"]
