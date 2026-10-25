import YAML from 'yaml';

// =============================================================================
// Docker Compose Structured Form Entities
// =============================================================================

export interface ComposePortModel {
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

export interface ComposePortalModel {
  name: string;
  scheme: string;
  port: string;
  path: string;
}

export interface ComposeAppFormModel {
  appId: string;
  title: string;
  services: ComposeServiceModel[];
  networks: ComposeTopLevelNetwork[];
  volumes: ComposeTopLevelVolume[];
  managementPort?: string; // Container internal port used for Open Web UI (e.g. "81")
  portals?: ComposePortalModel[];
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
    managementPort: '',
    portals: [],
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
            let cleanHostP = hostP.trim();
            if (!cleanHostP) {
              cleanHostP = String(generateRandomHighPort());
            }
            serviceModel.ports.push({
              hostPort: cleanHostP,
              containerPort: contP.trim(),
              protocol: (proto?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
              publishToHost: true,
            });
          } else {
            // Port specified under `ports:` without host mapping (e.g. - "80")
            // Pick a random high port in 32000+ range
            const [contP, proto] = str.split('/');
            const randHost = generateRandomHighPort();
            serviceModel.ports.push({
              hostPort: String(randHost),
              containerPort: contP.trim(),
              protocol: (proto?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
              publishToHost: true,
            });
          }
        } else if (p && typeof p === 'object') {
          const hasHost = p.published !== undefined && p.published !== null && String(p.published).trim() !== '';
          const hostP = hasHost ? String(p.published).trim() : String(generateRandomHighPort());
          serviceModel.ports.push({
            hostPort: hostP,
            containerPort: String(p.target || '').trim(),
            protocol: (p.protocol?.toLowerCase() === 'udp' ? 'udp' : 'tcp') as 'tcp' | 'udp',
            publishToHost: true,
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

  // 4. Parse Portals (TrueNAS SCALE x-portals or standard portals)
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
  if (mgmtPort) {
    for (const s of model.services) {
      for (const p of s.ports) {
        if (p.containerPort === mgmtPort) {
          p.isManagementPort = true;
        }
      }
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

    // Ports & Expose
    const activePorts = svc.ports.filter((p) => p.containerPort.trim());
    const publishedPorts = activePorts.filter((p) => p.publishToHost !== false);
    const internalPorts = activePorts.filter((p) => p.publishToHost === false);

    if (publishedPorts.length > 0) {
      svcObj.ports = publishedPorts.map((p) => {
        let hostP = p.hostPort.trim();
        if (!hostP) {
          hostP = String(generateRandomHighPort());
          p.hostPort = hostP;
        }
        const protoSuffix = p.protocol === 'udp' ? '/udp' : '';
        const portStr = `${hostP}:${p.containerPort.trim()}${protoSuffix}`;
        const s = new YAML.Scalar(portStr);
        s.type = 'QUOTE_DOUBLE';
        return s;
      });
    }

    if (internalPorts.length > 0) {
      svcObj.expose = internalPorts.map((p) => {
        const protoSuffix = p.protocol === 'udp' ? '/udp' : '';
        const portStr = `${p.containerPort.trim()}${protoSuffix}`;
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

    // Web UI Management Port label
    const hasMgmtPortInSvc = svc.ports.some((p) => p.containerPort.trim() === mgmtPort);
    if (mgmtPort && hasMgmtPortInSvc) {
      svcObj.labels = [`nas.management.port=${mgmtPort}`];
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
