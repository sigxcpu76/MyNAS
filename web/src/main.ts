import { createApp } from 'vue';
import App from './ui/App.vue';
import './ui/styles.css';
import { initRouter } from './ui/router';
import { initTheme } from './ui/theme';
import { initScale } from './ui/scale';

initTheme();
initScale();
initRouter();

const app = createApp(App);
app.config.errorHandler = (err, instance, info) => {
  console.error('[Vue Global Error]', err, info);
};
app.mount('#app');

