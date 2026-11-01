import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

function typographyScalePlugin() {
  return {
    name: 'typography-scale-plugin',
    enforce: 'pre' as const,
    transform(code: string, id: string) {
      if (id.includes('node_modules')) return;
      if (!id.endsWith('.vue') && !id.endsWith('.css')) return;

      const transformed = code.replace(
        /font-size:\s*([0-9]+(?:\.[0-9]+)?)px(?!\s*\*)/g,
        'font-size: calc($1px * var(--fs-scale, 1))'
      );
      return {
        code: transformed,
        map: null,
      };
    },
  };
}

export default defineConfig({
  plugins: [typographyScalePlugin(), vue()],
  build: {
    target: 'esnext',
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/echarts') || id.includes('node_modules/zrender')) {
            return 'vendor-echarts';
          }
          if (id.includes('node_modules/@xterm') || id.includes('node_modules/@novnc')) {
            return 'vendor-term';
          }
          if (id.includes('node_modules/@codemirror') || id.includes('node_modules/codemirror')) {
            return 'vendor-codemirror';
          }
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/v1': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
      },
      '/dav': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
  },
});
