import { describe, it, expect } from 'vitest';
import type { CatalogApp } from '../src/domain/models';
import {
  generateComposeFromQuestions,
  getQuestionsDefaultAnswers,
  organizeQuestionGroups,
} from '../src/domain/catalogQuestions';

describe('catalogQuestions domain helpers', () => {
  const sampleApp: CatalogApp = {
    appId: 'actual-budget',
    title: 'Actual Budget',
    description: 'Privacy-focused financial tracking and budgeting app',
    category: 'Finance',
    categories: ['Finance'],
    iconUrl: 'https://actualbudget.org/img/logo.png',
    home: 'https://actualbudget.org',
    train: 'community',
    version: '1.0.10',
    appVersion: '24.10.0',
    tags: ['finance', 'budget'],
    containerImage: 'ghcr.io/actualbudget/actual-server:latest',
    defaultPort: 5006,
    questions: [
      {
        variable: 'network.web_port',
        label: 'Web Port',
        description: 'Listening web port',
        type: 'int',
        defaultValue: '5006',
        required: true,
        group: 'Network Configuration',
      },
      {
        variable: 'storage.data',
        label: 'Data Storage',
        description: 'Main storage directory',
        type: 'hostpath',
        defaultValue: '/mnt/tank/actual-budget/data',
        required: true,
        group: 'Storage Configuration',
        mountPath: '/data',
      },
      {
        variable: 'actual_budget.password',
        label: 'Server Password',
        description: 'Administrative password',
        type: 'string',
        defaultValue: 'mysecret',
        required: false,
        group: 'Actual Budget Configuration',
      },
      {
        variable: 'run_as.user',
        label: 'User ID',
        description: 'Run as UID',
        type: 'int',
        defaultValue: '1000',
        required: false,
        group: 'User and Group Configuration',
      },
      {
        variable: 'run_as.group',
        label: 'Group ID',
        description: 'Run as GID',
        type: 'int',
        defaultValue: '1000',
        required: false,
        group: 'User and Group Configuration',
      },
    ],
  };

  it('correctly maps default answers with /mnt/', () => {
    const defaults = getQuestionsDefaultAnswers(sampleApp, 'tank');
    expect(defaults['network.web_port']).toBe('5006');
    expect(defaults['storage.data']).toBe('/mnt/tank/actual-budget/data');
    expect(defaults['actual_budget.password']).toBe('mysecret');
    expect(defaults['run_as.user']).toBe('1000');
  });

  it('organizes questions into standardized groups', () => {
    const groups = organizeQuestionGroups(sampleApp);
    expect(groups.length).toBeGreaterThan(0);
    const names = groups.map((g) => g.name);
    expect(names).toContain('Network & Web Portal');
    expect(names).toContain('Persistent Storage & Datasets');
    expect(names).toContain('Application Configuration & Credentials');
    expect(names).toContain('User Permissions & System');
  });

  it('generates valid Docker Compose descriptor with x-portals and answers applied', () => {
    const answers: Record<string, string> = {
      'network.web_port': '5050',
      'storage.data': '/nas/tank/appdata/actual-budget/custom-data',
      'actual_budget.password': 'supersecretpwd',
      'run_as.user': '1001',
      'run_as.group': '1001',
    };

    const composeYaml = generateComposeFromQuestions(sampleApp, answers, 'tank');

    expect(composeYaml).toContain('x-portals:');
    expect(composeYaml).toContain('port: 5050');
    expect(composeYaml).toContain('image: ghcr.io/actualbudget/actual-server:latest');
    expect(composeYaml).toContain('container_name: actual-budget');
    expect(composeYaml).toContain('- "5050:5006"');
    expect(composeYaml).toContain('- "/nas/tank/appdata/actual-budget/custom-data:/data"');
    expect(composeYaml).toContain('user: "1001:1001"');
    expect(composeYaml).toContain('- PASSWORD=supersecretpwd');
  });

  it('only includes web portal ports in x-portals, excluding non-portal ports like bt_port', () => {
    const qbittorrentApp: CatalogApp = {
      appId: 'qbittorrent',
      title: 'qBittorrent',
      description: 'qBittorrent client',
      category: 'Downloaders',
      categories: ['Downloaders'],
      iconUrl: '',
      home: '',
      train: 'community',
      version: '1.0.0',
      appVersion: '4.6.0',
      tags: ['torrent'],
      containerImage: 'ghcr.io/linuxserver/qbittorrent:latest',
      defaultPort: 8080,
      questions: [
        {
          variable: 'network.web_port',
          label: 'Web UI Port',
          description: 'Web UI port',
          type: 'int',
          defaultValue: '30024',
          required: true,
          group: 'Network Configuration',
        },
        {
          variable: 'network.bt_port',
          label: 'BitTorrent Port',
          description: 'BitTorrent peer port',
          type: 'int',
          defaultValue: '51413',
          required: true,
          group: 'Network Configuration',
        },
      ],
    };

    const answers = {
      'network.web_port': '30024',
      'network.bt_port': '51413',
    };

    const composeYaml = generateComposeFromQuestions(qbittorrentApp, answers, 'tank');

    // x-portals should contain 30024 (web portal) but NOT 51413 (bt_port)
    expect(composeYaml).toContain('x-portals:');
    expect(composeYaml).toContain('port: 30024');
    expect(composeYaml).not.toMatch(/x-portals:[\s\S]*port: 51413/);

    // Both ports should still be published in ports section
    expect(composeYaml).toContain('- "30024:8080"');
    expect(composeYaml).toContain('- "51413:51413"');
  });

  it('filters out TrueNAS storage plumbing settings and generates clean volume mapping for elasticsearch', () => {
    const elasticApp: CatalogApp = {
      appId: 'elastic-search',
      title: 'Elastic Search',
      description: 'Distributed search and analytics engine',
      category: 'Monitoring',
      categories: ['Monitoring'],
      iconUrl: '',
      home: 'https://elastic.co',
      train: 'stable',
      version: '1.4.27',
      appVersion: '9.5.5',
      tags: ['elasticsearch'],
      containerImage: 'docker.elastic.co/elasticsearch/elasticsearch:9.5.5',
      defaultPort: 8080,
      questions: [
        {
          variable: 'storage.data',
          label: 'Elastic Search Data Storage',
          description: 'The path to store Elastic Search Data.',
          type: 'hostpath',
          defaultValue: '/mnt/fast/apps/elastic-search/data',
          required: true,
          group: 'Persistent Storage & Datasets',
          mountPath: '/usr/share/elasticsearch/data',
        },
        // Even if raw TrueNAS setting questions were somehow present:
        {
          variable: 'storage.data.type',
          label: 'Type',
          description: 'Storage type',
          type: 'select',
          defaultValue: 'ix_volume',
          required: true,
          group: 'Persistent Storage & Datasets',
          mountPath: '/type',
        },
        {
          variable: 'storage.data.ix_volume_config.acl_enable',
          label: 'Enable ACL',
          description: 'ACL setting',
          type: 'boolean',
          defaultValue: 'false',
          required: false,
          group: 'Persistent Storage & Datasets',
          mountPath: '/acl_enable',
        },
        {
          variable: 'storage.data.ix_volume_config.dataset_name',
          label: 'Dataset Name',
          description: 'Dataset name',
          type: 'string',
          defaultValue: 'data',
          required: true,
          group: 'Persistent Storage & Datasets',
          mountPath: '/dataset_name',
        },
      ],
    };

    const answers: Record<string, string> = {
      'storage.data': '/mnt/fast/apps/elastic-search/data',
      'storage.data.type': 'ix_volume',
      'storage.data.ix_volume_config.acl_enable': 'false',
      'storage.data.ix_volume_config.dataset_name': 'data',
    };

    const composeYaml = generateComposeFromQuestions(elasticApp, answers, 'fast');

    // Should NOT contain any TrueNAS setting volumes!
    expect(composeYaml).not.toContain('ix_volume:/type');
    expect(composeYaml).not.toContain('false:/acl_enable');
    expect(composeYaml).not.toContain('data:/dataset_name');

    // Should contain the real host dataset and container mount path!
    expect(composeYaml).toContain('"/mnt/fast/apps/elastic-search/data:/usr/share/elasticsearch/data"');
  });

  it('preserves existing composeYaml and container image for CasaOS apps like ESPHome', () => {
    const casaOSEsphomeApp: CatalogApp = {
      appId: 'org.icewhale.esphome',
      title: 'ESPHome',
      description: 'Home Automation systems',
      category: 'Home',
      categories: ['home'],
      iconUrl: 'https://cdn.jsdelivr.net/gh/IceWhaleTech/CasaOS-AppStore@gh-pages/apps/org.icewhale.esphome/assets/icon.svg',
      train: 'casaos',
      version: '2026.5.1',
      appVersion: '2026.5.1',
      tags: [],
      containerImage: 'ghcr.io/esphome/esphome:2026.5.1',
      defaultPort: 6052,
      composeYaml: `x-portals:
  - name: "Web UI"
    scheme: "http"
    port: 6052
    path: "/"
name: esphome
services:
  esphome:
    container_name: esphome
    deploy:
      resources:
        reservations:
          memory: 512M
    image: ghcr.io/esphome/esphome:2026.5.1
    restart: always
    ports:
      - target: 6052
        published: "6052"
        protocol: tcp
    volumes:
      - /mnt/fast/apps/esphome/config:/config
    environment:
      ESPHOME_DASHBOARD_USE_PING: "false"
      TZ: Etc/UTC
    network_mode: host
    privileged: false
`,
      questions: [
        {
          variable: 'web_port',
          label: 'Web UI Port',
          description: 'Port mapped to host',
          type: 'int',
          defaultValue: '6052',
          required: true,
          group: 'Network & Web Portal',
        },
        {
          variable: 'storage.config',
          label: 'Storage Path (/config)',
          description: 'Host storage dataset for /config',
          type: 'hostpath',
          defaultValue: '/mnt/fast/apps/esphome/config',
          mountPath: '/config',
          required: true,
          group: 'Persistent Storage & Datasets',
        },
      ],
    };

    const answers: Record<string, string> = {
      web_port: '7052',
      'storage.config': '/mnt/fast/apps/esphome/my-config',
    };

    const resultYaml = generateComposeFromQuestions(casaOSEsphomeApp, answers, 'fast');

    // Must NOT contain dummy service or dummy image
    expect(resultYaml).not.toContain('org-icewhale-esphome:latest');
    expect(resultYaml).not.toContain('services:\n  org-icewhale-esphome:');

    // Must preserve real image and container configuration
    expect(resultYaml).toContain('image: ghcr.io/esphome/esphome:2026.5.1');
    expect(resultYaml).toContain('network_mode: host');
    expect(resultYaml).toContain('ESPHOME_DASHBOARD_USE_PING: "false"');
    expect(resultYaml).toContain('memory: 512M');

    // Must update port and storage mount
    expect(resultYaml).toContain('port: 7052');
    expect(resultYaml).toContain('/mnt/fast/apps/esphome/my-config:/config');
  });

  it('correctly handles relative mountPath like "config" and replaces single stub /data fallback', () => {
    const esphomeWithStub: CatalogApp = {
      appId: 'esphome',
      title: 'ESPHome',
      description: 'ESPHome system',
      category: 'Home Automation',
      categories: ['Home Automation'],
      iconUrl: 'https://esphome.io/icon.png',
      train: 'community',
      version: '1.2.46',
      composeYaml: `services:
  esphome:
    image: ghcr.io/esphome/esphome:2026.10.0
    container_name: esphome
    ports:
      - "36052:80"
    volumes:
      - /mnt/tank/apps/esphome:/data
`,
      questions: [
        {
          variable: 'storage.config',
          label: 'Storage Path',
          description: 'Host storage directory for /config',
          type: 'hostpath',
          defaultValue: '/mnt/fast/apps/esphome/config',
          mountPath: 'config', // Relative mount path without leading slash
          required: true,
          group: 'Persistent Storage & Datasets',
        },
      ],
    };

    const answers: Record<string, string> = {
      'storage.config': '/mnt/fast/apps/esphome/config',
    };

    const result = generateComposeFromQuestions(esphomeWithStub, answers, 'fast');
    // Must NOT have relative mount path :config
    expect(result).not.toContain(':config\n');
    expect(result).not.toContain(':config"');
    // Must have absolute mount path :/config
    expect(result).toContain('/mnt/fast/apps/esphome/config:/config');
    // Must replace the stub /data
    expect(result).not.toContain(':/data');
  });
});
