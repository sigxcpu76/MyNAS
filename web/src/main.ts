import { createApp } from 'vue';
import App from './ui/App.vue';
import './ui/styles.css';
import { initRouter } from './ui/router';
import { initTheme } from './ui/theme';
import { initFontSize } from './ui/fontSize';

try {
  if (typeof document !== 'undefined') {
    (document.documentElement.style as any).zoom = '';
    document.documentElement.removeAttribute('data-ui-scale');
    document.documentElement.removeAttribute('data-ui-scale-factor');
  }
  if (typeof window !== 'undefined' && window.localStorage) {
    window.localStorage.removeItem('nas_ui_scale_mode_v1');
  }
} catch {}

initTheme();
initFontSize();
initRouter();

const app = createApp(App);
app.config.errorHandler = (err, instance, info) => {
  console.error('[Vue Global Error]', err, info);
};
app.mount('#app');

