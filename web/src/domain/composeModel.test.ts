import { describe, expect, it } from 'vitest';
import {
  composeYamlToForm,
  createDefaultService,
  formToComposeYaml,
} from './composeModel';

describe('composeModel - GPU Acceleration', () => {
  it('initializes default service with GPU acceleration disabled', () => {
    const svc = createDefaultService('test-app');
    expect(svc.gpuEnabled).toBe(false);
    expect(svc.gpuCount).toBe('all');
    expect(svc.gpuDeviceIds).toBe('');
  });

  it('parses deploy.resources.reservations.devices with count: all', () => {
    const yaml = `services:
  ollama:
    image: ollama/ollama:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
`;
    const form = composeYamlToForm(yaml, 'ollama', 'Ollama AI');
    expect(form.services).toHaveLength(1);
    const svc = form.services[0];
    expect(svc.gpuEnabled).toBe(true);
    expect(svc.gpuCount).toBe('all');
    expect(svc.gpuDeviceIds).toBe('');
  });

  it('parses deploy.resources.reservations.devices with specific count and device_ids', () => {
    const yaml = `services:
  whisper:
    image: onnxruntime:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0", "1"]
              capabilities: [gpu]
`;
    const form = composeYamlToForm(yaml, 'whisper', 'Whisper');
    expect(form.services).toHaveLength(1);
    const svc = form.services[0];
    expect(svc.gpuEnabled).toBe(true);
    expect(svc.gpuCount).toBe('custom');
    expect(svc.gpuDeviceIds).toBe('0, 1');
  });

  it('parses runtime: nvidia as gpuEnabled', () => {
    const yaml = `services:
  cuda-app:
    image: nvidia/cuda:12.4.1-base
    runtime: nvidia
`;
    const form = composeYamlToForm(yaml, 'cuda-app', 'CUDA App');
    expect(form.services[0].gpuEnabled).toBe(true);
  });

  it('serializes form with GPU acceleration to deploy.resources.reservations.devices', () => {
    const form = composeYamlToForm(`services:
  web:
    image: my-ai-model:latest
`, 'ai-app', 'AI App');

    form.services[0].gpuEnabled = true;
    form.services[0].gpuCount = 'all';

    const outputYaml = formToComposeYaml(form);
    expect(outputYaml).toContain('driver: nvidia');
    expect(outputYaml).toContain('count: all');
    expect(outputYaml).toContain('- gpu');

    // Round-trip test
    const reloaded = composeYamlToForm(outputYaml, 'ai-app', 'AI App');
    expect(reloaded.services[0].gpuEnabled).toBe(true);
    expect(reloaded.services[0].gpuCount).toBe('all');
  });

  it('serializes custom device IDs correctly', () => {
    const form = composeYamlToForm(`services:
  web:
    image: my-ai-model:latest
`, 'ai-app', 'AI App');

    form.services[0].gpuEnabled = true;
    form.services[0].gpuCount = 'custom';
    form.services[0].gpuDeviceIds = '0, 1';

    const outputYaml = formToComposeYaml(form);
    expect(outputYaml).toContain('driver: nvidia');
    expect(outputYaml).toContain('- "0"');
    expect(outputYaml).toContain('- "1"');

    // Round-trip test
    const reloaded = composeYamlToForm(outputYaml, 'ai-app', 'AI App');
    expect(reloaded.services[0].gpuEnabled).toBe(true);
    expect(reloaded.services[0].gpuCount).toBe('custom');
    expect(reloaded.services[0].gpuDeviceIds).toBe('0, 1');
  });
});
