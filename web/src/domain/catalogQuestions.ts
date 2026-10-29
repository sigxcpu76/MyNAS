import YAML from 'yaml';
import type { CatalogApp, CatalogQuestion, CatalogQuestionGroup } from './models';
import { toZfsSafeSlug } from './id';
import { escapeComposeEnvValue } from './composeModel';

/**
 * Returns default answers for an application based on its question groups / questions.
 * Substitutes the actual apps pool name for any default paths.
 */
export function getQuestionsDefaultAnswers(
  app: CatalogApp,
  appsPool = 'tank'
): Record<string, string> {
  const answers: Record<string, string> = {};
  const flatQs = getAllAppQuestions(app);

  for (const q of flatQs) {
    let defVal = q.defaultValue || '';
    if (q.type === 'hostpath' || q.type === 'path') {
      if (defVal.startsWith('/nas/')) {
        defVal = defVal.replace('/nas/', '/mnt/');
      }
      if (!defVal) {
        const parts = q.variable.split('.');
        let leaf = parts[parts.length - 1] || 'data';
        if (leaf === 'host_path' || leaf === 'path') {
          leaf = parts.length > 1 ? parts[parts.length - 2] : 'data';
        }
        const cleanId = toZfsSafeSlug(app.appId) || 'app';
        defVal = `/mnt/${appsPool}/apps/${cleanId}/${leaf}`;
      }
    }
    answers[q.variable] = defVal;
  }

  return answers;
}

/**
 * Flattens all questions from groups or app.questions.
 */
export function getAllAppQuestions(app: CatalogApp): CatalogQuestion[] {
  if (app.questions && app.questions.length > 0) {
    return app.questions;
  }
  const result: CatalogQuestion[] = [];
  if (app.questionGroups) {
    for (const g of app.questionGroups) {
      if (g.questions) {
        result.push(...g.questions);
      }
    }
  }
  return result;
}

/**
 * Categorizes a question into our standardized groups.
 */
export function categorizeQuestion(q: CatalogQuestion): string {
  const vLower = q.variable.toLowerCase();
  const gLower = (q.group || '').toLowerCase();

  // Network & Web Portal
  if (
    vLower.includes('port') ||
    vLower.includes('network') ||
    gLower.includes('network') ||
    (vLower.includes('host') && !vLower.includes('path'))
  ) {
    return 'Network & Web Portal';
  }

  // Persistent Storage & Datasets
  if (
    q.type === 'hostpath' ||
    vLower.includes('storage') ||
    gLower.includes('storage') ||
    vLower.includes('path') ||
    vLower.includes('volume')
  ) {
    return 'Persistent Storage & Datasets';
  }

  // User Permissions & System
  if (
    vLower.includes('run_as') ||
    (vLower.includes('user') && !vLower.includes('admin')) ||
    (vLower.includes('group') && !vLower.includes('group_id')) ||
    gLower.includes('user and group') ||
    gLower.includes('permissions')
  ) {
    return 'User Permissions & System';
  }

  // Default to Application Configuration & Credentials
  return 'Application Configuration & Credentials';
}

/**
 * Organizes questions into standardized groups if they aren't already grouped.
 */
export function organizeQuestionGroups(app: CatalogApp): CatalogQuestionGroup[] {
  if (app.questionGroups && app.questionGroups.length > 0) {
    return app.questionGroups;
  }

  const flat = getAllAppQuestions(app);
  const map = new Map<string, CatalogQuestion[]>();

  const standardOrder = [
    'Network & Web Portal',
    'Persistent Storage & Datasets',
    'Application Configuration & Credentials',
    'User Permissions & System',
  ];

  for (const name of standardOrder) {
    map.set(name, []);
  }

  for (const q of flat) {
    const grp = categorizeQuestion(q);
    if (!map.has(grp)) {
      map.set(grp, []);
    }
    map.get(grp)!.push(q);
  }

  const result: CatalogQuestionGroup[] = [];
  for (const [name, questions] of map.entries()) {
    if (questions.length > 0) {
      result.push({
        name,
        description: getGroupDescription(name),
        questions,
      });
    }
  }

  return result;
}

