import YAML from 'yaml';

// =============================================================================
// Docker Compose Structured Form Entities
// =============================================================================

export interface ComposePortModel {
  hostIp?: string;
  hostPort: string;
  containerPort: string;
  protocol: 'tcp' | 'udp';
  publishToHost?: boolean;
  isManagementPort?: boolean;
}

export interface ComposeEnvModel {
  key: string;
  value: string;
}

export interface ComposeVolumeMountModel {
  type?: 'bind' | 'volume' | 'config';
  source: string; // Host path (e.g. /tank/data), named volume, or config name
  target: string; // Container path (e.g. /var/www/html)
  mode: 'rw' | 'ro';
  uid?: string;
  gid?: string;
}

export interface ComposeServiceModel {
  id: string; // Client UI row id
  name: string; // Service key in compose (e.g. "web", "db", "redis")
  containerName: string;
  hostname?: string;
  image: string;
  restart: 'unless-stopped' | 'always' | 'on-failure' | 'no' | string;
  command: string;
  ports: ComposePortModel[];
  environment: ComposeEnvModel[];
  volumes: ComposeVolumeMountModel[];
  dependsOn: string[];
  networkMode: string; // 'bridge' | 'host' | 'none' | custom
  networks: string[];
  memoryLimit: string; // e.g. "512m", "1g", "2g"
  cpuLimit: string; // e.g. "0.5", "1.0", "2.0"
  privileged: boolean;
  devices: string[];
  gpuEnabled: boolean;
  gpuCount: string; // 'all' | '1' | '2' | 'custom'
  gpuDeviceIds: string; // e.g. "0" or "0,1"
  rawRuntime?: string;
  rawCommand?: string[];
  rawDependsOn?: Record<string, any>;
  rawNetworks?: Record<string, any>;
  customFields?: Record<string, any>;
}

export interface ComposeTopLevelNetwork {
  id: string;
  name: string;
  driver?: 'bridge' | 'overlay' | 'macvlan' | string;
  internal?: boolean;
  external?: boolean;
  rawConfig?: Record<string, any>;
}

export interface ComposeTopLevelVolume {
  id: string;
  name: string;
  driver?: string;
  external?: boolean;
  rawConfig?: Record<string, any>;
}

export interface ComposeTopLevelConfig {
  id: string;
  name: string;
  content: string;
  file?: string;
  external?: boolean;
  rawConfig?: Record<string, any>;
}

export interface ComposePortalModel {
  name: string;
  scheme: string;
  port: string;
  path: string;
}

export interface ComposeAppFormModel {
  appId: string;
  title: string;
  autoStart: boolean;
  services: ComposeServiceModel[];
  networks: ComposeTopLevelNetwork[];
  volumes: ComposeTopLevelVolume[];
  configs: ComposeTopLevelConfig[];
  managementPort?: string; // Container internal port used for Open Web UI (e.g. "81")
  portals?: ComposePortalModel[];
  catalogId?: string;
  catalogSource?: string;
  catalogVersion?: string;
  customTopLevelFields?: Record<string, any>;
}

// =============================================================================
// Factory Helpers
// =============================================================================

let nextUid = 1;
export function generateUid(): string {
  return `uid_${Date.now()}_${nextUid++}`;
}

/**
 * Generates a random high host port in 32000-65000 range.
 */
export function generateRandomHighPort(usedPorts?: Set<number>): number {
  const min = 32000;
  const max = 65000;
  for (let attempt = 0; attempt < 100; attempt++) {
    const candidate = Math.floor(Math.random() * (max - min + 1)) + min;
    if (!usedPorts || !usedPorts.has(candidate)) {
      if (usedPorts) usedPorts.add(candidate);
      return candidate;
    }
  }
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

export const DEFAULT_COMPOSE_MEMORY_LIMIT = '512m';
export const DEFAULT_COMPOSE_CPU_LIMIT = '1.0';

export const COMPOSE_MEMORY_LIMIT_OPTIONS = [
  '128m',
  '256m',
  '512m',
  '1g',
  '2g',
  '4g',
  '8g',
  '16g',
  '32g',
] as const;

export const COMPOSE_CPU_LIMIT_OPTIONS = [
  '0.5',
  '1.0',
  '2',
  '4',
  '8',
  '16',
  '32',
] as const;

export function sanitizeDockerHostname(raw: string): string {
  if (!raw) return '';
  return raw
    .trim()
    .toLowerCase()
    .replace(/[_. ]+/g, '-')
    .replace(/[^a-z0-9-]/g, '')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 63)
    .replace(/-+$/g, '');
}

export function resolveDefaultServiceHostname(
  appId: string,
  serviceName: string,
  containerName?: string,
  totalServices = 1
): string {
  if (containerName && !containerName.includes('$')) {
    const h = sanitizeDockerHostname(containerName);
    if (h) return h;
  }
  const cleanApp = sanitizeDockerHostname(appId);
  const cleanSvc = sanitizeDockerHostname(serviceName);
  if (totalServices <= 1) {
    if (cleanApp && cleanApp !== 'app') return cleanApp;
    if (cleanSvc) return cleanSvc;
    return cleanApp;
  }
  if (cleanApp && cleanApp !== 'app') {
    if (!cleanSvc || cleanSvc === cleanApp || cleanSvc.startsWith(`${cleanApp}-`)) {
      return cleanSvc || cleanApp;
    }
    return sanitizeDockerHostname(`${cleanApp}-${cleanSvc}`);
  }
  return cleanSvc;
}

export function createDefaultService(name = 'app', image = ''): ComposeServiceModel {
  return {
    id: generateUid(),
    name,
    containerName: '',
    hostname: '',
    image,
    restart: 'unless-stopped',
    command: '',
    ports: [],
    environment: [],
    volumes: [],
    dependsOn: [],
    networkMode: '',
    networks: [],
    memoryLimit: DEFAULT_COMPOSE_MEMORY_LIMIT,
    cpuLimit: DEFAULT_COMPOSE_CPU_LIMIT,
    privileged: false,
    devices: [],
    gpuEnabled: false,
    gpuCount: 'all',
    gpuDeviceIds: '',
  };
}

