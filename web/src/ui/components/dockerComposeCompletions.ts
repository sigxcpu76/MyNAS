import {
  type Completion,
  type CompletionContext,
  type CompletionResult,
  snippetCompletion,
} from '@codemirror/autocomplete';
import type { Diagnostic } from '@codemirror/lint';
import type { EditorView } from '@codemirror/view';
import * as yaml from 'yaml';

// ============================================================================
// Docker Compose Root Keys & Top-Level Directives
// ============================================================================
const ROOT_COMPLETIONS: Completion[] = [
  snippetCompletion('services:\n  ${1:app}:\n    image: ${2:image:tag}\n    restart: unless-stopped\n    ports:\n      - "${3:8080}:${4:80}"\n', {
    label: 'services:',
    detail: 'Define application containers & microservices',
    type: 'keyword',
    boost: 99,
  }),
  snippetCompletion('networks:\n  ${1:default}:\n    driver: bridge\n', {
    label: 'networks:',
    detail: 'Top-level custom bridge/overlay networks',
    type: 'keyword',
    boost: 50,
  }),
  snippetCompletion('volumes:\n  ${1:data}:\n    driver: local\n', {
    label: 'volumes:',
    detail: 'Top-level persistent named volumes',
    type: 'keyword',
    boost: 50,
  }),
  {
    label: 'version: "3.8"',
    detail: 'Docker Compose file format specification version',
    type: 'keyword',
    boost: 40,
  },
  snippetCompletion('configs:\n  ${1:my_config}:\n    file: ${2:./config.json}\n', {
    label: 'configs:',
    detail: 'Application runtime configurations',
    type: 'keyword',
    boost: 20,
  }),
  snippetCompletion('secrets:\n  ${1:my_secret}:\n    file: ${2:./secret.txt}\n', {
    label: 'secrets:',
    detail: 'Sensitive runtime secrets',
    type: 'keyword',
    boost: 20,
  }),
];

