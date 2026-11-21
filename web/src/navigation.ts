export type NavId =
  | 'inicio'
  | 'revisao'
  | 'budget'
  | 'cofrinhos'
  | 'assinaturas'
  | 'patrimonio'
  | 'contas'
  | 'config';

export interface NavItem {
  id: NavId;
  label: string;
  to: string;
}

export const NAV_ITEMS: NavItem[] = [
  { id: 'inicio', label: 'Início', to: '/' },
  { id: 'revisao', label: 'Revisão', to: '/revisao' },
  { id: 'budget', label: 'Budget', to: '/budget' },
  { id: 'cofrinhos', label: 'Cofrinhos', to: '/cofrinhos' },
  { id: 'assinaturas', label: 'Assinaturas', to: '/assinaturas' },
  { id: 'patrimonio', label: 'Patrimônio', to: '/patrimonio' },
  { id: 'contas', label: 'Contas', to: '/contas' },
  { id: 'config', label: 'Configuração', to: '/config' },
];

// The phone's bottom bar has room for five destinations; the rest sit behind "Mais".
export const PHONE_BAR_COUNT = 5;