export function createDefaultAppForm(appId = '', title = ''): ComposeAppFormModel {
  return {
    appId,
    title,
    autoStart: true,
    services: [createDefaultService('app')],
    networks: [],
    volumes: [],
    configs: [],
    managementPort: '',
    portals: [],
  };
}

// =============================================================================
// Helper Functions for Dollar ($) Escaping in Compose Environment Variables
// =============================================================================

/**
 * Escapes literal dollar signs ($) in an environment variable value to ($$)
 * so Docker Compose passes them verbatim to the container instead of attempting
 * variable interpolation. Valid compose interpolations like ${VAR} or ${VAR:-default}
 * are preserved as is.
 */
export function escapeComposeEnvValue(val: string): string {
  if (!val || typeof val !== 'string') return val ?? '';
  let out = '';
  for (let i = 0; i < val.length; i++) {
    if (val[i] === '$') {
      // Already escaped as $$? Keep as $$
      if (i + 1 < val.length && val[i + 1] === '$') {
        out += '$$';
        i++;
        continue;
      }
      // Valid braced interpolation like ${VAR} or ${VAR:-default}? Preserve as is
      if (i + 1 < val.length && val[i + 1] === '{') {
        const closeIdx = val.indexOf('}', i + 2);
        if (closeIdx !== -1) {
          const inner = val.substring(i + 2, closeIdx);
          if (/^[a-zA-Z_][a-zA-Z0-9_]*(:-.*)?$/.test(inner)) {
            out += val.substring(i, closeIdx + 1);
            i = closeIdx;
            continue;
          }
        }
      }
      // Bare literal $: escape to $$
      out += '$$';
      continue;
    }
    out += val[i];
  }
  return out;
}

/**
 * Unescapes ($$) in a compose environment value back to ($) for human display
 * and editing in visual forms.
 */
export function unescapeComposeEnvValue(val: string): string {
  if (!val || typeof val !== 'string') return val ?? '';
  return val.replace(/\$\$/g, '$');
}

/**
 * Detects whether a volume entry is actually an internal TrueNAS setting / descriptor
 * mistakenly serialized as a volume mapping (e.g. ix_volume:/type or false:/acl_enable).
 */
export function isTrueNASSettingVolume(source: string, target: string): boolean {
  if (source === 'true' || source === 'false') {
    return true;
  }
  const cleanTarget = target.replace(/^\/+/, '').toLowerCase();
  switch (cleanTarget) {
    case 'type':
    case 'acl_enable':
    case 'dataset_name':
    case 'acl_entries':
    case 'cifs_config':
    case 'nfs_config':
    case 'read_only':
    case 'host_path_config':
    case 'ix_volume_config':
      return true;
    default:
      return false;
  }
}

// =============================================================================
// YAML <-> Form Bi-directional Converters
// =============================================================================

/**
 * Parses raw compose YAML into the structured form model.
 */
