import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router';
import type { NavId } from './navigation';
import AssinaturaScreen from './screens/assinaturas/AssinaturaScreen.vue';
import AssinaturasScreen from './screens/assinaturas/AssinaturasScreen.vue';
import BudgetScreen from './screens/budget/BudgetScreen.vue';
import ScenariosScreen from './screens/budget/ScenariosScreen.vue';
import CofrinhoScreen from './screens/cofrinhos/CofrinhoScreen.vue';
import CofrinhosScreen from './screens/cofrinhos/CofrinhosScreen.vue';
import InicioScreen from './screens/inicio/InicioScreen.vue';
import PatrimonioScreen from './screens/patrimonio/PatrimonioScreen.vue';
import PaydayTab from './screens/revisao/PaydayTab.vue';
import RevisaoScreen from './screens/revisao/RevisaoScreen.vue';
import WeeklyTab from './screens/revisao/WeeklyTab.vue';
import ScreenPlaceholder from './screens/ScreenPlaceholder.vue';

declare module 'vue-router' {
  interface RouteMeta {
    nav: NavId;
  }
}

const routes: RouteRecordRaw[] = [
  { path: '/', component: InicioScreen, meta: { nav: 'inicio' } },
  {
    path: '/revisao',
    component: RevisaoScreen,
    meta: { nav: 'revisao' },
    children: [
      { path: '', component: WeeklyTab },
      { path: 'fechamento', component: ScreenPlaceholder, props: { title: 'Fechamento do mês' } },
      { path: 'salario', component: PaydayTab },
    ],
  },
  { path: '/budget', component: BudgetScreen, meta: { nav: 'budget' } },
  { path: '/budget/cenarios', component: ScenariosScreen, meta: { nav: 'budget' } },
  { path: '/cofrinhos', component: CofrinhosScreen, meta: { nav: 'cofrinhos' } },
  { path: '/cofrinhos/:id', component: CofrinhoScreen, meta: { nav: 'cofrinhos' } },
  { path: '/assinaturas', component: AssinaturasScreen, meta: { nav: 'assinaturas' } },
  { path: '/assinaturas/:id', component: AssinaturaScreen, meta: { nav: 'assinaturas' } },
  { path: '/patrimonio', component: PatrimonioScreen, meta: { nav: 'patrimonio' } },
  { path: '/contas', component: ScreenPlaceholder, props: { title: 'Contas' }, meta: { nav: 'contas' } },
  { path: '/contas/cartao', component: ScreenPlaceholder, props: { title: 'Cartão' }, meta: { nav: 'contas' } },
  { path: '/config', component: ScreenPlaceholder, props: { title: 'Configuração' }, meta: { nav: 'config' } },
];

export function createAppRouter() {
  return createRouter({
    history: createWebHistory(),
    routes,
    scrollBehavior: () => ({ top: 0 }),
  });
}