// ============================================================================
// Service Property Directives
// ============================================================================
const SERVICE_PROPERTY_COMPLETIONS: Completion[] = [
  snippetCompletion('image: ${1:repo/image:tag}', {
    label: 'image:',
    detail: 'Container image to run',
    type: 'property',
    boost: 95,
  }),
  snippetCompletion('container_name: ${1:custom-name}', {
    label: 'container_name:',
    detail: 'Specify a custom container name',
    type: 'property',
    boost: 90,
  }),
  snippetCompletion('restart: ${1:unless-stopped}', {
    label: 'restart:',
    detail: 'Container restart policy (always, unless-stopped, on-failure, no)',
    type: 'property',
    boost: 88,
  }),
  snippetCompletion('ports:\n  - "${1:8080}:${2:80}"', {
    label: 'ports:',
    detail: 'Expose host-to-container port mappings',
    type: 'property',
    boost: 86,
  }),
  snippetCompletion('environment:\n  - ${1:KEY}=${2:value}', {
    label: 'environment:',
    detail: 'Environment variables for container',
    type: 'property',
    boost: 85,
  }),
  snippetCompletion('volumes:\n  - ${1:/host/path}:${2:/container/path}:${3:rw}', {
    label: 'volumes:',
    detail: 'Mount host path or named volume inside container',
    type: 'property',
    boost: 84,
  }),
  snippetCompletion('depends_on:\n  - ${1:service_name}', {
    label: 'depends_on:',
    detail: 'Express startup and dependency order between services',
    type: 'property',
    boost: 80,
  }),
  snippetCompletion('networks:\n  - ${1:default}', {
    label: 'networks:',
    detail: 'Networks that the container joins',
    type: 'property',
    boost: 78,
  }),
  snippetCompletion('network_mode: "${1:host}"', {
    label: 'network_mode:',
    detail: 'Network isolation mode (host, bridge, none, container:...)',
    type: 'property',
    boost: 76,
  }),
  snippetCompletion('command: ${1:npm start}', {
    label: 'command:',
    detail: 'Override default container command',
    type: 'property',
    boost: 75,
  }),
  snippetCompletion('entrypoint: ${1:/entrypoint.sh}', {
    label: 'entrypoint:',
    detail: 'Override container entrypoint executable',
    type: 'property',
    boost: 74,
  }),
  snippetCompletion('healthcheck:\n  test: ["CMD", "${1:curl}", "-f", "${2:http://localhost:8080}"]\n  interval: ${3:30s}\n  timeout: ${4:10s}\n  retries: ${5:3}\n  start_period: ${6:20s}', {
    label: 'healthcheck:',
    detail: 'Container health check probes',
    type: 'property',
    boost: 72,
  }),
  snippetCompletion('deploy:\n  resources:\n    limits:\n      cpus: "${1:1.0}"\n      memory: ${2:512M}', {
    label: 'deploy / resources:',
    detail: 'Set CPU core and RAM memory constraints',
    type: 'property',
    boost: 70,
  }),
  snippetCompletion('privileged: ${1:true}', {
    label: 'privileged:',
    detail: 'Give full extended host kernel privileges to container',
    type: 'property',
    boost: 68,
  }),
  snippetCompletion('devices:\n  - "${1:/dev/dri}:${2:/dev/dri}"', {
    label: 'devices:',
    detail: 'Hardware device passthrough (e.g. Intel/AMD GPU transcoding)',
    type: 'property',
    boost: 66,
  }),
  snippetCompletion('labels:\n  - "${1:traefik.enable=true}"', {
    label: 'labels:',
    detail: 'Attach metadata labels to container',
    type: 'property',
    boost: 64,
  }),
  snippetCompletion('env_file:\n  - ${1:.env}', {
    label: 'env_file:',
    detail: 'Load environment variables from file',
    type: 'property',
    boost: 62,
  }),
  snippetCompletion('user: "${1:1000:1000}"', {
    label: 'user:',
    detail: 'Run container process as specific UID:GID',
    type: 'property',
    boost: 60,
  }),
  snippetCompletion('working_dir: ${1:/app}', {
    label: 'working_dir:',
    detail: 'Working directory inside container',
    type: 'property',
    boost: 58,
  }),
  snippetCompletion('stdin_open: true\ntty: true', {
    label: 'stdin_open & tty:',
    detail: 'Keep STDIN open and allocate pseudo-TTY (-it)',
    type: 'property',
    boost: 56,
  }),
  snippetCompletion('cap_add:\n  - ${1:NET_ADMIN}', {
    label: 'cap_add:',
    detail: 'Add Linux kernel capabilities',
    type: 'property',
    boost: 54,
  }),
  snippetCompletion('cap_drop:\n  - ${1:ALL}', {
    label: 'cap_drop:',
    detail: 'Drop Linux kernel capabilities for hardening',
    type: 'property',
    boost: 52,
  }),
  snippetCompletion('dns:\n  - ${1:1.1.1.1}\n  - ${2:8.8.8.8}', {
    label: 'dns:',
    detail: 'Custom DNS nameservers for container',
    type: 'property',
    boost: 50,
  }),
  snippetCompletion('extra_hosts:\n  - "${1:host.docker.internal}:${2:host-gateway}"', {
    label: 'extra_hosts:',
    detail: 'Add entries into /etc/hosts',
    type: 'property',
    boost: 48,
  }),
];

// ============================================================================
// Service Snippets (Pre-configured stacks & microservices)
// ============================================================================
const SERVICE_SNIPPETS: Completion[] = [
  snippetCompletion(
    '${1:db}:\n    image: postgres:16-alpine\n    restart: unless-stopped\n    environment:\n      - POSTGRES_DB=${2:app}\n      - POSTGRES_USER=${3:app}\n      - POSTGRES_PASSWORD=${4:secret123}\n    volumes:\n      - ${5:pg_data}:/var/lib/postgresql/data\n',
    {
      label: 'postgres-service',
      detail: 'Complete PostgreSQL 16 Alpine database snippet',
      type: 'class',
      boost: 90,
    }
  ),
  snippetCompletion(
    '${1:redis}:\n    image: redis:7-alpine\n    restart: unless-stopped\n    command: redis-server --appendonly yes\n    volumes:\n      - ${2:redis_data}:/data\n',
    {
      label: 'redis-service',
      detail: 'Complete Redis 7 Alpine cache snippet',
      type: 'class',
      boost: 89,
    }
  ),
  snippetCompletion(
    '${1:web}:\n    image: nginx:alpine\n    restart: unless-stopped\n    ports:\n      - "${2:80}:80"\n    volumes:\n      - ${3:./html}:/usr/share/nginx/html:ro\n',
    {
      label: 'nginx-service',
      detail: 'Complete Nginx Alpine web server snippet',
      type: 'class',
      boost: 88,
    }
  ),
  snippetCompletion(
    '${1:mariadb}:\n    image: mariadb:11\n    restart: unless-stopped\n    environment:\n      - MARIADB_ROOT_PASSWORD=${2:rootsecret}\n      - MARIADB_DATABASE=${3:app}\n      - MARIADB_USER=${4:app}\n      - MARIADB_PASSWORD=${5:secret}\n    volumes:\n      - ${6:mariadb_data}:/var/lib/mysql\n',
    {
      label: 'mariadb-service',
      detail: 'Complete MariaDB 11 database snippet',
      type: 'class',
      boost: 85,
    }
  ),
];

