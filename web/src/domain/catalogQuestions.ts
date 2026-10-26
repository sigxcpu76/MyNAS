import type { CatalogApp, CatalogQuestion, CatalogQuestionGroup } from './models';
import { toZfsSafeSlug } from './id';

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
    if (q.type === 'hostpath' || q.variable.toLowerCase().includes('storage')) {
      if (defVal.startsWith('/mnt/')) {
        defVal = defVal.replace('/mnt/', '/nas/');
      }
      if (!defVal) {
        const parts = q.variable.split('.');
        const leaf = parts[parts.length - 1] || 'data';
        const cleanId = toZfsSafeSlug(app.appId) || 'app';
        defVal = `/nas/${appsPool}/apps/${cleanId}/${leaf}`;
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

/**
 * Generates a clean Docker Compose YAML descriptor from answered TrueNAS SCALE questions.
 */
export function generateComposeFromQuestions(
  app: CatalogApp,
  answers: Record<string, string>,
  appsPool = 'tank'
): string {
  const serviceName = toZfsSafeSlug(app.appId || 'app') || 'app';
  const image = app.containerImage || `${serviceName}:latest`;
  const appdataPath = `/nas/${appsPool}/apps`;

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
        const containerPort = app.defaultPort && app.defaultPort > 0 ? app.defaultPort : p;
        ports.push(`"${p}:${containerPort}"`);
        portals.push({
          name: q.label || 'Web UI',
          port: p,
          scheme: 'http',
          path: '/',
        });
      }
    }

    // 2. Storage / Volumes
    if (q.type === 'hostpath' || vLower.includes('storage') || vLower.includes('volume')) {
      const mountPath = q.mountPath || '/data';
      const hostPath = val || `${appdataPath}/${serviceName}/data`;
      volumes.push(`"${hostPath}:${mountPath}"`);
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
    volumes.push(`"${appdataPath}/${serviceName}/data:/data"`);
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
      lines.push(`      - ${k}=${v}`);
    }
  }

  return lines.join('\n') + '\n';
}
