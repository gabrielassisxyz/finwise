import '@fontsource-variable/geist';
import '@fontsource-variable/geist-mono';
import './styles/tokens.css';
import './styles/base.css';

import { createPinia } from 'pinia';
import { createApp } from 'vue';
import App from './App.vue';
import { apiKey } from './api/context';
import { createFakeApi } from './api/fake';
import { createAppRouter } from './router';

const app = createApp(App);

// There is no server yet: the app runs on the in-memory stand-in until the
// HTTP client exists to take its place here.
app.provide(apiKey, createFakeApi());
app.use(createPinia());
app.use(createAppRouter());
app.mount('#app');