export function getGroupDescription(name: string): string {
  switch (name) {
    case 'Network & Web Portal':
      return 'Configure host listening ports and web portal endpoints';
    case 'Persistent Storage & Datasets':
      return 'Bind mount persistent ZFS dataset storage paths for config and application data';
    case 'Application Configuration & Credentials':
      return 'Set application administrative credentials, environment variables, and operational mode';
    case 'User Permissions & System':
      return 'Configure container process user/group ownership (PUID/PGID)';
    default:
      return '';
  }
}

export function isWebPortalVariable(variable: string, label: string): boolean {
  const vLower = variable.toLowerCase();
  const lLower = label.toLowerCase();

  const nonWebPatterns = [
    'bt_port', 'torrent', 'peer', 'dns', 'dhcp', 'metrics',
    'prometheus', 'exporter', 'sync', 'cluster', 'member',
    'wireguard', 'vpn', 'tunnel', 'rtp', 'sip', 'tftp',
  ];
  for (const p of nonWebPatterns) {
    if (vLower.includes(p) || lLower.includes(p)) {
      return false;
    }
  }

  const webPatterns = [
    'web_port', 'webui', 'web_ui', 'portal', 'http', 'https',
    'admin', 'dashboard', 'gui', 'ui_port', 'frontend',
  ];
  for (const p of webPatterns) {
    if (vLower.includes(p) || lLower.includes(p)) {
      return true;
    }
  }

  if (lLower.includes('web ui') || lLower.includes('web portal') || lLower.includes('web interface')) {
    return true;
  }

  return false;
}

/**
 * Updates an existing Docker Compose definition by applying question answers
 * for ports, storage volumes, user IDs, and environment configuration.
 * Preserves all other services, container images, networks, devices, and settings.
 */
