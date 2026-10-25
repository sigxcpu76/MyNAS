import { describe, expect, it } from 'vitest';
import {
  composeYamlToForm,
  formToComposeYaml,
  createDefaultAppForm,
  createDefaultService,
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
      { hostPort: '8080', containerPort: '80', protocol: 'tcp' },
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
});
