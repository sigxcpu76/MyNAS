import { describe, expect, it } from 'vitest';
import { EditorState } from '@codemirror/state';
import { EditorView } from 'codemirror';
import {
  dockerComposeCompletionSource,
  dockerComposeLinter,
} from '../src/ui/components/dockerComposeCompletions';

describe('CodeMirror 6 Docker Compose completions and linter', () => {
  it('provides root-level completions at indentation 0', () => {
    const doc = 'serv';
    const state = EditorState.create({ doc });
    const view = new EditorView({ state });

    const context: any = {
      state,
      pos: 4,
      explicit: true,
      line: { from: 0 },
      matchBefore: () => ({ from: 0, to: 4, text: 'serv' }),
    };

    const res = dockerComposeCompletionSource(context);
    expect(res).not.toBeNull();
    const labels = (res?.options || []).map((o: any) => o.label);
    expect(labels).toContain('services:');
    expect(labels).toContain('networks:');
    expect(labels).toContain('volumes:');

    view.destroy();
  });

  it('provides service-level property completions inside a service', () => {
    const doc = 'services:\n  web:\n    ';
    const state = EditorState.create({ doc });
    const view = new EditorView({ state });

    const context: any = {
      state,
      pos: doc.length,
      explicit: true,
      matchBefore: () => null,
    };

    const res = dockerComposeCompletionSource(context);
    expect(res).not.toBeNull();
    const labels = (res?.options || []).map((o: any) => o.label);
    expect(labels).toContain('image:');
    expect(labels).toContain('ports:');
    expect(labels).toContain('environment:');
    expect(labels).toContain('restart:');

    view.destroy();
  });

  it('provides restart policy completions when cursor is after restart:', () => {
    const doc = 'services:\n  web:\n    restart: ';
    const state = EditorState.create({ doc });
    const view = new EditorView({ state });

    const context: any = {
      state,
      pos: doc.length,
      explicit: true,
      matchBefore: () => null,
    };

    const res = dockerComposeCompletionSource(context);
    expect(res).not.toBeNull();
    const labels = (res?.options || []).map((o: any) => o.label);
    expect(labels).toContain('unless-stopped');
    expect(labels).toContain('always');
    expect(labels).toContain('no');

    view.destroy();
  });

  it('reports syntax error diagnostics on invalid YAML', () => {
    const invalidYaml = 'services:\n  app:\n    image: [unclosed';
    const state = EditorState.create({ doc: invalidYaml });
    const view = new EditorView({ state });

    const diagnostics = dockerComposeLinter(view);
    expect(diagnostics.length).toBeGreaterThan(0);
    expect(diagnostics[0].severity).toBe('error');

    view.destroy();
  });

  it('reports warning when services: block is missing in Compose file', () => {
    const docWithoutServices = 'version: "3.8"\nname: myapp\n';
    const state = EditorState.create({ doc: docWithoutServices });
    const view = new EditorView({ state });

    const diagnostics = dockerComposeLinter(view);
    expect(diagnostics.length).toBeGreaterThan(0);
    expect(diagnostics[0].severity).toBe('warning');
    expect(diagnostics[0].message).toContain('services:');

    view.destroy();
  });

  it('reports no errors on valid Docker Compose YAML', () => {
    const validYaml = `services:
  web:
    image: nginx:alpine
    restart: unless-stopped
    ports:
      - "80:80"
`;
    const state = EditorState.create({ doc: validYaml });
    const view = new EditorView({ state });

    const diagnostics = dockerComposeLinter(view);
    expect(diagnostics.length).toBe(0);

    view.destroy();
  });
});