export function applyAnswersToCompose(
  rawYaml: string,
  app: CatalogApp,
  answers: Record<string, string>,
  appsPool = 'tank'
): string {
  try {
    const doc = YAML.parseDocument(rawYaml);
    const services = doc.get('services');
    if (!services || !YAML.isMap(services)) {
      return rawYaml;
    }

    const flat = getAllAppQuestions(app);

    for (const q of flat) {
      const val = answers[q.variable] ?? q.defaultValue;
      if (val === undefined || val === '') continue;
      const vLower = q.variable.toLowerCase();

      // 1. Port updates
      if (q.type === 'int' && (vLower.includes('port') || vLower.includes('web_port'))) {
        const newPort = parseInt(val, 10);
        if (!isNaN(newPort) && newPort > 0) {
          // Update x-portals in root
          const portals = doc.get('x-portals');
          if (portals && YAML.isSeq(portals)) {
            for (const pItem of portals.items) {
              if (YAML.isMap(pItem)) {
                pItem.set('port', newPort);
              }
            }
          }
          // Update port in services
          for (const sEntry of services.items) {
            const svc = sEntry.value;
            if (!svc || !YAML.isMap(svc)) continue;
            const ports = svc.get('ports');
            if (ports && YAML.isSeq(ports)) {
              for (let i = 0; i < ports.items.length; i++) {
                const portNode = ports.items[i];
                if (typeof portNode === 'string' || (portNode && typeof (portNode as any).value === 'string')) {
                  const str = String((portNode as any).value ?? portNode).replace(/^["']+|["']+$/g, '');
                  const parts = str.split(':');
                  if (parts.length >= 2) {
                    const cPart = parts.slice(1).join(':').replace(/^["']+|["']+$/g, '').trim();
                    const s = new YAML.Scalar(`${newPort}:${cPart}`);
                    s.type = 'QUOTE_DOUBLE';
                    ports.set(i, s);
                    break;
                  }
                } else if (YAML.isMap(portNode)) {
                  (portNode as YAML.YAMLMap).set('published', String(newPort));
                  break;
                }
              }
            }
          }
        }
      }

      // 2. Storage / Volumes updates
      if (q.type === 'hostpath' || (q.mountPath && (vLower.includes('storage') || vLower.includes('volume')))) {
        let targetMount = (q.mountPath || '').replace(/^["']+|["']+$/g, '').trim();
        let newHostPath = val.replace(/^["']+|["']+$/g, '').trim();
        if (targetMount && !targetMount.startsWith('/')) {
          targetMount = '/' + targetMount;
        }
        if (newHostPath && !newHostPath.startsWith('/')) {
          newHostPath = '/' + newHostPath;
        }
        if (targetMount && newHostPath) {
          let updated = false;
          for (const sEntry of services.items) {
            const svc = sEntry.value;
            if (!svc || !YAML.isMap(svc)) continue;
            const vols = svc.get('volumes');
            if (vols && YAML.isSeq(vols)) {
              for (let i = 0; i < vols.items.length; i++) {
                const vNode = vols.items[i];
                if (typeof vNode === 'string' || (vNode && typeof (vNode as any).value === 'string')) {
                  const str = String((vNode as any).value ?? vNode).replace(/^["']+|["']+$/g, '');
                  const parts = str.split(':');
                  if (parts.length >= 2) {
                    let cTarget = parts[1].replace(/^["']+|["']+$/g, '').trim();
                    if (!cTarget.startsWith('/')) {
                      cTarget = '/' + cTarget;
                    }
                    if (cTarget === targetMount) {
                      const mode = parts.length > 2 ? `:${parts[2].replace(/^["']+|["']+$/g, '').trim()}` : '';
                      vols.set(i, `${newHostPath}:${cTarget}${mode}`);
                      updated = true;
                      break;
                    }
                  }
                } else if (YAML.isMap(vNode)) {
                  let tgt = String((vNode as YAML.YAMLMap).get('target') || '').replace(/^["']+|["']+$/g, '').trim();
                  if (tgt && !tgt.startsWith('/')) {
                    tgt = '/' + tgt;
                  }
                  if (tgt === targetMount) {
                    (vNode as YAML.YAMLMap).set('source', newHostPath);
                    updated = true;
                    break;
                  }
                }
              }
            }
          }
          if (!updated && services.items.length > 0) {
            const firstSvc = services.items[0].value;
            if (firstSvc && YAML.isMap(firstSvc)) {
              let vols = firstSvc.get('volumes');
              if (!vols) {
                firstSvc.set('volumes', doc.createNode([`${newHostPath}:${targetMount}`]));
              } else if (YAML.isSeq(vols)) {
                // If the only existing volume is a generic stub fallback (/data), replace it
                if (vols.items.length === 1) {
                  const singleV = String((vols.items[0] as any)?.value ?? vols.items[0] ?? '').replace(/^["']+|["']+$/g, '').trim();
                  const parts = singleV.split(':');
                  if (parts.length >= 2) {
                    const existingTgt = parts[1].replace(/^["']+|["']+$/g, '').trim();
                    if (existingTgt === '/data' && targetMount !== '/data') {
                      vols.set(0, `${newHostPath}:${targetMount}`);
                      updated = true;
                    }
                  }
                }
                if (!updated) {
                  vols.add(`${newHostPath}:${targetMount}`);
                }
              }
            }
          }
        }
      }

      // 3. User / Group (PUID / PGID)
      if (vLower.includes('puid') || (vLower.includes('user') && vLower.includes('run_as'))) {
        updateOrSetEnv(services, 'PUID', val);
      }
      if (vLower.includes('pgid') || (vLower.includes('group') && vLower.includes('run_as'))) {
        updateOrSetEnv(services, 'PGID', val);
      }

      // 4. Configuration & Credentials / Environment variables
      if (categorizeQuestion(q) === 'Application Configuration & Credentials') {
        const parts = q.variable.split('.');
        const key = parts[parts.length - 1].toUpperCase().replace(/[^A-Z0-9_]/g, '_');
        updateOrSetEnv(services, key, val);
      }
    }

    let outYaml = doc.toString();
    for (const [k, v] of Object.entries(answers)) {
      if (v && outYaml.includes(`\${${k}}`)) {
        outYaml = outYaml.replaceAll(`\${${k}}`, v);
      }
    }
    return outYaml;
  } catch (err) {
    console.warn('Failed to apply answers to existing compose YAML:', err);
    return rawYaml;
  }
}

function updateOrSetEnv(services: YAML.YAMLMap, key: string, val: string) {
  for (const sEntry of services.items) {
    const svc = sEntry.value;
    if (!svc || !YAML.isMap(svc)) continue;
    const env = svc.get('environment');
    if (env && YAML.isSeq(env)) {
      let found = false;
      for (let i = 0; i < env.items.length; i++) {
        const item = String((env.items[i] as any)?.value ?? env.items[i]);
        if (item.startsWith(`${key}=`)) {
          env.set(i, `${key}=${val}`);
          found = true;
          break;
        }
      }
      if (!found) {
        env.add(`${key}=${val}`);
      }
    } else if (env && YAML.isMap(env)) {
      env.set(key, val);
    }
  }
}

/**
 * Generates a clean Docker Compose YAML descriptor from answered TrueNAS SCALE questions.
 */
export function generateComposeFromQuestions(
  app: CatalogApp,
  answers: Record<string, string>,
  appsPool = 'tank'
): string {
  if (app.composeYaml && app.composeYaml.includes('services:')) {
    return applyAnswersToCompose(app.composeYaml, app, answers, appsPool);
  }

  const rawId = app.appId?.includes('.') ? app.appId.split('.').pop()! : (app.appId || 'app');
  const serviceName = toZfsSafeSlug(rawId) || 'app';
  const image = app.containerImage || `${serviceName}:latest`;
  const appdataPath = `/mnt/${appsPool}/apps`;

  const flat = getAllAppQuestions(app);
  const ports: string[] = [];
  const portals: { name: string; port: number; scheme: string; path: string }[] = [];
  const volumes: string[] = [];
  const envs: Record<string, string> = {};
  let puid = '1000';
  let pgid = '1000';
  let hasUserConfig = false;

  for (const q of flat) {
    const val = answers[q.variable] ?? q.defaultValue;
    const vLower = q.variable.toLowerCase();

    // 1. Ports
    if (q.type === 'int' && (vLower.includes('port') || vLower.includes('web_port'))) {
      const p = parseInt(val, 10);
      if (!isNaN(p) && p > 0) {
        const isPortal = isWebPortalVariable(q.variable, q.label || '');
        let containerPort = isPortal && app.defaultPort && app.defaultPort > 0 && app.defaultPort !== p ? app.defaultPort : p;
        if (app.appId === 'elastic-search' || app.appId?.includes('elasticsearch')) {
          containerPort = 9200;
        }
        ports.push(`"${p}:${containerPort}"`);
        if (isPortal) {
          portals.push({
            name: q.label || 'Web UI',
            port: p,
            scheme: 'http',
            path: '/',
          });
        }
      }
    }

    // 2. Storage / Volumes
    if (q.type === 'hostpath' || (q.mountPath && (vLower.includes('storage') || vLower.includes('volume')))) {
      // Ignore TrueNAS internal storage configuration variables / settings
      if (
        vLower.endsWith('.type') ||
        vLower.includes('ix_volume') ||
        vLower.includes('acl_enable') ||
        vLower.includes('dataset_name') ||
        vLower.includes('acl_entries') ||
        vLower.includes('cifs_config') ||
        vLower.includes('nfs_config') ||
        vLower.endsWith('.read_only') ||
        val === 'ix_volume' ||
        val === 'host_path' ||
        val === 'true' ||
        val === 'false'
      ) {
        continue;
      }

      let mountPath = q.mountPath;
      if (
        !mountPath ||
        mountPath === '/type' ||
        mountPath === '/acl_enable' ||
        mountPath === '/dataset_name' ||
        mountPath === '/path'
      ) {
        const parts = q.variable.split('.');
        let name = parts[parts.length - 1];
        if (name === 'host_path' || name === 'path') {
          name = parts.length > 1 ? parts[parts.length - 2] : 'data';
        }
        mountPath = (app.appId === 'elastic-search' || app.appId.includes('elasticsearch')) && name === 'data'
          ? '/usr/share/elasticsearch/data'
          : `/${name}`;
      }
      if (mountPath && !mountPath.startsWith('/')) {
        mountPath = '/' + mountPath;
      }

      let hostPath = val;
      if (!hostPath || !hostPath.startsWith('/')) {
        const parts = q.variable.split('.');
        let name = parts[parts.length - 1];
        if (name === 'host_path' || name === 'path') {
          name = parts.length > 1 ? parts[parts.length - 2] : 'data';
        }
        hostPath = `${appdataPath}/${serviceName}/${name}`;
      }

      const mapping = `"${hostPath}:${mountPath}"`;
      if (!volumes.includes(mapping)) {
        volumes.push(mapping);
      }
    }

    // 3. User / Group Permissions
    if (vLower.includes('user') && (vLower.includes('run_as') || vLower.includes('puid'))) {
      if (val && val !== '0') {
        puid = val;
        hasUserConfig = true;
      }
    }
    if (vLower.includes('group') && (vLower.includes('run_as') || vLower.includes('pgid'))) {
      if (val && val !== '0') {
        pgid = val;
        hasUserConfig = true;
      }
    }

    // 4. Configuration / Environment Variables
    if (categorizeQuestion(q) === 'Application Configuration & Credentials' && val !== '') {
      const parts = q.variable.split('.');
      const rawKey = parts[parts.length - 1];
      const key = rawKey.toUpperCase().replace(/[^A-Z0-9_]/g, '_');
      envs[key] = val;
    }
  }

  // Standalone Elasticsearch defaults
  if (app.appId === 'elastic-search' || app.appId?.includes('elasticsearch')) {
    if (!envs['discovery.type'] && !envs['DISCOVERY_TYPE']) {
      envs['discovery.type'] = 'single-node';
    }
    if (!envs['xpack.security.enabled'] && !envs['XPACK_SECURITY_ENABLED']) {
      envs['xpack.security.enabled'] = 'false';
    }
    if (!envs['ES_JAVA_OPTS']) {
      const heap = envs['HEAP_SIZE'] || '512m';
      envs['ES_JAVA_OPTS'] = `-Xms${heap} -Xmx${heap}`;
    }
  }

  // Fallbacks if no port was defined by questions but app has defaultPort
  if (ports.length === 0 && app.defaultPort && app.defaultPort > 0) {
    ports.push(`"${app.defaultPort}:${app.defaultPort}"`);
    portals.push({
      name: 'Web UI',
      port: app.defaultPort,
      scheme: 'http',
      path: '/',
    });
  }

  // Fallback volume if none specified
  if (volumes.length === 0) {
    const defaultMount = (app.appId === 'elastic-search' || app.appId.includes('elasticsearch'))
      ? '/usr/share/elasticsearch/data'
      : '/data';
    volumes.push(`"${appdataPath}/${serviceName}/data:${defaultMount}"`);
  }

  // Build Compose YAML with x-portals
  const lines: string[] = [];

  // x-portals header
  if (portals.length > 0) {
    lines.push('x-portals:');
    for (const portal of portals) {
      lines.push(`  - name: "${portal.name}"`);
      lines.push(`    port: ${portal.port}`);
      lines.push(`    scheme: "${portal.scheme}"`);
      lines.push(`    path: "${portal.path}"`);
    }
  }

  lines.push('services:');
  lines.push(`  ${serviceName}:`);
  lines.push(`    image: ${image}`);
  lines.push(`    container_name: ${serviceName}`);
  lines.push('    restart: unless-stopped');

  if (hasUserConfig && puid && pgid) {
    lines.push(`    user: "${puid}:${pgid}"`);
  }

  if (ports.length > 0) {
    lines.push('    ports:');
    for (const p of ports) {
      lines.push(`      - ${p}`);
    }
  }

  if (volumes.length > 0) {
    lines.push('    volumes:');
    for (const v of volumes) {
      lines.push(`      - ${v}`);
    }
  }

  const envEntries = Object.entries(envs);
  if (envEntries.length > 0) {
    lines.push('    environment:');
    for (const [k, v] of envEntries) {
      lines.push(`      - ${k}=${escapeComposeEnvValue(String(v))}`);
    }
  }

  return lines.join('\n') + '\n';
}
