import { createApp } from 'vue';
import App from './ui/App.vue';
import './ui/styles.css';
import { initRouter } from './ui/router';

initRouter();
createApp(App).mount('#app');
