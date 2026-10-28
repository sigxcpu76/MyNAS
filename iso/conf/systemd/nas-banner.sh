#!/bin/sh
# Generate dynamic appliance console greeting in /etc/issue

IP_ADDR=$(ip -4 addr show br0 2>/dev/null | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -n 1)
if [ -z "$IP_ADDR" ]; then
    # Fallback to default route interface
    DEF_IFACE=$(ip route show default 2>/dev/null | awk '/default/ {print $5}' | head -n 1)
    if [ -n "$DEF_IFACE" ]; then
        IP_ADDR=$(ip -4 addr show "$DEF_IFACE" 2>/dev/null | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -n 1)
    fi
fi

if [ -z "$IP_ADDR" ]; then
    IP_ADDR="[Acquiring IP / No Link]"
fi

cat <<EOF > /etc/issue

==============================================================
                         mynas Appliance
==============================================================
  Web UI:        http://${IP_ADDR}:8080 (via br0)
  API / gRPC:    https://${IP_ADDR}:9090
  WebDAV:        http://${IP_ADDR}:8081
  SSH:           ssh root@${IP_ADDR}
  ZFS Root:      rpool (OpenZFS on Proxmox 7.0 Kernel)
==============================================================

\n \l
EOF