export function composeYamlToForm(
  yamlStr: string,
  fallbackAppId = '',
  fallbackTitle = '',
  poolName = 'tank'
): ComposeAppFormModel {
  const model: ComposeAppFormModel = {
    appId: fallbackAppId,
    title: fallbackTitle,
    autoStart: true,
    services: [],
    networks: [],
    volumes: [],
    configs: [],
  };

  if (!yamlStr || !yamlStr.trim()) {
    model.services.push(createDefaultService());
    return model;
  }

  let parsed: any;
  try {
    parsed = YAML.parse(yamlStr);
  } catch (err: any) {
    throw new Error(`YAML syntax error: ${err?.message || err}`);
  }

  if (!parsed || typeof parsed !== 'object') {
    model.services.push(createDefaultService());
    return model;
  }

  // Extract management port if specified in extension fields or comments
  let mgmtPort = '';
  if (parsed['x-nas-management-port'] !== undefined && parsed['x-nas-management-port'] !== null) {
    mgmtPort = String(parsed['x-nas-management-port']).trim();
  } else if (parsed['x-management-port'] !== undefined && parsed['x-management-port'] !== null) {
    mgmtPort = String(parsed['x-management-port']).trim();
  } else if (parsed['x-webui-port'] !== undefined && parsed['x-webui-port'] !== null) {
    mgmtPort = String(parsed['x-webui-port']).trim();
  }

  // 1. Parse Services
  const rawServices = parsed.services || {};
  for (const [svcName, rawSvc] of Object.entries(rawServices)) {
    if (!rawSvc || typeof rawSvc !== 'object') continue;
    const svc = rawSvc as any;

    if (!mgmtPort && svc.labels) {
      if (Array.isArray(svc.labels)) {
        for (const l of svc.labels) {
          const lStr = String(l);
          if (lStr.startsWith('nas.management.port=') || lStr.startsWith('nas.webui.port=')) {
            mgmtPort = lStr.split('=')[1]?.trim() || '';
          }
        }
      } else if (typeof svc.labels === 'object') {
        mgmtPort = String(svc.labels['nas.management.port'] || svc.labels['nas.webui.port'] || '').trim();
      }
    }

    // Parse GPU reservations
    let gpuEnabled = false;
    let gpuCount = 'all';
    let gpuDeviceIds = '';

    const resDevices = svc.deploy?.resources?.reservations?.devices;
    if (Array.isArray(resDevices)) {
      for (const dev of resDevices) {
        if (dev && (dev.driver === 'nvidia' || (Array.isArray(dev.capabilities) && dev.capabilities.includes('gpu')))) {
          gpuEnabled = true;
          if (Array.isArray(dev.device_ids) && dev.device_ids.length > 0) {
            gpuDeviceIds = dev.device_ids.map(String).join(', ');
            gpuCount = 'custom';
          } else if (dev.count !== undefined && dev.count !== null) {
            gpuCount = String(dev.count).toLowerCase();
          }
          break;
        }
      }
    } else if (svc.runtime === 'nvidia') {
      gpuEnabled = true;
    }

    const serviceModel: ComposeServiceModel = {
      id: generateUid(),
      name: svcName,
      containerName: svc.container_name || '',
      hostname: svc.hostname ? String(svc.hostname).trim() : '',
      image: svc.image || '',
      restart: svc.restart || 'unless-stopped',
      command: Array.isArray(svc.command)
        ? svc.command.join(' ')
        : typeof svc.command === 'string'
        ? svc.command
        : '',
      rawCommand: Array.isArray(svc.command) ? svc.command.map(String) : undefined,
      ports: [],
      environment: [],
      volumes: [],
      dependsOn: [],
      networkMode: svc.network_mode || '',
      networks: [],
      memoryLimit: svc.mem_limit
        ? String(svc.mem_limit).trim()
        : svc.deploy?.resources?.limits?.memory
        ? String(svc.deploy.resources.limits.memory).trim()
        : '',
      cpuLimit: svc.cpus !== undefined && svc.cpus !== null && svc.cpus !== ''
        ? (Number(svc.cpus) === 1 ? '1.0' : String(svc.cpus).trim())
        : svc.deploy?.resources?.limits?.cpus !== undefined && svc.deploy?.resources?.limits?.cpus !== null && svc.deploy?.resources?.limits?.cpus !== ''
        ? (Number(svc.deploy.resources.limits.cpus) === 1 ? '1.0' : String(svc.deploy.resources.limits.cpus).trim())
        : '',
      privileged: Boolean(svc.privileged ?? false),
      devices: Array.isArray(svc.devices) ? svc.devices.map(String) : [],
      gpuEnabled,
      gpuCount,
      gpuDeviceIds,
      rawRuntime: typeof svc.runtime === 'string' && svc.runtime.trim() ? svc.runtime.trim() : undefined,
    };

    // Parse Ports
    if (Array.isArray(svc.ports)) {
      for (const p of svc.ports) {
        if (typeof p === 'string' || typeof p === 'number') {
          const str = String(p).trim().replace(/^["']+|["']+$/g, '');
          const isSinglePort = !str.includes(':') || (str.startsWith('[') && str.endsWith(']'));
          if (!isSinglePort) {
            const lastColon = str.lastIndexOf(':');
            const prefix = str.slice(0, lastColon).trim();
            const containerPart = str.slice(lastColon + 1).trim();
            let hostIp: string | undefined = undefined;
            let hostP = '';

            if (prefix.startsWith('[')) {
              if (prefix.endsWith(']')) {
                hostIp = prefix;
                hostP = '';
              } else {
                const closeBracket = prefix.indexOf(']:');
                if (closeBracket !== -1) {
                  hostIp = prefix.slice(0, closeBracket + 1).trim();
                  hostP = prefix.slice(closeBracket + 2).trim();
                } else {
                  hostP = prefix;
                }
              }
            } else {
              const prevColon = prefix.lastIndexOf(':');
              if (prevColon === -1) {
                hostP = prefix;
              } else {
                const rawIp = prefix.slice(0, prevColon).replace(/^["']+|["']+$/g, '').trim();
                hostP = prefix.slice(prevColon + 1).replace(/^["']+|["']+$/g, '').trim();
                if (rawIp) {
                  hostIp = rawIp.includes(':') ? `[${rawIp}]` : rawIp;
                }
              }
            }

            const [contP, proto] = containerPart.split('/');
            let cleanHostP = hostP.replace(/^["']+|["']+$/g, '').trim();
            if (!cleanHostP) {
              cleanHostP = String(generateRandomHighPort());
            }
            const cleanContP = (contP || '').replace(/^["']+|["']+$/g, '').trim();
            serviceModel.ports.push({
              hostIp: hostIp || undefined,
              hostPort: cleanHostP,
              containerPort: cleanContP,
              protocol: (proto?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
              publishToHost: true,
            });
          } else {
            // Port specified under `ports:` without host mapping (e.g. - "80")
            // Assign a deterministic high host port in 32000-65000 range based on container port
            const [contP, proto] = str.split('/');
            const cleanContP = (contP || '').replace(/^["']+|["']+$/g, '').trim();
            const contNum = parseInt(cleanContP, 10);
            const detHost =
              !isNaN(contNum) && contNum > 0
                ? contNum >= 32000 && contNum <= 65000
                  ? contNum
                  : 32000 + (contNum % 33000)
                : 32000;
            serviceModel.ports.push({
              hostPort: String(detHost),
              containerPort: cleanContP,
              protocol: (proto?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
              publishToHost: true,
            });
          }
        } else if (p && typeof p === 'object') {
          const hasHost = p.published !== undefined && p.published !== null && String(p.published).trim() !== '';
          const contP = String(p.target || p.container || '').replace(/^["']+|["']+$/g, '').trim();
          const contNum = parseInt(contP, 10);
          const detHost =
            !isNaN(contNum) && contNum > 0
              ? contNum >= 32000 && contNum <= 65000
                ? contNum
                : 32000 + (contNum % 33000)
              : 32000;
          const hostP = hasHost ? String(p.published).replace(/^["']+|["']+$/g, '').trim() : String(detHost);
          const hostIp = p.host_ip ? String(p.host_ip).trim() : (p.ip ? String(p.ip).trim() : undefined);
          serviceModel.ports.push({
            hostIp: hostIp || undefined,
            hostPort: hostP,
            containerPort: contP,
            protocol: (p.protocol?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
            publishToHost: hasHost,
          });
        }
      }
    }

    // Parse Expose (Internal-only container ports not published to host)
    if (Array.isArray(svc.expose)) {
      for (const exp of svc.expose) {
        if (typeof exp === 'string' || typeof exp === 'number') {
          const str = String(exp).trim();
          const [contP, proto] = str.split('/');
          const exists = serviceModel.ports.some(
            (existing) => existing.containerPort === contP.trim() && !existing.publishToHost
          );
          if (!exists) {
            serviceModel.ports.push({
              hostPort: '',
              containerPort: contP.trim(),
              protocol: (proto?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
              publishToHost: false,
            });
          }
        }
      }
    }

    // Parse Environment
    if (Array.isArray(svc.environment)) {
      for (const envItem of svc.environment) {
        if (typeof envItem === 'string') {
          const idx = envItem.indexOf('=');
          if (idx !== -1) {
            serviceModel.environment.push({
              key: envItem.substring(0, idx).trim(),
              value: unescapeComposeEnvValue(envItem.substring(idx + 1)),
            });
          } else {
            serviceModel.environment.push({
              key: envItem.trim(),
              value: '',
            });
          }
        }
      }
    } else if (svc.environment && typeof svc.environment === 'object') {
      for (const [k, v] of Object.entries(svc.environment)) {
        serviceModel.environment.push({
          key: k,
          value: v === null || v === undefined ? '' : unescapeComposeEnvValue(String(v)),
        });
      }
    }

    // Parse Volumes
    if (Array.isArray(svc.volumes)) {
      for (const v of svc.volumes) {
        if (typeof v === 'string') {
          const cleanV = v.trim().replace(/^["']+|["']+$/g, '').trim();
          const parts = cleanV.split(':');
          if (parts.length >= 2) {
            let source = parts[0].replace(/^["']+|["']+$/g, '').trim();
            let target = parts[1].replace(/^["']+|["']+$/g, '').trim();
            const mode = (parts[2]?.replace(/^["']+|["']+$/g, '').trim().toLowerCase() === 'ro' ? 'ro' : 'rw') as 'rw' | 'ro';

            // Filter out TrueNAS settings mistakenly serialized as volume lines
            if (isTrueNASSettingVolume(source, target)) {
              continue;
            }

            // Rewrite TrueNAS ix_volume reference
            if (source === 'ix_volume' || source === 'ix-volume' || source.startsWith('ix_volume')) {
              const leaf = target ? target.replace(/^\/+/, '').split('/').pop() || 'data' : 'data';
              const cleanPool = (poolName || 'tank').replace(/^zfsPools\//, '').replace(/^\/mnt\//, '').trim() || 'tank';
              const appSlug = (fallbackAppId || parsed.name || serviceModel.name || 'app').trim();
              source = `/mnt/${cleanPool}/apps/${appSlug}/${leaf}`;
            }

            // Fix /path target
            if (target === '/path' && source.endsWith('/path')) {
              target = serviceModel.name.includes('elastic') ? '/usr/share/elasticsearch/data' : '/data';
              source = source.replace(/\/path$/, '/data');
            }

            serviceModel.volumes.push({ source, target, mode });
          } else {
            serviceModel.volumes.push({ source: '', target: cleanV, mode: 'rw' });
          }
        } else if (v && typeof v === 'object') {
          const source = String(v.source || '').replace(/^["']+|["']+$/g, '').trim();
          const target = String(v.target || '').replace(/^["']+|["']+$/g, '').trim();
          if (isTrueNASSettingVolume(source, target)) {
            continue;
          }
          serviceModel.volumes.push({
            source,
            target,
            mode: v.read_only ? 'ro' : 'rw',
          });
        }
      }
    }

    // Parse Configs (injected files / ConfigMaps)
    if (Array.isArray(svc.configs)) {
      for (const c of svc.configs) {
        if (typeof c === 'string') {
          const src = c.trim();
          serviceModel.volumes.push({
            type: 'config',
            source: src,
            target: `/${src}`,
            mode: 'ro',
          });
        } else if (c && typeof c === 'object') {
          const src = String(c.source || '').trim();
          const tgt = String(c.target || (src ? `/${src}` : '')).trim();
          serviceModel.volumes.push({
            type: 'config',
            source: src,
            target: tgt,
            mode: 'ro',
            uid: c.uid !== undefined ? String(c.uid) : undefined,
            gid: c.gid !== undefined ? String(c.gid) : undefined,
          });
        }
      }
    }

    // Parse depends_on
    if (Array.isArray(svc.depends_on)) {
      serviceModel.dependsOn = svc.depends_on.map(String);
    } else if (svc.depends_on && typeof svc.depends_on === 'object') {
      serviceModel.dependsOn = Object.keys(svc.depends_on);
      serviceModel.rawDependsOn = JSON.parse(JSON.stringify(svc.depends_on));
    }

    // Parse networks inside service
    if (Array.isArray(svc.networks)) {
      serviceModel.networks = svc.networks.map(String);
    } else if (svc.networks && typeof svc.networks === 'object') {
      serviceModel.networks = Object.keys(svc.networks);
      serviceModel.rawNetworks = JSON.parse(JSON.stringify(svc.networks));
    }

    // Preserve custom or unrecognized service attributes (e.g. labels, healthcheck, cap_add, user, shm_size, deploy, etc.)
    const knownKeys = new Set([
      'container_name', 'hostname', 'image', 'restart', 'command', 'ports', 'expose',
      'environment', 'volumes', 'configs', 'depends_on', 'network_mode', 'networks',
      'mem_limit', 'cpus', 'privileged', 'devices', 'runtime', 'labels'
    ]);
    const customFields: Record<string, any> = {};
    for (const [k, v] of Object.entries(svc)) {
      if (!knownKeys.has(k)) {
        customFields[k] = k === 'deploy' && v && typeof v === 'object' ? JSON.parse(JSON.stringify(v)) : v;
      }
    }
    // Also preserve non-management labels
    if (svc.labels) {
      if (Array.isArray(svc.labels)) {
        const filtered = svc.labels.filter((l: any) => {
          const s = String(l);
          return !s.startsWith('nas.management.port=') && !s.startsWith('nas.webui.port=');
        });
        if (filtered.length > 0) {
          customFields['labels'] = filtered;
        }
      } else if (typeof svc.labels === 'object') {
        const copy = { ...(svc.labels as Record<string, any>) };
        delete copy['nas.management.port'];
        delete copy['nas.webui.port'];
        if (Object.keys(copy).length > 0) {
          customFields['labels'] = copy;
        }
      }
    }
    if (Object.keys(customFields).length > 0) {
      serviceModel.customFields = customFields;
    }

    model.services.push(serviceModel);
  }

  if (model.services.length === 0) {
    model.services.push(createDefaultService());
  }

  // 2. Parse Top-Level Networks
  if (parsed.networks && typeof parsed.networks === 'object') {
    for (const [netName, netConfig] of Object.entries(parsed.networks)) {
      const net = (netConfig && typeof netConfig === 'object') ? (netConfig as any) : {};
      const isExternal = Boolean(
        net.external === true ||
        (net.external && typeof net.external === 'object')
      );

      // Preserve any custom or unrecognized network attributes (e.g. driver_opts, labels, attachable, name, ipam)
      const rawConfig: Record<string, any> = {};
      if (netConfig && typeof netConfig === 'object') {
        for (const [k, v] of Object.entries(netConfig)) {
          if (!['driver', 'internal', 'external'].includes(k)) {
            rawConfig[k] = v;
          }
        }
      }

      model.networks.push({
        id: generateUid(),
        name: netName,
        driver: net.driver || (isExternal ? '' : 'bridge'),
        internal: Boolean(net.internal ?? false),
        external: isExternal,
        rawConfig: Object.keys(rawConfig).length > 0 ? rawConfig : undefined,
      });
    }
  }

  // 3. Parse Top-Level Volumes
  if (parsed.volumes && typeof parsed.volumes === 'object') {
    for (const [volName, volConfig] of Object.entries(parsed.volumes)) {
      const vol = (volConfig && typeof volConfig === 'object') ? (volConfig as any) : {};
      const isExternal = Boolean(
        vol.external === true ||
        (vol.external && typeof vol.external === 'object')
      );

      // Preserve any custom volume attributes (e.g. driver_opts, labels, name)
      const rawConfig: Record<string, any> = {};
      if (volConfig && typeof volConfig === 'object') {
        for (const [k, v] of Object.entries(volConfig)) {
          if (!['driver', 'external'].includes(k)) {
            rawConfig[k] = v;
          }
        }
      }

      model.volumes.push({
        id: generateUid(),
        name: volName,
        driver: vol.driver || (isExternal ? '' : 'local'),
        external: isExternal,
        rawConfig: Object.keys(rawConfig).length > 0 ? rawConfig : undefined,
      });
    }
  }

  // 4. Parse Top-Level Configs (inline files / ConfigMaps)
  if (parsed.configs && typeof parsed.configs === 'object') {
    for (const [cfgName, cfgConfig] of Object.entries(parsed.configs)) {
      let content = '';
      let file: string | undefined = undefined;
      let external = false;
      const rawConfig: Record<string, any> = {};

      if (typeof cfgConfig === 'string') {
        content = cfgConfig;
      } else if (cfgConfig && typeof cfgConfig === 'object') {
        const cObj = cfgConfig as any;
        if (typeof cObj.content === 'string') {
          content = cObj.content;
        }
        if (typeof cObj.file === 'string') {
          file = cObj.file;
        }
        external = Boolean(cObj.external === true || (cObj.external && typeof cObj.external === 'object'));

        for (const [k, v] of Object.entries(cObj)) {
          if (!['content', 'file', 'external'].includes(k)) {
            rawConfig[k] = v;
          }
        }
      }

      model.configs.push({
        id: generateUid(),
        name: cfgName,
        content,
        file,
        external,
        rawConfig: Object.keys(rawConfig).length > 0 ? rawConfig : undefined,
      });
    }
  }

  // Ensure any config referenced by services is present in model.configs
  for (const s of model.services) {
    for (const v of s.volumes) {
      if (v.type === 'config' && v.source.trim()) {
        const src = v.source.trim();
        if (!model.configs.some((c) => c.name === src)) {
          model.configs.push({
            id: generateUid(),
            name: src,
            content: '',
          });
        }
      }
    }
  }

  // 5. Parse Portals (TrueNAS SCALE x-portals or standard portals)
  const portals: ComposePortalModel[] = [];
  const rawPortals = parsed['x-portals'] || parsed['portals'];
  if (Array.isArray(rawPortals)) {
    for (const p of rawPortals) {
      if (p && typeof p === 'object') {
        portals.push({
          name: String(p.name || 'Web UI'),
          scheme: String(p.scheme || p.protocol || 'http'),
          port: String(p.port ?? ''),
          path: String(p.path || '/'),
        });
      }
    }
  } else if (rawPortals && typeof rawPortals === 'object') {
    for (const [key, p] of Object.entries(rawPortals)) {
      if (p && typeof p === 'object') {
        const pObj = p as any;
        portals.push({
          name: String(pObj.name || key),
          scheme: String(pObj.scheme || pObj.protocol || 'http'),
          port: String(pObj.port ?? ''),
          path: String(pObj.path || '/'),
        });
      }
    }
  }
  model.portals = portals;

  // Fallback: check portals or YAML comments for management port definition
  if (!mgmtPort) {
    if (portals.length > 0 && portals[0].port) {
      mgmtPort = portals[0].port;
    } else {
      const commentMatch = yamlStr.match(/#\s*(?:x-nas-management-port|nas\.management\.port|webui-port)[:=]\s*(\d+)/i);
      if (commentMatch) {
        mgmtPort = commentMatch[1];
      }
    }
  }

  model.managementPort = mgmtPort;
  if (parsed['x-nas-catalog-id']) {
    model.catalogId = String(parsed['x-nas-catalog-id']).trim();
  }
  if (parsed['x-nas-catalog-source']) {
    model.catalogSource = String(parsed['x-nas-catalog-source']).trim();
  }
  if (parsed['x-nas-catalog-version']) {
    model.catalogVersion = String(parsed['x-nas-catalog-version']).trim();
  }
  if (mgmtPort) {
    for (const s of model.services) {
      for (const p of s.ports) {
        if (p.containerPort === mgmtPort) {
          p.isManagementPort = true;
        }
      }
    }
  }

  // Preserve unrecognized top-level keys (secrets, name, x-* extensions, etc.)
  const knownTopKeys = new Set([
    'services',
    'networks',
    'volumes',
    'configs',
    'x-portals',
    'portals',
    'x-nas-management-port',
    'x-management-port',
    'x-webui-port',
    'x-nas-catalog-id',
    'x-nas-catalog-source',
    'x-nas-catalog-version',
  ]);
  const customTopLevelFields: Record<string, any> = {};
  for (const [k, v] of Object.entries(parsed)) {
    if (!knownTopKeys.has(k)) {
      customTopLevelFields[k] = v && typeof v === 'object' ? JSON.parse(JSON.stringify(v)) : v;
    }
  }
  if (Object.keys(customTopLevelFields).length > 0) {
    model.customTopLevelFields = customTopLevelFields;
  }

  // 5. Determine autoStart status across services
  const hasRestart = model.services.some(
    (s) => s.restart === 'always' || s.restart === 'unless-stopped'
  );
  const allNoRestart = model.services.length > 0 && model.services.every((s) => s.restart === 'no');
  model.autoStart = hasRestart || !allNoRestart;

  return model;
}

/**
 * Converts structured form model to standard Docker Compose YAML specification.
 */
export function formToComposeYaml(form: ComposeAppFormModel): string {
  const rootObj: any = {};

  if (form.customTopLevelFields?.name !== undefined) {
    rootObj.name = form.customTopLevelFields.name;
  }

  if (form.catalogId) {
    rootObj['x-nas-catalog-id'] = form.catalogId;
  }
  if (form.catalogSource) {
    rootObj['x-nas-catalog-source'] = form.catalogSource;
  }
  if (form.catalogVersion) {
    rootObj['x-nas-catalog-version'] = form.catalogVersion;
  }

  const mgmtPort = form.managementPort?.trim() || '';
  if (mgmtPort) {
    const numPort = Number(mgmtPort);
    rootObj['x-nas-management-port'] = Number.isNaN(numPort) ? mgmtPort : numPort;
  }

  if (form.portals && form.portals.length > 0) {
    rootObj['x-portals'] = form.portals.map((p, idx) => {
      const portVal = idx === 0 && mgmtPort ? mgmtPort : p.port;
      const pNum = Number(portVal);
      return {
        name: p.name || 'Web UI',
        scheme: p.scheme || 'http',
        port: Number.isNaN(pNum) ? portVal : pNum,
        path: p.path || '/',
      };
    });
  } else if (mgmtPort) {
    const pNum = Number(mgmtPort);
    rootObj['x-portals'] = [
      {
        name: 'Web UI',
        scheme: 'http',
        port: Number.isNaN(pNum) ? mgmtPort : pNum,
        path: '/',
      },
    ];
  }

  if (form.customTopLevelFields) {
    for (const [k, v] of Object.entries(form.customTopLevelFields)) {
      if (k.startsWith('x-') && !(k in rootObj)) {
        rootObj[k] = v;
      }
    }
  }

  rootObj.services = {};
  const allServicesRestartNo =
    form.services.length > 0 && form.services.every((s) => s.restart === 'no');

  for (const svc of form.services) {
    const svcKey = svc.name.trim() || 'app';
    const svcObj: any = {};

    if (svc.containerName.trim()) {
      svcObj.container_name = svc.containerName.trim();
    }

    const netMode = (svc.networkMode || '').trim();
    const sharesContainerNet = netMode.startsWith('service:') || netMode.startsWith('container:');
    if (!sharesContainerNet) {
      const explicitHost = (svc.hostname || '').trim();
      const resolvedHost =
        explicitHost ||
        resolveDefaultServiceHostname(form.appId, svcKey, svc.containerName, form.services.length);
      if (resolvedHost) {
        svcObj.hostname = resolvedHost;
      }
    }

    if (svc.image.trim()) {
      svcObj.image = svc.image.trim();
    }

    if (form.autoStart === false) {
      svcObj.restart = 'no';
    } else if (form.autoStart === true && allServicesRestartNo) {
      svcObj.restart = 'unless-stopped';
    } else if (svc.restart) {
      svcObj.restart = svc.restart;
    } else {
      svcObj.restart = 'unless-stopped';
    }

    if (svc.command.trim()) {
      if (
        svc.rawCommand &&
        Array.isArray(svc.rawCommand) &&
        svc.rawCommand.join(' ').trim() === svc.command.trim()
      ) {
        svcObj.command = [...svc.rawCommand];
      } else {
        svcObj.command = svc.command.trim();
      }
    }

    // Ports & Expose
    const activePorts = svc.ports.filter((p) => p.containerPort.trim());
    const publishedPorts = activePorts.filter((p) => p.publishToHost !== false);
    const internalPorts = activePorts.filter((p) => p.publishToHost === false);

    if (publishedPorts.length > 0) {
      svcObj.ports = publishedPorts.map((p) => {
        let hostP = p.hostPort.replace(/^["']+|["']+$/g, '').trim();
        if (!hostP) {
          hostP = String(generateRandomHighPort());
          p.hostPort = hostP;
        }
        const cleanContP = p.containerPort.replace(/^["']+|["']+$/g, '').trim();
        const protoSuffix = p.protocol === 'udp' ? '/udp' : '';
        const hostIpPrefix = p.hostIp ? `${p.hostIp}:` : '';
        const portStr = `${hostIpPrefix}${hostP}:${cleanContP}${protoSuffix}`;
        const s = new YAML.Scalar(portStr);
        s.type = 'QUOTE_DOUBLE';
        return s;
      });
    }

    if (internalPorts.length > 0) {
      svcObj.expose = internalPorts.map((p) => {
        const protoSuffix = p.protocol === 'udp' ? '/udp' : '';
        const cleanContP = p.containerPort.replace(/^["']+|["']+$/g, '').trim();
        const portStr = `${cleanContP}${protoSuffix}`;
        const s = new YAML.Scalar(portStr);
        s.type = 'QUOTE_DOUBLE';
        return s;
      });
    }

    // Environment
    const activeEnv = svc.environment.filter((e) => e.key.trim());
    if (activeEnv.length > 0) {
      svcObj.environment = activeEnv.map((e) => `${e.key.trim()}=${escapeComposeEnvValue(e.value)}`);
    }

    // Volumes & Configs
    const activeVols = svc.volumes.filter((v) => v.target.trim());
    const regularVols = activeVols.filter((v) => v.type !== 'config');
    const configMounts = activeVols.filter((v) => v.type === 'config' && v.source.trim());

    if (regularVols.length > 0) {
      svcObj.volumes = regularVols.map((v) => {
        const modeSuffix = v.mode === 'ro' ? ':ro' : '';
        const cleanSrc = v.source.replace(/^["']+|["']+$/g, '').trim();
        const cleanTgt = v.target.replace(/^["']+|["']+$/g, '').trim();
        if (cleanSrc) {
          return `${cleanSrc}:${cleanTgt}${modeSuffix}`;
        }
        return `${cleanTgt}${modeSuffix}`;
      });
    }

    if (configMounts.length > 0) {
      svcObj.configs = configMounts.map((c) => {
        const item: any = {
          source: c.source.replace(/^["']+|["']+$/g, '').trim(),
          target: c.target.replace(/^["']+|["']+$/g, '').trim(),
        };
        if (c.uid && c.uid.trim()) item.uid = c.uid.trim();
        if (c.gid && c.gid.trim()) item.gid = c.gid.trim();
        return item;
      });
    }

    // Depends On
    if (svc.dependsOn && svc.dependsOn.length > 0) {
      const validDeps = svc.dependsOn.filter((d) => d && d !== svcKey);
      if (validDeps.length > 0) {
        if (svc.rawDependsOn && typeof svc.rawDependsOn === 'object') {
          const rawKeys = Object.keys(svc.rawDependsOn);
          const sameKeys =
            rawKeys.length === validDeps.length &&
            validDeps.every((d) => Object.prototype.hasOwnProperty.call(svc.rawDependsOn, d));
          if (sameKeys) {
            svcObj.depends_on = JSON.parse(JSON.stringify(svc.rawDependsOn));
          } else {
            const depObj: Record<string, any> = {};
            for (const d of validDeps) {
              depObj[d] = svc.rawDependsOn[d] || { condition: 'service_started' };
            }
            svcObj.depends_on = depObj;
          }
        } else {
          svcObj.depends_on = validDeps;
        }
      }
    }

    // Network Mode or Networks
    if (svc.networkMode && svc.networkMode.trim()) {
      svcObj.network_mode = svc.networkMode.trim();
    } else if (svc.networks && svc.networks.length > 0) {
      const validNets = svc.networks.filter(Boolean);
      if (validNets.length > 0) {
        if (svc.rawNetworks && typeof svc.rawNetworks === 'object') {
          const rawKeys = Object.keys(svc.rawNetworks);
          const sameKeys =
            rawKeys.length === validNets.length &&
            validNets.every((n) => Object.prototype.hasOwnProperty.call(svc.rawNetworks, n));
          if (sameKeys) {
            svcObj.networks = JSON.parse(JSON.stringify(svc.rawNetworks));
          } else {
            const netObj: Record<string, any> = {};
            for (const n of validNets) {
              netObj[n] = svc.rawNetworks[n] !== undefined ? svc.rawNetworks[n] : null;
            }
            svcObj.networks = netObj;
          }
        } else {
          svcObj.networks = validNets;
        }
      }
    }

    // Resource limits
    if (svc.memoryLimit.trim()) {
      svcObj.mem_limit = svc.memoryLimit.trim();
    }
    if (svc.cpuLimit.trim()) {
      svcObj.cpus = Number(svc.cpuLimit.trim()) || svc.cpuLimit.trim();
    }

    // Privileged
    if (svc.privileged) {
      svcObj.privileged = true;
    }

    // Web UI Management Port label + preserved custom labels
    const hasMgmtPortInSvc = svc.ports.some((p) => p.containerPort.trim() === mgmtPort);
    const mgmtLabel = (mgmtPort && hasMgmtPortInSvc) ? `nas.management.port=${mgmtPort}` : null;
    const customLabels = svc.customFields?.labels;
    if (customLabels) {
      if (Array.isArray(customLabels)) {
        svcObj.labels = mgmtLabel ? [...customLabels, mgmtLabel] : [...customLabels];
      } else if (typeof customLabels === 'object') {
        svcObj.labels = mgmtLabel ? { ...customLabels, 'nas.management.port': mgmtPort } : { ...customLabels };
      }
    } else if (mgmtLabel) {
      svcObj.labels = [mgmtLabel];
    }

    // Merge any other customFields (healthcheck, cap_add, user, shm_size, env_file, deploy, etc.)
    if (svc.customFields) {
      for (const [k, v] of Object.entries(svc.customFields)) {
        if (k !== 'labels') {
          svcObj[k] = k === 'deploy' && v && typeof v === 'object' ? JSON.parse(JSON.stringify(v)) : v;
        }
      }
    }

    // Devices
    const activeDevices = (svc.devices || []).filter((d) => d.trim());
    if (activeDevices.length > 0) {
      svcObj.devices = activeDevices;
    }

    // Clean up migrated deploy.resources.limits (moved to mem_limit / cpus)
    if (svcObj.deploy?.resources?.limits) {
      delete svcObj.deploy.resources.limits.memory;
      delete svcObj.deploy.resources.limits.cpus;
      if (Object.keys(svcObj.deploy.resources.limits).length === 0) {
        delete svcObj.deploy.resources.limits;
      }
    }

    // Preserve explicit container runtime (e.g. nvidia, sysbox-runc) unless GPU was disabled on nvidia runtime
    if (svc.rawRuntime && (svc.rawRuntime !== 'nvidia' || svc.gpuEnabled)) {
      svcObj.runtime = svc.rawRuntime;
    }

    // NVIDIA GPU Passthrough (deploy.resources.reservations.devices)
    if (svc.gpuEnabled) {
      if (!svcObj.deploy) {
        svcObj.deploy = {};
      }
      if (!svcObj.deploy.resources) {
        svcObj.deploy.resources = {};
      }
      if (!svcObj.deploy.resources.reservations) {
        svcObj.deploy.resources.reservations = {};
      }
      const gpuReservation: any = {
        driver: 'nvidia',
        capabilities: ['gpu'],
      };
      if (svc.gpuCount === 'custom' && svc.gpuDeviceIds && svc.gpuDeviceIds.trim()) {
        gpuReservation.device_ids = svc.gpuDeviceIds
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean);
      } else {
        const countVal = svc.gpuCount ? svc.gpuCount.trim().toLowerCase() : 'all';
        if (countVal === 'all') {
          gpuReservation.count = 'all';
        } else if (!isNaN(Number(countVal))) {
          gpuReservation.count = Number(countVal);
        } else {
          gpuReservation.count = 'all';
        }
      }
      svcObj.deploy.resources.reservations.devices = [gpuReservation];
    } else if (Array.isArray(svcObj.deploy?.resources?.reservations?.devices)) {
      svcObj.deploy.resources.reservations.devices = svcObj.deploy.resources.reservations.devices.filter(
        (dev: any) => !(dev && (dev.driver === 'nvidia' || (Array.isArray(dev.capabilities) && dev.capabilities.includes('gpu'))))
      );
      if (svcObj.deploy.resources.reservations.devices.length === 0) {
        delete svcObj.deploy.resources.reservations.devices;
      }
    }

    // Clean up empty deploy.resources without deleting other deploy keys (replicas, placement, restart_policy, labels)
    if (svcObj.deploy && typeof svcObj.deploy === 'object') {
      if (svcObj.deploy.resources && typeof svcObj.deploy.resources === 'object') {
        if (
          svcObj.deploy.resources.reservations &&
          typeof svcObj.deploy.resources.reservations === 'object' &&
          Object.keys(svcObj.deploy.resources.reservations).length === 0
        ) {
          delete svcObj.deploy.resources.reservations;
        }
        if (Object.keys(svcObj.deploy.resources).length === 0) {
          delete svcObj.deploy.resources;
        }
      }
      if (Object.keys(svcObj.deploy).length === 0) {
        delete svcObj.deploy;
      }
    }

    rootObj.services[svcKey] = svcObj;
  }

  // Top-Level Networks
  const activeNets = form.networks.filter((n) => n.name.trim());
  if (activeNets.length > 0) {
    rootObj.networks = {};
    for (const net of activeNets) {
      const netKey = net.name.trim();
      const netObj: any = net.rawConfig ? { ...net.rawConfig } : {};
      if (net.external) {
        netObj.external = true;
      } else {
        delete netObj.external;
        if (net.driver && net.driver !== 'bridge') {
          netObj.driver = net.driver;
        }
        if (net.internal) {
          netObj.internal = true;
        }
      }
      rootObj.networks[netKey] = Object.keys(netObj).length > 0 ? netObj : {};
    }
  }

  // Top-Level Volumes
  const activeTopVols = form.volumes.filter((v) => v.name.trim());
  if (activeTopVols.length > 0) {
    rootObj.volumes = {};
    for (const vol of activeTopVols) {
      const volKey = vol.name.trim();
      const volObj: any = vol.rawConfig ? { ...vol.rawConfig } : {};
      if (vol.external) {
        volObj.external = true;
      } else {
        delete volObj.external;
        if (vol.driver && vol.driver !== 'local') {
          volObj.driver = vol.driver;
        }
      }
      rootObj.volumes[volKey] = Object.keys(volObj).length > 0 ? volObj : {};
    }
  }

  // Top-Level Configs (Inline files / ConfigMaps)
  const configMap = new Map<string, ComposeTopLevelConfig>();
  for (const cfg of (form.configs || [])) {
    if (cfg.name && cfg.name.trim()) {
      configMap.set(cfg.name.trim(), cfg);
    }
  }
  // Ensure configs referenced by services are also emitted
  for (const s of form.services) {
    for (const v of s.volumes) {
      if (v.type === 'config' && v.source.trim() && !configMap.has(v.source.trim())) {
        configMap.set(v.source.trim(), {
          id: generateUid(),
          name: v.source.trim(),
          content: '',
        });
      }
    }
  }

  if (configMap.size > 0) {
    rootObj.configs = {};
    for (const [cfgKey, cfg] of configMap.entries()) {
      const cfgObj: any = cfg.rawConfig ? { ...cfg.rawConfig } : {};
      if (cfg.external) {
        cfgObj.external = true;
      } else if (cfg.file) {
        cfgObj.file = cfg.file;
      } else {
        cfgObj.content = cfg.content !== undefined && cfg.content !== null ? cfg.content : '';
      }
      rootObj.configs[cfgKey] = cfgObj;
    }
  }

  // Emit remaining custom top-level fields (such as secrets)
  if (form.customTopLevelFields) {
    for (const [k, v] of Object.entries(form.customTopLevelFields)) {
      if (!(k in rootObj)) {
        rootObj[k] = v;
      }
    }
  }

  return YAML.stringify(rootObj, { indent: 2 });
}
