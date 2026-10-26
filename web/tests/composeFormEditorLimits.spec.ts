import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import ComposeFormEditor from '../src/ui/components/ComposeFormEditor.vue';
import {
  DEFAULT_COMPOSE_MEMORY_LIMIT,
  DEFAULT_COMPOSE_CPU_LIMIT,
  COMPOSE_MEMORY_LIMIT_OPTIONS,
  COMPOSE_CPU_LIMIT_OPTIONS,
} from '../src/domain/composeModel';

describe('ComposeFormEditor: Resource Limits Select Boxes', () => {
  it('defines correct constants for memory and CPU limit options and defaults', () => {
    expect(DEFAULT_COMPOSE_MEMORY_LIMIT).toBe('512m');
    expect(DEFAULT_COMPOSE_CPU_LIMIT).toBe('1.0');

    expect(COMPOSE_MEMORY_LIMIT_OPTIONS).toEqual([
      '128m',
      '256m',
      '512m',
      '1g',
      '2g',
      '4g',
      '8g',
      '16g',
      '32g',
    ]);

    expect(COMPOSE_CPU_LIMIT_OPTIONS).toEqual([
      '0.5',
      '1.0',
      '2',
      '4',
      '8',
      '16',
      '32',
    ]);
  });

  it('renders select boxes with default 512m and 1.0 values and all expected options', async () => {
    const wrapper = mount(ComposeFormEditor, {
      props: {
        initialAppId: 'my-app',
        initialTitle: 'My Test App',
        initialComposeYaml: '',
      },
    });

    // Switch to resources subtab
    const buttons = wrapper.findAll('button');
    const resourcesBtn = buttons.find((b) => b.text().includes('Resources & Hardware'));
    expect(resourcesBtn).toBeDefined();
    await resourcesBtn!.trigger('click');

    // Find all select inputs in resources tab
    const selects = wrapper.findAll('select.form-input');
    expect(selects.length).toBeGreaterThanOrEqual(2);

    // Find memory limit and cpu limit selects
    const memorySelect = selects[0];
    const cpuSelect = selects[1];

    // Verify default values
    expect((memorySelect.element as HTMLSelectElement).value).toBe('512m');
    expect((cpuSelect.element as HTMLSelectElement).value).toBe('1.0');

    // Verify Memory options
    const memoryOptions = memorySelect.findAll('option').map((o) => o.attributes('value'));
    expect(memoryOptions).toContain('128m');
    expect(memoryOptions).toContain('256m');
    expect(memoryOptions).toContain('512m');
    expect(memoryOptions).toContain('1g');
    expect(memoryOptions).toContain('2g');
    expect(memoryOptions).toContain('4g');
    expect(memoryOptions).toContain('8g');
    expect(memoryOptions).toContain('16g');
    expect(memoryOptions).toContain('32g');

    // Verify CPU options
    const cpuOptions = cpuSelect.findAll('option').map((o) => o.attributes('value'));
    expect(cpuOptions).toContain('0.5');
    expect(cpuOptions).toContain('1.0');
    expect(cpuOptions).toContain('2');
    expect(cpuOptions).toContain('4');
    expect(cpuOptions).toContain('8');
    expect(cpuOptions).toContain('16');
    expect(cpuOptions).toContain('32');
  });

  it('allows changing memory and CPU limit via select boxes', async () => {
    const wrapper = mount(ComposeFormEditor, {
      props: {
        initialAppId: 'my-app',
        initialTitle: 'My Test App',
        initialComposeYaml: '',
      },
    });

    // Switch to resources tab
    const buttons = wrapper.findAll('button');
    const resourcesBtn = buttons.find((b) => b.text().includes('Resources & Hardware'));
    expect(resourcesBtn).toBeDefined();
    await resourcesBtn!.trigger('click');

    const selects = wrapper.findAll('select.form-input');
    const memorySelect = selects[0];
    const cpuSelect = selects[1];

    // Select 2g and 4 CPU
    await memorySelect.setValue('2g');
    await cpuSelect.setValue('4');

    expect((memorySelect.element as HTMLSelectElement).value).toBe('2g');
    expect((cpuSelect.element as HTMLSelectElement).value).toBe('4');
  });
});