// Value completions for specific keys
const RESTART_VALUES: Completion[] = [
  { label: 'unless-stopped', detail: 'Restart unless explicitly stopped (recommended)', type: 'constant' },
  { label: 'always', detail: 'Always restart the container on crash or reboot', type: 'constant' },
  { label: 'on-failure', detail: 'Restart only on non-zero exit status', type: 'constant' },
  { label: 'no', detail: 'Do not automatically restart', type: 'constant' },
];

const NETWORK_MODE_VALUES: Completion[] = [
  { label: '"host"', detail: 'Host network mode (shares host network stack directly)', type: 'constant' },
  { label: '"bridge"', detail: 'Standard Docker bridge network isolation', type: 'constant' },
  { label: '"none"', detail: 'Disable all container networking', type: 'constant' },
  { label: '"service:..."', detail: 'Share network stack with another container', type: 'constant' },
];

const NETWORK_DRIVER_VALUES: Completion[] = [
  { label: 'bridge', detail: 'Local bridge network driver', type: 'constant' },
  { label: 'host', detail: 'Use host networking', type: 'constant' },
  { label: 'overlay', detail: 'Multi-host swarm overlay network', type: 'constant' },
  { label: 'macvlan', detail: 'Assign MAC address to container on physical LAN', type: 'constant' },
];

// Helper to determine YAML context from current position and preceding lines
function analyzeYamlContext(docText: string, pos: number) {
  const lines = docText.slice(0, pos).split('\n');
  const currentLine = lines[lines.length - 1] ?? '';
  const currentIndent = currentLine.match(/^\s*/)?.[0].length ?? 0;

  let parentKey = '';
  let insideServices = false;
  let insideNetworks = false;
  let insideVolumes = false;

  // Scan backwards through preceding lines to discover hierarchy
  for (let i = lines.length - 2; i >= 0; i--) {
    const l = lines[i];
    if (!l.trim() || l.trim().startsWith('#')) continue;

    const indent = l.match(/^\s*/)?.[0].length ?? 0;
    const match = l.match(/^\s*([a-zA-Z0-9_\-]+):/);
    if (!match) continue;

    const key = match[1];

    if (indent === 0) {
      if (key === 'services') insideServices = true;
      if (key === 'networks') insideNetworks = true;
      if (key === 'volumes') insideVolumes = true;
      break;
    } else if (indent < currentIndent && !parentKey) {
      parentKey = key;
    }
  }

  return {
    currentLine,
    currentIndent,
    parentKey,
    insideServices,
    insideNetworks,
    insideVolumes,
  };
}

