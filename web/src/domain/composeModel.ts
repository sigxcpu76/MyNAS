import YAML from 'yaml';

// =============================================================================
// Docker Compose Structured Form Entities
// =============================================================================

export interface ComposePortModel {
  hostPort: string;
  containerPort: string;
  protocol: 'tcp' | 'udp';
}

export interface ComposeEnvModel {
  key: string;
  value: string;
}

export interface ComposeVolumeMountModel {
  source: string; // Host path (e.g. /tank/data) or named volume
  target: string; // Container path (e.g. /var/www/html)
  mode: 'rw' | 'ro';
}

export interface ComposeServiceModel {
  id: string; // Client UI row id
  name: string; // Service key in compose (e.g. "web", "db", "redis")
  containerName: string;
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
}

export interface ComposeTopLevelNetwork {
  id: string;
  name: string;
  driver: 'bridge' | 'overlay' | 'macvlan' | string;
  internal: boolean;
}

export interface ComposeTopLevelVolume {
  id: string;
  name: string;
  driver: string;
  external: boolean;
}

export interface ComposeAppFormModel {
  appId: string;
  title: string;
  services: ComposeServiceModel[];
  networks: ComposeTopLevelNetwork[];
  volumes: ComposeTopLevelVolume[];
}

// =============================================================================
// Factory Helpers
// =============================================================================

let nextUid = 1;
export function generateUid(): string {
  return `uid_${Date.now()}_${nextUid++}`;
}

export function createDefaultService(name = 'app', image = ''): ComposeServiceModel {
  return {
    id: generateUid(),
    name,
    containerName: '',
    image,
    restart: 'unless-stopped',
    command: '',
    ports: [],
    environment: [],
    volumes: [],
    dependsOn: [],
    networkMode: '',
    networks: [],
    memoryLimit: '',
    cpuLimit: '',
    privileged: false,
    devices: [],
  };
}

