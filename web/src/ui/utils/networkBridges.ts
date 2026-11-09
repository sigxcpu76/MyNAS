import { computed } from 'vue';
import type { NetworkInterfaceTelemetry } from '../../domain/models';
import { nasState } from '../store';

export interface BridgeOption {
  name: string;
  label: string;
}

function isDockerOrEphemeralInterface(name: string): boolean {
  return (
    name === 'lo' ||
    name.startsWith('lo') ||
    name.startsWith('docker') ||
    /^br-[0-9a-f]{6,}/i.test(name) ||
    name.startsWith('veth') ||
    name.startsWith('vnet') ||
    name.startsWith('tap') ||
    name.startsWith('tun') ||
    name.startsWith('tailscale') ||
    name.startsWith('wg')
  );
}

function isSystemBridgeInterface(n: NetworkInterfaceTelemetry): boolean {
  if (isDockerOrEphemeralInterface(n.name)) return false;
  if (n.isBridge) return true;
  return /^(br\d+|vmbr\d+|virbr\d+|lxcbr\d+|ovsbr\d+|bond\d+)/i.test(n.name);
}

function formatBridgeLabel(n: NetworkInterfaceTelemetry): string {
  const ip = n.ipAddresses?.find((a) => !a.includes(':')) || n.ipAddresses?.[0];
  const parts: string[] = [n.isUp ? 'up' : 'down'];
  if (ip) parts.push(ip);
  if (n.isDefault) parts.push('default route');
  return `${n.name} (${parts.join(' · ')})`;
}

export function useNetworkBridges() {
  const hostBridgeInterfaces = computed<BridgeOption[]>(() => {
    const net = nasState.systemTelemetry?.network;
    if (Array.isArray(net) && net.length > 0) {
      const bridges = net.filter(isSystemBridgeInterface);
      const candidates =
        bridges.length > 0
          ? bridges
          : net.filter((n) => !isDockerOrEphemeralInterface(n.name));

      const sorted = [...candidates].sort((a, b) => {
        if (Boolean(a.isDefault) !== Boolean(b.isDefault)) return a.isDefault ? -1 : 1;
        if (Boolean(a.isUp) !== Boolean(b.isUp)) return a.isUp ? -1 : 1;
        return a.name.localeCompare(b.name);
      });

      if (sorted.length > 0) {
        return sorted.map((n) => ({
          name: n.name,
          label: formatBridgeLabel(n),
        }));
      }
    }

    // Fallback: gather bridges referenced by existing VMs/LXC containers, or br0
    const known = new Set<string>();
    for (const c of nasState.lxcContainers || []) {
      if (c.networkBridge) known.add(c.networkBridge);
    }
    for (const v of nasState.vms || []) {
      if (v.networkBridge) known.add(v.networkBridge);
      for (const nic of v.networkInterfaces || []) {
        if (nic.bridge) known.add(nic.bridge);
      }
    }
    if (known.size === 0) {
      known.add('br0');
    }
    return Array.from(known).map((name) => ({ name, label: name }));
  });

  const defaultBridge = computed<string>(() => {
    const opts = hostBridgeInterfaces.value;
    if (opts.length > 0) {
      const br0 = opts.find((o) => o.name === 'br0' || o.name === 'vmbr0');
      return br0 ? br0.name : opts[0].name;
    }
    return 'br0';
  });

  function bridgeOptionsFor(current?: string): BridgeOption[] {
    const opts = [...hostBridgeInterfaces.value];
    const trimmed = (current || '').trim();
    if (trimmed && !opts.some((o) => o.name === trimmed)) {
      opts.unshift({ name: trimmed, label: `${trimmed} (current)` });
    }
    return opts;
  }

  return {
    hostBridgeInterfaces,
    defaultBridge,
    bridgeOptionsFor,
  };
}
