import '@fontsource-variable/geist';
import '@fontsource-variable/geist-mono';
import './styles/tokens.css';
import './styles/base.css';

import { createPinia } from 'pinia';
import { createApp } from 'vue';
import App from './App.vue';
import { apiKey } from './api/context';
import { provisionApi } from './api/provision';
import { createAppRouter } from './router';

const app = createApp(App);
app.provide(apiKey, await provisionApi());
app.use(createPinia());
app.use(createAppRouter());
app.mount('#app');
