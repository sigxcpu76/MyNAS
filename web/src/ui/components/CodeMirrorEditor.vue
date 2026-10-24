<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { EditorState } from '@codemirror/state';
import { EditorView, basicSetup } from 'codemirror';
import { yaml } from '@codemirror/lang-yaml';
import { oneDark } from '@codemirror/theme-one-dark';
import { autocompletion } from '@codemirror/autocomplete';
import { lintGutter, linter } from '@codemirror/lint';
import { dockerComposeCompletionSource, dockerComposeLinter } from './dockerComposeCompletions';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    readOnly?: boolean;
    height?: string;
    minHeight?: string;
  }>(),
  {
    placeholder: 'services:\n  app:\n    image: nginx:alpine\n    ports:\n      - "8080:80"\n',
    readOnly: false,
    height: '100%',
    minHeight: '480px',
  }
);

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void;
  (e: 'change', value: string): void;
}>();

const editorContainer = ref<HTMLDivElement | null>(null);
let view: EditorView | null = null;
let isUpdatingFromProp = false;

// Custom Theme to blend with NAS Dark Theme
const nasDarkTheme = EditorView.theme({
  '&': {
    height: props.height,
    minHeight: props.minHeight,
    maxHeight: '100%',
    fontSize: '13px',
    fontFamily: 'var(--font-mono, JetBrains Mono, monospace)',
    backgroundColor: '#070b14',
    color: '#e2e8f0',
    borderRadius: '8px',
    border: '1px solid #334155',
  },
  '.cm-content': {
    caretColor: '#38bdf8',
    padding: '12px 14px',
    lineHeight: '1.55',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: '#38bdf8',
    borderLeftWidth: '2px',
  },
  '&.cm-focused': {
    outline: 'none',
    borderColor: '#38bdf8',
    boxShadow: '0 0 0 2px rgba(56, 189, 248, 0.25)',
  },
  '.cm-gutters': {
    backgroundColor: '#0a0f1d',
    color: '#64748b',
    borderRight: '1px solid #1e293b',
    borderRadius: '8px 0 0 8px',
  },
  '.cm-activeLine': {
    backgroundColor: 'rgba(56, 189, 248, 0.06)',
  },
  '.cm-activeLineGutter': {
    backgroundColor: 'rgba(56, 189, 248, 0.12)',
    color: '#38bdf8',
  },
  '.cm-scroller': {
    overflow: 'auto',
    scrollbarWidth: 'thin',
    scrollbarColor: '#334155 #070b14',
  },
  '.cm-selectionBackground, ::selection': {
    backgroundColor: '#1e3a8a !important',
  },
  // Tooltip & completion popup styling
  '.cm-tooltip': {
    backgroundColor: '#0f172a',
    border: '1px solid #334155',
    borderRadius: '6px',
    boxShadow: '0 10px 25px -5px rgba(0, 0, 0, 0.5)',
    color: '#f8fafc',
  },
  '.cm-tooltip-autocomplete': {
    '& > ul > li': {
      padding: '5px 9px',
      fontSize: '12.5px',
      fontFamily: 'var(--font-mono, monospace)',
    },
    '& > ul > li[aria-selected]': {
      backgroundColor: 'rgba(56, 189, 248, 0.2)',
      color: '#38bdf8',
    },
  },
  '.cm-completionDetail': {
    fontStyle: 'italic',
    color: '#94a3b8',
    marginLeft: '10px',
  },
});

onMounted(() => {
  if (!editorContainer.value) return;

  const state = EditorState.create({
    doc: props.modelValue || '',
    extensions: [
      basicSetup,
      yaml(),
      oneDark,
      nasDarkTheme,
      EditorView.lineWrapping,
      EditorState.readOnly.of(props.readOnly),
      autocompletion({
        override: [dockerComposeCompletionSource],
        defaultKeymap: true,
      }),
      linter(dockerComposeLinter),
      lintGutter(),
      EditorView.updateListener.of((update) => {
        if (update.docChanged && !isUpdatingFromProp) {
          const value = update.state.doc.toString();
          emit('update:modelValue', value);
          emit('change', value);
        }
      }),
    ],
  });

  view = new EditorView({
    state,
    parent: editorContainer.value,
  });
});

// Watch for external prop updates
watch(
  () => props.modelValue,
  (newVal) => {
    if (!view) return;
    const currentVal = view.state.doc.toString();
    if (newVal !== currentVal) {
      isUpdatingFromProp = true;
      try {
        view.dispatch({
          changes: { from: 0, to: currentVal.length, insert: newVal || '' },
        });
      } finally {
        isUpdatingFromProp = false;
      }
    }
  }
);

onBeforeUnmount(() => {
  if (view) {
    view.destroy();
    view = null;
  }
});
</script>

<template>
  <div class="codemirror-wrapper" :style="{ height: props.height, minHeight: props.minHeight }">
    <div ref="editorContainer" class="codemirror-host" />
  </div>
</template>

<style scoped>
.codemirror-wrapper {
  width: 100%;
  display: flex;
  flex-direction: column;
  position: relative;
}

.codemirror-host {
  width: 100%;
  height: 100%;
  flex: 1;
  display: flex;
  flex-direction: column;
}

:deep(.cm-editor) {
  height: 100%;
  width: 100%;
  flex: 1;
}
</style>
