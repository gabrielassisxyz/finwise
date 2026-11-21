// Every state the design is checked in. A state names where its reference
// comes from in the prototype and how the app reaches the same state.
//
//   prototype  how the prototype reaches it: the value seeded into its
//              localStorage key. Absent when the state reuses another
//              state's reference through `reference`.
//   app        how the app reaches it: a path. `null` while the screen has
//              not been ported.
//   steps      what to do once the screen is up, the same on both sides.
//              They find things by role and text, which the port keeps.
//   clip       compare only this rectangle, per viewport. Used to check a
//              part of a screen before the rest of it exists.

export const VIEWPORTS = {
  desktop: { width: 1440, height: 900 },
  phone: { width: 390, height: 844 },
};

export const PROTOTYPE_STATE_ITEM = 'finwise-proto-v1';
export const PROTOTYPE_PIN = '123456';

const unlocked = (screen, extra = {}) => ({ seed: { locked: false, screen, ...extra } });

const click = {
  button: (name) => (page) => page.getByRole('button', { name }).first().click(),
};

// The menus of the prototype are plain buttons and the app's carry the menuitem role, so an
// option is found by its text.
const menuOption = (label, hint) => (page) =>
  page.locator('button').filter({ hasText: new RegExp(`^${label}\\s*${hint}$`) }).first().click();

const selectRow = (index) => (page) =>
  page.getByRole('checkbox', { name: 'Selecionar', exact: true }).nth(index).check();

const panelOf = (page) => page.locator('aside[data-screen-label="Painel da transação"]');

const openTransaction = (label, which = 'first') => async (page) => {
  const rows = page.getByText(label, { exact: true }).filter({ visible: true });
  await (which === 'last' ? rows.last() : rows.first()).click();
  await panelOf(page).waitFor();
};

const inPanel = {
  button: (name) => (page) => panelOf(page).getByRole('button', { name, exact: true }).click(),
  switch: (name) => (page) => panelOf(page).getByRole('switch', { name }).click(),
};

export const STATES = [
  { name: 'pin', prototype: { seed: { locked: true, screen: 'inicio' } }, app: { path: '/', locked: true } },
  { name: 'inicio', prototype: unlocked('inicio'), app: { path: '/' } },
  ...[
    ['inicio-aba-cartao', [click.button(/^Cartão\s*\d+$/)]],
    ['inicio-aba-outros', [click.button(/^Outros\s*\d+$/)]],
    ['inicio-so-pendentes', [click.button('Só pendentes')]],
    ['inicio-menu-linha', [click.button('Linha do budget ▾')]],
    ['inicio-menu-semanas', [click.button(/^Esta semana/)]],
    ['inicio-iof', [(page, viewport) => click.button(viewport === 'phone' ? /^IOF/ : 'Mostrar IOF')(page)]],
    ['inicio-busca-sem-resultado', [(page) => page.getByPlaceholder(/^Buscar por descrição/).fill('zzzz')]],
    ['inicio-mes-fechado', [click.button('Mês anterior')]],
    ['inicio-mes-vazio', [click.button('Próximo mês')]],
    ['inicio-linha-alterada', [click.button(/^Lazer\s*▾$/), menuOption('Delivery', 'Variáveis')]],
  ].map(([name, steps]) => ({ name, prototype: unlocked('inicio'), app: { path: '/' }, steps })),
  // A phone card has no checkbox, so the selection exists on the wide layout only.
  ...[
    ['inicio-selecao', [selectRow(0), selectRow(1)]],
    ['inicio-selecao-tag', [selectRow(0), selectRow(1), click.button('Adicionar tag')]],
  ].map(([name, steps]) => ({
    name,
    prototype: unlocked('inicio'),
    app: { path: '/' },
    viewports: ['desktop'],
    steps,
  })),
  // The transaction panel opens over the home screen from a row (a card on a phone), found by the
  // name it shows. Inside it a target is found by role and name, the same on both sides.
  ...[
    ['painel-compra-cartao', 'Posto Shell'],
    ['painel-ia', 'Pão de Açúcar'],
    ['painel-pendente', 'Bar do Zé'],
    ['painel-internacional', 'Serviço de nuvem'],
    ['painel-estorno', 'Estorno de anuidade'],
    ['painel-pix-pessoa', 'Marcos A. Lima'],
    ['painel-pix-recebido', 'Ana'],
    ['painel-movimentacao', 'Pagamento da fatura'],
    ['painel-historico', 'Hortifruti', 'last'],
  ].map(([name, label, which]) => ({
    name,
    prototype: unlocked('inicio'),
    app: { path: '/' },
    steps: [openTransaction(label, which)],
  })),
  ...[
    ['painel-divisao', [inPanel.button('Dividir em linhas')]],
    ['painel-reembolsavel', [inPanel.switch('Reembolsável')]],
    ['painel-nao-conta', [inPanel.switch('Não conta')]],
    ['painel-tag', [inPanel.button('#trabalho')]],
  ].map(([name, steps]) => ({
    name,
    prototype: unlocked('inicio'),
    app: { path: '/' },
    steps: [openTransaction('Posto Shell'), ...steps],
  })),
  { name: 'revisao-semanal', prototype: unlocked('revisao', { revTab: 'semanal' }), app: null },
  { name: 'revisao-fechamento', prototype: unlocked('revisao', { revTab: 'fech' }), app: null },
  { name: 'revisao-salario', prototype: unlocked('revisao', { revTab: 'sal' }), app: null },
  { name: 'budget', prototype: unlocked('budget'), app: null },
  { name: 'cofrinhos', prototype: unlocked('cofrinhos'), app: null },
  { name: 'assinaturas', prototype: unlocked('assinaturas'), app: null },
  { name: 'patrimonio', prototype: unlocked('patrimonio'), app: null },
  { name: 'contas', prototype: unlocked('contas'), app: null },
  { name: 'config', prototype: unlocked('config'), app: null },
  {
    name: 'shell-sidebar',
    reference: 'inicio',
    viewports: ['desktop'],
    clip: { desktop: { x: 0, y: 0, width: 224, height: 900 } },
    app: { path: '/' },
  },
  {
    name: 'shell-header',
    reference: 'inicio',
    clip: {
      desktop: { x: 224, y: 0, width: 1216, height: 57 },
      phone: { x: 0, y: 0, width: 390, height: 57 },
    },
    app: { path: '/' },
  },
];