export function createDefaultAppForm(appId = '', title = ''): ComposeAppFormModel {
  return {
    appId,
    title,
    services: [createDefaultService('app')],
    networks: [],
    volumes: [],
  };
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
  fallbackTitle = ''
): ComposeAppFormModel {
  const model: ComposeAppFormModel = {
    appId: fallbackAppId,
    title: fallbackTitle,
    services: [],
    networks: [],
    volumes: [],
  };

  if (!yamlStr || !yamlStr.trim()) {
    model.services.push(createDefaultService());
    return model;
  }

  let parsed: any;
  try {
    parsed = YAML.parse(yamlStr);
  } catch {
    // If parse fails, return default with raw YAML intact
    model.services.push(createDefaultService());
    return model;
  }

  if (!parsed || typeof parsed !== 'object') {
    model.services.push(createDefaultService());
    return model;
  }

  // 1. Parse Services
  const rawServices = parsed.services || {};
  for (const [svcName, rawSvc] of Object.entries(rawServices)) {
    if (!rawSvc || typeof rawSvc !== 'object') continue;
    const svc = rawSvc as any;

    const serviceModel: ComposeServiceModel = {
      id: generateUid(),
      name: svcName,
      containerName: svc.container_name || '',
      image: svc.image || '',
      restart: svc.restart || 'unless-stopped',
      command: Array.isArray(svc.command)
        ? svc.command.join(' ')
        : typeof svc.command === 'string'
        ? svc.command
        : '',
      ports: [],
      environment: [],
      volumes: [],
      dependsOn: [],
      networkMode: svc.network_mode || '',
      networks: [],
      memoryLimit: svc.mem_limit || svc.deploy?.resources?.limits?.memory || '',
      cpuLimit: svc.cpus || svc.deploy?.resources?.limits?.cpus || '',
      privileged: Boolean(svc.privileged ?? false),
      devices: Array.isArray(svc.devices) ? svc.devices.map(String) : [],
    };

    // Parse Ports
    if (Array.isArray(svc.ports)) {
      for (const p of svc.ports) {
        if (typeof p === 'string' || typeof p === 'number') {
          const str = String(p);
          const parts = str.split(':');
          if (parts.length >= 2) {
            let hostP = parts[0];
            let containerPart = parts[1];
            if (parts.length === 3) {
              hostP = parts[1];
              containerPart = parts[2];
            }
            const [contP, proto] = containerPart.split('/');
            serviceModel.ports.push({
              hostPort: hostP.trim(),
              containerPort: contP.trim(),
              protocol: (proto?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
            });
          } else {
            const [contP, proto] = str.split('/');
            serviceModel.ports.push({
              hostPort: '',
              containerPort: contP.trim(),
              protocol: (proto?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
            });
          }
        } else if (p && typeof p === 'object') {
          serviceModel.ports.push({
            hostPort: String(p.published || ''),
            containerPort: String(p.target || ''),
            protocol: (p.protocol?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
          });
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
              value: envItem.substring(idx + 1),
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
          value: v === null || v === undefined ? '' : String(v),
        });
      }
    }

    // Parse Volumes
    if (Array.isArray(svc.volumes)) {
      for (const v of svc.volumes) {
        if (typeof v === 'string') {
          const parts = v.split(':');
          if (parts.length >= 2) {
            const source = parts[0].trim();
            const target = parts[1].trim();
            const mode = (parts[2]?.trim().toLowerCase() === 'ro' ? 'ro' : 'rw') as 'rw' | 'ro';
            serviceModel.volumes.push({ source, target, mode });
          } else {
            serviceModel.volumes.push({ source: '', target: v.trim(), mode: 'rw' });
          }
        } else if (v && typeof v === 'object') {
          serviceModel.volumes.push({
            source: v.source || '',
            target: v.target || '',
            mode: v.read_only ? 'ro' : 'rw',
          });
        }
      }
    }

    // Parse depends_on
    if (Array.isArray(svc.depends_on)) {
      serviceModel.dependsOn = svc.depends_on.map(String);
    } else if (svc.depends_on && typeof svc.depends_on === 'object') {
      serviceModel.dependsOn = Object.keys(svc.depends_on);
    }

    // Parse networks inside service
    if (Array.isArray(svc.networks)) {
      serviceModel.networks = svc.networks.map(String);
    } else if (svc.networks && typeof svc.networks === 'object') {
      serviceModel.networks = Object.keys(svc.networks);
    }

    model.services.push(serviceModel);
  }

  if (model.services.length === 0) {
    model.services.push(createDefaultService());
  }

  // 2. Parse Top-Level Networks
  if (parsed.networks && typeof parsed.networks === 'object') {
    for (const [netName, netConfig] of Object.entries(parsed.networks)) {
      const net = (netConfig || {}) as any;
      model.networks.push({
        id: generateUid(),
        name: netName,
        driver: net.driver || 'bridge',
        internal: Boolean(net.internal ?? false),
      });
    }
  }

  // 3. Parse Top-Level Volumes
  if (parsed.volumes && typeof parsed.volumes === 'object') {
    for (const [volName, volConfig] of Object.entries(parsed.volumes)) {
      const vol = (volConfig || {}) as any;
      model.volumes.push({
        id: generateUid(),
        name: volName,
        driver: vol.driver || 'local',
        external: Boolean(vol.external ?? false),
      });
    }
  }

  return model;
}

/**
 * Converts structured form model to standard Docker Compose YAML specification.
 */
export function formToComposeYaml(form: ComposeAppFormModel): string {
  const rootObj: any = {
    services: {},
  };

  for (const svc of form.services) {
    const svcKey = svc.name.trim() || 'app';
    const svcObj: any = {};

    if (svc.containerName.trim()) {
      svcObj.container_name = svc.containerName.trim();
    }

    if (svc.image.trim()) {
      svcObj.image = svc.image.trim();
    }

    if (svc.restart && svc.restart !== 'no') {
      svcObj.restart = svc.restart;
    }

    if (svc.command.trim()) {
      svcObj.command = svc.command.trim();
    }

    // Ports
    const activePorts = svc.ports.filter((p) => p.containerPort.trim());
    if (activePorts.length > 0) {
      svcObj.ports = activePorts.map((p) => {
        const protoSuffix = p.protocol === 'udp' ? '/udp' : '';
        const portStr = p.hostPort.trim()
          ? `${p.hostPort.trim()}:${p.containerPort.trim()}${protoSuffix}`
          : `${p.containerPort.trim()}${protoSuffix}`;
        const s = new YAML.Scalar(portStr);
        s.type = 'QUOTE_DOUBLE';
        return s;
      });
    }

    // Environment
    const activeEnv = svc.environment.filter((e) => e.key.trim());
    if (activeEnv.length > 0) {
      svcObj.environment = activeEnv.map((e) => `${e.key.trim()}=${e.value}`);
    }

    // Volumes
    const activeVols = svc.volumes.filter((v) => v.target.trim());
    if (activeVols.length > 0) {
      svcObj.volumes = activeVols.map((v) => {
        const modeSuffix = v.mode === 'ro' ? ':ro' : '';
        if (v.source.trim()) {
          return `${v.source.trim()}:${v.target.trim()}${modeSuffix}`;
        }
        return `${v.target.trim()}${modeSuffix}`;
      });
    }

    // Depends On
    if (svc.dependsOn && svc.dependsOn.length > 0) {
      svcObj.depends_on = svc.dependsOn.filter((d) => d && d !== svcKey);
    }

    // Network Mode or Networks
    if (svc.networkMode && svc.networkMode.trim()) {
      svcObj.network_mode = svc.networkMode.trim();
    } else if (svc.networks && svc.networks.length > 0) {
      svcObj.networks = svc.networks.filter(Boolean);
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

    // Devices
    const activeDevices = (svc.devices || []).filter((d) => d.trim());
    if (activeDevices.length > 0) {
      svcObj.devices = activeDevices;
    }

    rootObj.services[svcKey] = svcObj;
  }

  // Top-Level Networks
  const activeNets = form.networks.filter((n) => n.name.trim());
  if (activeNets.length > 0) {
    rootObj.networks = {};
    for (const net of activeNets) {
      const netKey = net.name.trim();
      const netObj: any = {};
      if (net.driver && net.driver !== 'bridge') {
        netObj.driver = net.driver;
      }
      if (net.internal) {
        netObj.internal = true;
      }
      rootObj.networks[netKey] = Object.keys(netObj).length > 0 ? netObj : null;
    }
  }

  // Top-Level Volumes
  const activeTopVols = form.volumes.filter((v) => v.name.trim());
  if (activeTopVols.length > 0) {
    rootObj.volumes = {};
    for (const vol of activeTopVols) {
      const volKey = vol.name.trim();
      const volObj: any = {};
      if (vol.driver && vol.driver !== 'local') {
        volObj.driver = vol.driver;
      }
      if (vol.external) {
        volObj.external = true;
      }
      rootObj.volumes[volKey] = Object.keys(volObj).length > 0 ? volObj : null;
    }
  }

  return YAML.stringify(rootObj, { indent: 2 });
}