// ============================================================================
// CodeMirror 6 Docker Compose Completion Source
// ============================================================================
export function dockerComposeCompletionSource(context: CompletionContext): CompletionResult | null {
  const word = context.matchBefore(/[\w\-:"]*/);
  if (!word && !context.explicit) return null;

  const docText = context.state.doc.toString();
  const { currentLine, currentIndent, insideServices, insideNetworks, insideVolumes } =
    analyzeYamlContext(docText, context.pos);

  const lineObj = context.state.doc.lineAt(context.pos);
  const trimmedBefore = currentLine.slice(0, context.pos - lineObj.from).trim();

  // 1. After restart:
  if (trimmedBefore.endsWith('restart:') || trimmedBefore.endsWith('restart: ')) {
    return {
      from: word ? word.from : context.pos,
      options: RESTART_VALUES,
    };
  }

  // 2. After network_mode:
  if (trimmedBefore.endsWith('network_mode:') || trimmedBefore.endsWith('network_mode: ')) {
    return {
      from: word ? word.from : context.pos,
      options: NETWORK_MODE_VALUES,
    };
  }

  // 3. After driver:
  if (trimmedBefore.endsWith('driver:') || trimmedBefore.endsWith('driver: ')) {
    return {
      from: word ? word.from : context.pos,
      options: NETWORK_DRIVER_VALUES,
    };
  }

  // 4. Root level (indent 0)
  if (currentIndent === 0) {
    return {
      from: word ? word.from : context.pos,
      options: ROOT_COMPLETIONS,
    };
  }

  // 5. Inside services:
  if (insideServices) {
    if (currentIndent === 2) {
      // Defining a new service name
      return {
        from: word ? word.from : context.pos,
        options: [
          snippetCompletion('${1:app}:\n    image: ${2:repo/image:tag}\n    restart: unless-stopped\n', {
            label: 'service (custom)',
            detail: 'New service block skeleton',
            type: 'class',
            boost: 95,
          }),
          ...SERVICE_SNIPPETS,
        ],
      };
    } else if (currentIndent >= 4) {
      // Service properties
      return {
        from: word ? word.from : context.pos,
        options: SERVICE_PROPERTY_COMPLETIONS,
      };
    }
  }

  // 6. Top-level networks:
  if (insideNetworks && currentIndent >= 2) {
    return {
      from: word ? word.from : context.pos,
      options: [
        snippetCompletion('driver: ${1:bridge}', { label: 'driver:', detail: 'Network driver (bridge, host)', type: 'property' }),
        snippetCompletion('external: ${1:true}', { label: 'external:', detail: 'Use pre-existing Docker network', type: 'property' }),
        snippetCompletion('internal: ${1:true}', { label: 'internal:', detail: 'Restrict external internet gateway', type: 'property' }),
        snippetCompletion('ipam:\n  config:\n    - subnet: ${1:172.28.0.0/16}', { label: 'ipam:', detail: 'Custom IP address management / subnet', type: 'property' }),
      ],
    };
  }

  // 7. Top-level volumes:
  if (insideVolumes && currentIndent >= 2) {
    return {
      from: word ? word.from : context.pos,
      options: [
        snippetCompletion('driver: local', { label: 'driver:', detail: 'Volume driver (local)', type: 'property' }),
        snippetCompletion('external: ${1:true}', { label: 'external:', detail: 'Use pre-existing Docker volume', type: 'property' }),
      ],
    };
  }

  // Default: offer combined service properties and root completions
  return {
    from: word ? word.from : context.pos,
    options: [...SERVICE_PROPERTY_COMPLETIONS, ...ROOT_COMPLETIONS],
  };
}

// ============================================================================
// CodeMirror 6 YAML Syntax & Schema Diagnostics / Linter (LSP-like)
// ============================================================================
export function dockerComposeLinter(view: EditorView): Diagnostic[] {
  const docText = view.state.doc.toString();
  if (!docText.trim()) return [];

  const diagnostics: Diagnostic[] = [];

  try {
    const parsed = yaml.parseDocument(docText);

    // Collect syntax errors
    for (const err of parsed.errors) {
      const from = Math.max(0, Math.min(err.pos?.[0] ?? 0, docText.length));
      const to = Math.max(from + 1, Math.min(err.pos?.[1] ?? (from + 1), docText.length));
      diagnostics.push({
        from,
        to,
        severity: 'error',
        message: err.message.replace(/\s+at line \d+.*$/, ''),
      });
    }

    // Collect warnings
    for (const warn of parsed.warnings) {
      const from = Math.max(0, Math.min(warn.pos?.[0] ?? 0, docText.length));
      const to = Math.max(from + 1, Math.min(warn.pos?.[1] ?? (from + 1), docText.length));
      diagnostics.push({
        from,
        to,
        severity: 'warning',
        message: warn.message,
      });
    }

    // Basic structural checks: Docker Compose should have `services:` map
    const json = parsed.toJS();
    if (json && typeof json === 'object' && !json.services) {
      diagnostics.push({
        from: 0,
        to: Math.min(docText.indexOf('\n') > 0 ? docText.indexOf('\n') : docText.length, docText.length),
        severity: 'warning',
        message: 'A Docker Compose file typically requires a "services:" block defining containers.',
      });
    }
  } catch (err: any) {
    diagnostics.push({
      from: 0,
      to: Math.min(20, docText.length),
      severity: 'error',
      message: `Failed to parse YAML: ${err?.message || 'Syntax error'}`,
    });
  }

  return diagnostics;
}
