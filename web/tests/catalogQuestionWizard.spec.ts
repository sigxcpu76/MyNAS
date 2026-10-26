import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import CatalogQuestionWizard from '../src/ui/components/CatalogQuestionWizard.vue';
import type { CatalogApp } from '../src/domain/models';

describe('CatalogQuestionWizard Component', () => {
  const sampleApp: CatalogApp = {
    appId: 'actual-budget',
    title: 'Actual Budget',
    description: 'Privacy-first personal finance app',
    category: 'Finance',
    categories: ['Finance'],
    iconUrl: 'https://actualbudget.org/img/logo.png',
    home: 'https://actualbudget.org',
    train: 'community',
    version: '1.0.10',
    appVersion: '24.10.0',
    tags: ['finance'],
    containerImage: 'ghcr.io/actualbudget/actual-server:latest',
    defaultPort: 5006,
    questionGroups: [
      {
        name: 'Network & Web Portal',
        description: 'Configure network ports',
        questions: [
          {
            variable: 'network.web_port',
            label: 'Web Port',
            description: 'Listening web port',
            type: 'int',
            defaultValue: '5006',
            required: true,
            group: 'Network & Web Portal',
          },
        ],
      },
      {
        name: 'Persistent Storage & Datasets',
        description: 'Configure storage mount',
        questions: [
          {
            variable: 'storage.data',
            label: 'Data Storage',
            description: 'Main storage directory',
            type: 'hostpath',
            defaultValue: '/nas/tank/apps/actual-budget/data',
            required: true,
            group: 'Persistent Storage & Datasets',
            mountPath: '/data',
          },
        ],
      },
    ],
  };

  it('renders app title, category tabs, and initial questions', () => {
    const wrapper = mount(CatalogQuestionWizard, {
      props: {
        app: sampleApp,
        appsPool: 'tank',
        isDeploying: false,
      },
      global: {
        stubs: {
          CodeMirrorEditor: {
            template: '<div class="stubbed-codemirror">{{ modelValue }}</div>',
            props: ['modelValue'],
          },
          PathSelector: {
            template: '<input class="stubbed-path-selector" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
            props: ['modelValue'],
          },
        },
      },
    });

    expect(wrapper.text()).toContain('Actual Budget');
    expect(wrapper.text()).toContain('community');
    expect(wrapper.text()).toContain('Network & Web Portal');
    expect(wrapper.text()).toContain('Persistent Storage & Datasets');
    expect(wrapper.text()).toContain('Web Port');
  });

  it('switches tabs and displays questions in selected category', async () => {
    const wrapper = mount(CatalogQuestionWizard, {
      props: {
        app: sampleApp,
        appsPool: 'tank',
        isDeploying: false,
      },
      global: {
        stubs: {
          CodeMirrorEditor: {
            template: '<div class="stubbed-codemirror">{{ modelValue }}</div>',
            props: ['modelValue'],
          },
          PathSelector: {
            template: '<input class="stubbed-path-selector" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
            props: ['modelValue'],
          },
        },
      },
    });

    const tabs = wrapper.findAll('.nav-tab-btn');
    const storageTab = tabs.find((t) => t.text().includes('Persistent Storage & Datasets'));
    expect(storageTab).toBeDefined();

    await storageTab!.trigger('click');
    expect(wrapper.text()).toContain('Data Storage');
  });

  it('emits openInEditor with compiled compose YAML when clicking Customize in Editor', async () => {
    const wrapper = mount(CatalogQuestionWizard, {
      props: {
        app: sampleApp,
        appsPool: 'tank',
        isDeploying: false,
      },
      global: {
        stubs: {
          CodeMirrorEditor: {
            template: '<div class="stubbed-codemirror">{{ modelValue }}</div>',
            props: ['modelValue'],
          },
          PathSelector: {
            template: '<input class="stubbed-path-selector" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
            props: ['modelValue'],
          },
        },
      },
    });

    const customizeBtn = wrapper.findAll('button').find((b) => b.text().includes('Customize in Editor'));
    expect(customizeBtn).toBeDefined();

    await customizeBtn!.trigger('click');
    expect(wrapper.emitted('openInEditor')).toBeTruthy();
    const emittedData = wrapper.emitted('openInEditor')![0][0] as any;
    expect(emittedData.appId).toBe('actual-budget');
    expect(emittedData.composeYaml).toContain('x-portals:');
    expect(emittedData.composeYaml).toContain('services:');
  });

  it('emits deploy when clicking Deploy App Now', async () => {
    const wrapper = mount(CatalogQuestionWizard, {
      props: {
        app: sampleApp,
        appsPool: 'tank',
        isDeploying: false,
      },
      global: {
        stubs: {
          CodeMirrorEditor: {
            template: '<div class="stubbed-codemirror">{{ modelValue }}</div>',
            props: ['modelValue'],
          },
          PathSelector: {
            template: '<input class="stubbed-path-selector" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
            props: ['modelValue'],
          },
        },
      },
    });

    const deployBtn = wrapper.findAll('button').find((b) => b.text().includes('Deploy App Now'));
    expect(deployBtn).toBeDefined();

    await deployBtn!.trigger('click');
    expect(wrapper.emitted('deploy')).toBeTruthy();
    const emittedData = wrapper.emitted('deploy')![0][0] as any;
    expect(emittedData.appId).toBe('actual-budget');
  });

  it('emits skipWizard when clicking Skip Wizard', async () => {
    const wrapper = mount(CatalogQuestionWizard, {
      props: {
        app: sampleApp,
        appsPool: 'tank',
        isDeploying: false,
      },
      global: {
        stubs: {
          CodeMirrorEditor: {
            template: '<div class="stubbed-codemirror">{{ modelValue }}</div>',
            props: ['modelValue'],
          },
          PathSelector: {
            template: '<input class="stubbed-path-selector" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
            props: ['modelValue'],
          },
        },
      },
    });

    const skipBtn = wrapper.findAll('button').find((b) => b.text().includes('Skip Wizard & Open Raw YAML'));
    expect(skipBtn).toBeDefined();

    await skipBtn!.trigger('click');
    expect(wrapper.emitted('skipWizard')).toBeTruthy();
  });
});
