import { describe, expect, it } from 'vitest';
import {
  composeYamlToForm,
  formToComposeYaml,
  createDefaultAppForm,
  createDefaultService,
  escapeComposeEnvValue,
  unescapeComposeEnvValue,
} from '../src/domain/composeModel';

describe('ComposeModel: YAML <-> Form Bi-directional Conversion', () => {
  it('parses a multi-service Compose YAML with networks and volumes into structured form', () => {
    const yaml = `services:
  app:
    image: nextcloud:apache
    container_name: nextcloud-app
    restart: always
    ports:
      - "8080:80/tcp"
    environment:
      - POSTGRES_HOST=db
      - REDIS_HOST=redis
    volumes:
      - /tank/appdata/nextcloud:/var/www/html:rw
    depends_on:
      - db
      - redis
    mem_limit: 1024m
    privileged: true
  db:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: nextcloud
      POSTGRES_USER: nextcloud
    volumes:
      - db_data:/var/lib/postgresql/data
networks:
  internal:
    driver: bridge
    internal: true
volumes:
  db_data:
    driver: local
`;

    const form = composeYamlToForm(yaml, 'nextcloud', 'Nextcloud Hub');
    expect(form.appId).toBe('nextcloud');
    expect(form.title).toBe('Nextcloud Hub');
    expect(form.services).toHaveLength(2);

    // App service
    const appSvc = form.services.find((s) => s.name === 'app')!;
    expect(appSvc.image).toBe('nextcloud:apache');
    expect(appSvc.containerName).toBe('nextcloud-app');
    expect(appSvc.restart).toBe('always');
    expect(appSvc.ports).toEqual([
      { hostPort: '8080', containerPort: '80', protocol: 'tcp', publishToHost: true },
    ]);
    expect(appSvc.environment).toEqual([
      { key: 'POSTGRES_HOST', value: 'db' },
      { key: 'REDIS_HOST', value: 'redis' },
    ]);
    expect(appSvc.volumes).toEqual([
      { source: '/tank/appdata/nextcloud', target: '/var/www/html', mode: 'rw' },
    ]);
    expect(appSvc.dependsOn).toEqual(['db', 'redis']);
    expect(appSvc.memoryLimit).toBe('1024m');
    expect(appSvc.privileged).toBe(true);

    // DB service with object-style environment and named volume
    const dbSvc = form.services.find((s) => s.name === 'db')!;
    expect(dbSvc.image).toBe('postgres:16-alpine');
    expect(dbSvc.restart).toBe('unless-stopped');
    expect(dbSvc.environment).toEqual([
      { key: 'POSTGRES_DB', value: 'nextcloud' },
      { key: 'POSTGRES_USER', value: 'nextcloud' },
    ]);
    expect(dbSvc.volumes).toEqual([
      { source: 'db_data', target: '/var/lib/postgresql/data', mode: 'rw' },
    ]);

    // Top level networks & volumes
    expect(form.networks).toHaveLength(1);
    expect(form.networks[0].name).toBe('internal');
    expect(form.networks[0].internal).toBe(true);

    expect(form.volumes).toHaveLength(1);
    expect(form.volumes[0].name).toBe('db_data');
  });

  it('converts structured form back to valid YAML matching expected structure', () => {
    const form = createDefaultAppForm('testapp', 'Test App');
    form.services[0].name = 'web';
    form.services[0].image = 'nginx:alpine';
    form.services[0].ports.push({ hostPort: '80', containerPort: '80', protocol: 'tcp' });
    form.services[0].environment.push({ key: 'ENV', value: 'production' });
    form.services[0].volumes.push({ source: '/tank/web', target: '/usr/share/nginx/html', mode: 'ro' });

    const yamlOutput = formToComposeYaml(form);
    expect(yamlOutput).toContain('services:');
    expect(yamlOutput).toContain('web:');
    expect(yamlOutput).toContain('image: nginx:alpine');
    expect(yamlOutput).toContain('"80:80"');
    expect(yamlOutput).toContain('ENV=production');
    expect(yamlOutput).toContain('/tank/web:/usr/share/nginx/html:ro');

    // Round-trip parse check
    const reloaded = composeYamlToForm(yamlOutput, 'testapp');
    expect(reloaded.services).toHaveLength(1);
    expect(reloaded.services[0].name).toBe('web');
    expect(reloaded.services[0].image).toBe('nginx:alpine');
  });

  it('handles nginx-proxy-manager compose YAML correctly', () => {
    const yaml = `services:
  nginx-proxy-manager:
    image: jc21/nginx-proxy-manager:latest
    container_name: nginx-proxy-manager
    restart: unless-stopped
    ports:
      - "80:80"
      - "81:81"
      - "443:443"
    environment:
      - DISABLE_IPV6=true
    volumes:
      - /tank/appdata/nginx-proxy-manager/data:/data
      - /tank/appdata/nginx-proxy-manager/letsencrypt:/etc/letsencrypt
`;
    const form = composeYamlToForm(yaml, 'nginx-proxy-manager', 'Nginx Proxy Manager');
    expect(form.appId).toBe('nginx-proxy-manager');
    expect(form.services).toHaveLength(1);
    expect(form.services[0].name).toBe('nginx-proxy-manager');
    expect(form.services[0].image).toBe('jc21/nginx-proxy-manager:latest');
    expect(form.services[0].ports).toHaveLength(3);

    const out = formToComposeYaml(form);
    expect(out).toContain('nginx-proxy-manager:');
    expect(out).toContain('jc21/nginx-proxy-manager:latest');
  });

  it('correctly parses, stores, and generates Web UI management port from container internal ports', () => {
    const yaml = `x-nas-management-port: 81
services:
  nginx-proxy-manager:
    image: jc21/nginx-proxy-manager:latest
    ports:
      - "80:80"
      - "81:81"
      - "443:443"
`;
    const form = composeYamlToForm(yaml, 'npm');
    expect(form.managementPort).toBe('81');
    const port81 = form.services[0].ports.find((p) => p.containerPort === '81');
    expect(port81?.isManagementPort).toBe(true);

    const port80 = form.services[0].ports.find((p) => p.containerPort === '80');
    expect(port80?.isManagementPort).toBeFalsy();

    // Round-trip generates x-nas-management-port and label
    const generatedYaml = formToComposeYaml(form);
    expect(generatedYaml).toContain('x-nas-management-port: 81');
    expect(generatedYaml).toContain('nas.management.port=81');

    // Change management port to 80
    form.managementPort = '80';
    const updatedYaml = formToComposeYaml(form);
    expect(updatedYaml).toContain('x-nas-management-port: 80');
    expect(updatedYaml).toContain('nas.management.port=80');
  });

  it('correctly parses and outputs TrueNAS SCALE style x-portals', () => {
    const yaml = `x-portals:
  - name: Admin Web UI
    scheme: http
    port: 81
    path: /admin
  - name: HTTP Proxy
    scheme: http
    port: 80
    path: /
services:
  nginx-proxy-manager:
    image: jc21/nginx-proxy-manager:latest
    ports:
      - "80:80"
      - "81:81"
`;
    const form = composeYamlToForm(yaml, 'npm');
    expect(form.portals).toHaveLength(2);
    expect(form.portals?.[0]).toEqual({
      name: 'Admin Web UI',
      scheme: 'http',
      port: '81',
      path: '/admin',
    });
    expect(form.portals?.[1]).toEqual({
      name: 'HTTP Proxy',
      scheme: 'http',
      port: '80',
      path: '/',
    });
    // Default management port should fall back to first portal's port
    expect(form.managementPort).toBe('81');

    const generated = formToComposeYaml(form);
    expect(generated).toContain('x-portals:');
    expect(generated).toContain('Admin Web UI');
    expect(generated).toContain('path: /admin');
  });

  it('handles internal container ports (expose) without mapping to host', () => {
    const yaml = `services:
  db:
    image: postgres:16
    expose:
      - "5432"
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
    expose:
      - "9000/tcp"
`;
    const form = composeYamlToForm(yaml, 'stack');
    const dbSvc = form.services.find((s) => s.name === 'db')!;
    expect(dbSvc.ports).toEqual([
      { hostPort: '', containerPort: '5432', protocol: 'tcp', publishToHost: false },
    ]);

    const webSvc = form.services.find((s) => s.name === 'web')!;
    expect(webSvc.ports).toEqual([
      { hostPort: '8080', containerPort: '80', protocol: 'tcp', publishToHost: true },
      { hostPort: '', containerPort: '9000', protocol: 'tcp', publishToHost: false },
    ]);

    const generated = formToComposeYaml(form);
    expect(generated).toContain('expose:');
    expect(generated).toContain('"5432"');
    expect(generated).toContain('"9000"');
    expect(generated).toContain('"8080:80"');
  });

  it('assigns a random high port in 32000+ range when no host port is specified in mapping', () => {
    const yaml = `services:
  web:
    image: nginx:alpine
    ports:
      - "80"
      - "8080/udp"
`;
    const form = composeYamlToForm(yaml, 'autoport');
    const webSvc = form.services.find((s) => s.name === 'web')!;
    expect(webSvc.ports).toHaveLength(2);

    const port80 = webSvc.ports[0];
    expect(port80.containerPort).toBe('80');
    expect(port80.publishToHost).toBe(true);
    const hostPort80 = Number(port80.hostPort);
    expect(hostPort80).toBeGreaterThanOrEqual(32000);
    expect(hostPort80).toBeLessThanOrEqual(65000);

    const port8080 = webSvc.ports[1];
    expect(port8080.containerPort).toBe('8080');
    expect(port8080.protocol).toBe('udp');
    expect(port8080.publishToHost).toBe(true);
    const hostPort8080 = Number(port8080.hostPort);
    expect(hostPort8080).toBeGreaterThanOrEqual(32000);
    expect(hostPort8080).toBeLessThanOrEqual(65000);

    // Also verify formToComposeYaml generates high ports when hostPort is empty
    const manualForm = createDefaultAppForm('manual', 'Manual App');
    manualForm.services[0].ports.push({
      hostPort: '',
      containerPort: '9000',
      protocol: 'tcp',
      publishToHost: true,
    });
    const generatedYaml = formToComposeYaml(manualForm);
    const match = generatedYaml.match(/"(\d+):9000"/);
    expect(match).not.toBeNull();
    const assignedPort = Number(match![1]);
    expect(assignedPort).toBeGreaterThanOrEqual(32000);
    expect(assignedPort).toBeLessThanOrEqual(65000);
  });

  it('starts new services with default memoryLimit (512m) and cpuLimit (1.0)', () => {
    const svc = createDefaultService('test-service');
    expect(svc.memoryLimit).toBe('512m');
    expect(svc.cpuLimit).toBe('1.0');

    const appForm = createDefaultAppForm('testapp', 'Test App');
    expect(appForm.services[0].memoryLimit).toBe('512m');
    expect(appForm.services[0].cpuLimit).toBe('1.0');
  });

  it('serializes default 512m and 1.0 limits to YAML correctly and parses back', () => {
    const appForm = createDefaultAppForm('res-test', 'Resource Limit Test');
    appForm.services[0].image = 'redis:alpine';
    const yaml = formToComposeYaml(appForm);

    expect(yaml).toContain('mem_limit: 512m');
    expect(yaml).toContain('cpus: 1');

    const parsed = composeYamlToForm(yaml, 'res-test');
    expect(parsed.services[0].memoryLimit).toBe('512m');
    expect(parsed.services[0].cpuLimit).toBe('1.0');
  });

  it('escapes and unescapes environment variables containing dollar ($) signs', () => {
    expect(escapeComposeEnvValue('secret$123')).toBe('secret$$123');
    expect(escapeComposeEnvValue('foo$bar$baz')).toBe('foo$$bar$$baz');
    expect(escapeComposeEnvValue('already$$escaped')).toBe('already$$escaped');
    expect(escapeComposeEnvValue('$2y$10$abcdef')).toBe('$$2y$$10$$abcdef');
    expect(escapeComposeEnvValue('${PUID:-1000}')).toBe('${PUID:-1000}');
    expect(escapeComposeEnvValue('P@$$w0rd!')).toBe('P@$$w0rd!');

    expect(unescapeComposeEnvValue('secret$$123')).toBe('secret$123');
    expect(unescapeComposeEnvValue('$$2y$$10$$abcdef')).toBe('$2y$10$abcdef');
    expect(unescapeComposeEnvValue('${PUID:-1000}')).toBe('${PUID:-1000}');
  });

  it('preserves environment values with dollars during formToComposeYaml and composeYamlToForm', () => {
    const appForm = createDefaultAppForm('env-dollar-test', 'Env Dollar Test');
    appForm.services[0].environment = [
      { key: 'POSTGRES_PASSWORD', value: 'my$secret$pass' },
      { key: 'BCRYPT_HASH', value: '$2y$10$abcdef' },
      { key: 'COMPOSE_VAR', value: '${PUID:-1000}' },
    ];

    const yaml = formToComposeYaml(appForm);
    expect(yaml).toContain('POSTGRES_PASSWORD=my$$secret$$pass');
    expect(yaml).toContain('BCRYPT_HASH=$$2y$$10$$abcdef');
    expect(yaml).toContain('COMPOSE_VAR=${PUID:-1000}');

    // Load back into form
    const loadedForm = composeYamlToForm(yaml, 'env-dollar-test');
    const envs = loadedForm.services[0].environment;
    expect(envs.find((e) => e.key === 'POSTGRES_PASSWORD')?.value).toBe('my$secret$pass');
    expect(envs.find((e) => e.key === 'BCRYPT_HASH')?.value).toBe('$2y$10$abcdef');
    expect(envs.find((e) => e.key === 'COMPOSE_VAR')?.value).toBe('${PUID:-1000}');
  });

  it('cleans ports and volumes with surrounding quotes without escaping them to backslash-quotes', () => {
    // Simulating input where ports or volumes had escaped or surrounding quotes
    const yamlWithQuotes = `services:
  esphome:
    image: ghcr.io/esphome/esphome:2026.10.0
    ports:
      - "\\\"36052:80\\\""
    volumes:
      - "\\\"/mnt/tank/apps/esphome:/data\\\""
`;
    const form = composeYamlToForm(yamlWithQuotes, 'esphome', 'ESPHome');
    expect(form.services[0].ports[0].hostPort).toBe('36052');
    expect(form.services[0].ports[0].containerPort).toBe('80');
    expect(form.services[0].volumes[0].source).toBe('/mnt/tank/apps/esphome');
    expect(form.services[0].volumes[0].target).toBe('/data');

    const generatedYaml = formToComposeYaml(form);
    expect(generatedYaml).not.toContain('\\"');
    expect(generatedYaml).toContain('"36052:80"');
    expect(generatedYaml).toContain('/mnt/tank/apps/esphome:/data');
  });

  it('preserves host IP binding in ports during round-trip', () => {
    const yaml = `services:
  web:
    image: nginx:alpine
    ports:
      - "127.0.0.1:8080:80"
`;
    const form = composeYamlToForm(yaml, 'web');
    expect(form.services[0].ports[0].hostIp).toBe('127.0.0.1');
    expect(form.services[0].ports[0].hostPort).toBe('8080');
    expect(form.services[0].ports[0].containerPort).toBe('80');

    const generated = formToComposeYaml(form);
    expect(generated).toContain('"127.0.0.1:8080:80"');
  });

  it('preserves custom fields like labels, healthcheck, cap_add, user, and shm_size on round-trip', () => {
    const yaml = `services:
  app:
    image: custom-app:latest
    user: "1000:1000"
    shm_size: 2gb
    cap_add:
      - NET_ADMIN
    healthcheck:
      test:
        - CMD
        - curl
        - -f
        - http://localhost
      interval: 30s
    labels:
      - traefik.enable=true
      - homepage.group=Media
`;
    const form = composeYamlToForm(yaml, 'app');
    expect(form.services[0].customFields?.user).toBe('1000:1000');
    expect(form.services[0].customFields?.shm_size).toBe('2gb');
    expect(form.services[0].customFields?.cap_add).toEqual(['NET_ADMIN']);
    expect(form.services[0].customFields?.healthcheck).toBeDefined();

    const generated = formToComposeYaml(form);
    expect(generated).toContain('user: 1000:1000');
    expect(generated).toContain('shm_size: 2gb');
    expect(generated).toContain('NET_ADMIN');
    expect(generated).toContain('traefik.enable=true');
    expect(generated).toContain('homepage.group=Media');
  });

  it('parses inline configs (ConfigMaps) and maps them into service config mounts and top-level configs', () => {
    const yaml = `services:
  web:
    image: nginx:alpine
    configs:
      - source: nginx_conf
        target: /etc/nginx/conf.d/default.conf
    volumes:
      - /tank/html:/usr/share/nginx/html:ro

configs:
  nginx_conf:
    content: |
      server {
        listen 80;
        server_name example.com;
      }
`;
    const form = composeYamlToForm(yaml, 'web-app', 'Web App');
    expect(form.configs).toHaveLength(1);
    expect(form.configs[0].name).toBe('nginx_conf');
    expect(form.configs[0].content).toContain('listen 80;');
    expect(form.configs[0].content).toContain('server_name example.com;');

    expect(form.services[0].volumes).toHaveLength(2);
    // Regular volume
    const regVol = form.services[0].volumes.find((v) => v.type !== 'config')!;
    expect(regVol.source).toBe('/tank/html');
    expect(regVol.target).toBe('/usr/share/nginx/html');

    // Config mount
    const cfgVol = form.services[0].volumes.find((v) => v.type === 'config')!;
    expect(cfgVol.source).toBe('nginx_conf');
    expect(cfgVol.target).toBe('/etc/nginx/conf.d/default.conf');
    expect(cfgVol.mode).toBe('ro');

    // Bi-directional generation back to YAML
    const regenerated = formToComposeYaml(form);
    expect(regenerated).toContain('configs:');
    expect(regenerated).toContain('source: nginx_conf');
    expect(regenerated).toContain('target: /etc/nginx/conf.d/default.conf');
    expect(regenerated).toContain('listen 80;');
    expect(regenerated).toContain('/tank/html:/usr/share/nginx/html:ro');
  });

  it('throws syntax error when parsing malformed YAML', () => {
    const badYaml = `services:
  app:
    image: [unclosed
`;
    expect(() => composeYamlToForm(badYaml, 'bad')).toThrow(/YAML syntax error/);
  });

  it('sets default container hostname and preserves explicit hostname overrides', () => {
    const form = createDefaultAppForm('jellyfin', 'Jellyfin');
    form.services[0].name = 'app';
    form.services[0].image = 'jellyfin/jellyfin:latest';

    const generatedSingle = formToComposeYaml(form);
    expect(generatedSingle).toContain('hostname: jellyfin');

    // Explicit hostname override
    form.services[0].hostname = 'custom-jellyfin-host';
    const generatedCustom = formToComposeYaml(form);
    expect(generatedCustom).toContain('hostname: custom-jellyfin-host');

    const parsed = composeYamlToForm(generatedCustom, 'jellyfin', 'Jellyfin');
    expect(parsed.services[0].hostname).toBe('custom-jellyfin-host');
  });
});
