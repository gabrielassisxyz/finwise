import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router';
import type { NavId } from './navigation';
import ScreenPlaceholder from './screens/ScreenPlaceholder.vue';

declare module 'vue-router' {
  interface RouteMeta {
    nav: NavId;
  }
}

const routes: RouteRecordRaw[] = [
  { path: '/', component: ScreenPlaceholder, props: { title: 'Início' }, meta: { nav: 'inicio' } },
  { path: '/revisao', component: ScreenPlaceholder, props: { title: 'Revisão' }, meta: { nav: 'revisao' } },
  { path: '/budget', component: ScreenPlaceholder, props: { title: 'Budget' }, meta: { nav: 'budget' } },
  { path: '/cofrinhos', component: ScreenPlaceholder, props: { title: 'Cofrinhos' }, meta: { nav: 'cofrinhos' } },
  { path: '/assinaturas', component: ScreenPlaceholder, props: { title: 'Assinaturas' }, meta: { nav: 'assinaturas' } },
  { path: '/patrimonio', component: ScreenPlaceholder, props: { title: 'Patrimônio' }, meta: { nav: 'patrimonio' } },
  { path: '/contas', component: ScreenPlaceholder, props: { title: 'Contas' }, meta: { nav: 'contas' } },
  { path: '/config', component: ScreenPlaceholder, props: { title: 'Configuração' }, meta: { nav: 'config' } },
];

export function createAppRouter() {
  return createRouter({
    history: createWebHistory(),
    routes,
    scrollBehavior: () => ({ top: 0 }),
  });
}
