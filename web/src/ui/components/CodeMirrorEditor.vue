<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Compartment, EditorState } from '@codemirror/state';
import { EditorView, basicSetup } from 'codemirror';
import { yaml } from '@codemirror/lang-yaml';
import { oneDark } from '@codemirror/theme-one-dark';
import { defaultHighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { autocompletion } from '@codemirror/autocomplete';
import { lintGutter, linter } from '@codemirror/lint';
import { dockerComposeCompletionSource, dockerComposeLinter } from './dockerComposeCompletions';
import { isDarkMode } from '../theme';

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

// Theme-adaptive styling leveraging Aegis CSS variables
const nasTheme = EditorView.theme({
  '&': {
    height: props.height,
    minHeight: props.minHeight,
    maxHeight: '100%',
    fontSize: '13px',
    fontFamily: 'var(--font-mono, JetBrains Mono, monospace)',
    backgroundColor: 'var(--input-bg, #16161e)',
    color: 'var(--text-primary, #c0caf5)',
    borderRadius: '8px',
    border: '1px solid var(--border-strong, #3b4261)',
  },
  '.cm-content': {
    caretColor: 'var(--accent-primary, #7aa2f7)',
    padding: '12px 14px',
    lineHeight: '1.55',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: 'var(--accent-primary, #7aa2f7)',
    borderLeftWidth: '2px',
  },
  '&.cm-focused': {
    outline: 'none',
    borderColor: 'var(--accent-primary, #7aa2f7)',
    boxShadow: '0 0 0 2px var(--focus-ring, rgba(122, 162, 247, 0.25))',
  },
  '.cm-gutters': {
    backgroundColor: 'var(--bg-elevated, #24283b)',
    color: 'var(--text-muted, #787c99)',
    borderRight: '1px solid var(--border-subtle, #24283b)',
    borderRadius: '8px 0 0 8px',
  },
  '.cm-activeLine': {
    backgroundColor: 'var(--accent-bg-subtle, rgba(122, 162, 247, 0.06))',
  },
  '.cm-activeLineGutter': {
    backgroundColor: 'var(--accent-bg-subtle, rgba(122, 162, 247, 0.12))',
    color: 'var(--accent-primary, #7aa2f7)',
  },
  '.cm-scroller': {
    overflow: 'auto',
    scrollbarWidth: 'thin',
    scrollbarColor: 'var(--scrollbar-thumb) var(--scrollbar-track)',
  },
  '.cm-selectionBackground, ::selection': {
    backgroundColor: 'var(--accent-bg-subtle, rgba(122, 162, 247, 0.25)) !important',
  },
  // Tooltip & completion popup styling
  '.cm-tooltip': {
    backgroundColor: 'var(--bg-surface, #1f2335)',
    border: '1px solid var(--border-strong, #3b4261)',
    borderRadius: '6px',
    boxShadow: 'var(--modal-shadow)',
    color: 'var(--text-primary, #c0caf5)',
  },
  '.cm-tooltip-autocomplete': {
    '& > ul > li': {
      padding: '5px 9px',
      fontSize: '12.5px',
      fontFamily: 'var(--font-mono, monospace)',
    },
    '& > ul > li[aria-selected]': {
      backgroundColor: 'var(--accent-bg-subtle)',
      color: 'var(--accent-primary)',
    },
  },
  '.cm-completionDetail': {
    fontStyle: 'italic',
    color: 'var(--text-secondary)',
    marginLeft: '10px',
  },
});

const highlightCompartment = new Compartment();

function getHighlightExtension(dark: boolean) {
  return dark ? oneDark : syntaxHighlighting(defaultHighlightStyle, { fallback: true });
}

onMounted(() => {
  if (!editorContainer.value) return;

  const state = EditorState.create({
    doc: props.modelValue || '',
    extensions: [
      basicSetup,
      yaml(),
      highlightCompartment.of(getHighlightExtension(isDarkMode.value)),
      nasTheme,
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

// Dynamically reconfigure highlight style on theme mode change
watch(isDarkMode, (newDark) => {
  if (view) {
    view.dispatch({
      effects: highlightCompartment.reconfigure(getHighlightExtension(newDark)),
    });
  }
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
