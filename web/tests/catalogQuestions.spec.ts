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
});
