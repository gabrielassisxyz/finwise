// Every state the design is checked in. A state names where its reference
// comes from in the prototype and how the app reaches the same state.
//
//   prototype  how the prototype reaches it: the value seeded into its
//              localStorage key. Absent when the state reuses another
//              state's reference through `reference`.
//   app        how the app reaches it: a path. `null` while the screen has
//              not been ported.
//   viewports  which viewports to shoot, by name. Defaults to desktop and phone; the tablet
//              has to be asked for.
//   steps      what to do once the screen is up, the same on both sides.
//              They find things by role and text, which the port keeps.
//   clip       compare only this rectangle, per viewport. Used to check a
//              part of a screen before the rest of it exists.

export const VIEWPORTS = {
  desktop: { width: 1440, height: 900 },
  phone: { width: 390, height: 844 },
  // Between the two breakpoints (820 and 1200 pixels), which the other two never reach.
  tablet: { width: 1024, height: 768 },
};

// A state is shot on these unless it lists `viewports`. The tablet is opt-in: a state that
// means "desktop and phone" keeps meaning that, whatever viewport is added later.
export const DEFAULT_VIEWPORTS = ['desktop', 'phone'];

export const viewportsOf = (state) => state.viewports ?? DEFAULT_VIEWPORTS;

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

// A pot's row on the list, opened by clicking its name.
const openPot = (name) => (page) => page.getByText(name, { exact: true }).first().click();

// A subscription's row on the list, opened by clicking its name.
const openSubscription = (name) => (page) => page.getByText(name, { exact: true }).first().click();

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
  { name: 'budget', prototype: unlocked('budget'), app: { path: '/budget' } },
  { name: 'cofrinhos', prototype: unlocked('cofrinhos'), app: { path: '/cofrinhos' } },
  { name: 'assinaturas', prototype: unlocked('assinaturas'), app: { path: '/assinaturas' } },
  { name: 'patrimonio', prototype: unlocked('patrimonio'), app: { path: '/patrimonio' } },
  { name: 'contas', prototype: unlocked('contas'), app: { path: '/contas' } },
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
  // The savings pots and the net worth. Rows are found by their text, which both sides share.
  ...[
    ['cofrinhos-encerrados', [click.button('Mostrar encerrados')]],
    ['cofrinho-abaixo-do-alvo', [openPot('RF Pets')]],
    ['cofrinho-compartilhado', [openPot('Gás e IPTU')]],
    ['cofrinho-sem-movimentos', [openPot('Compras')]],
    ['cofrinho-investimento-rendimento', [openPot('Investimento 1'), click.button(/Novembro/)]],
    ['cofrinho-encerrado', [click.button('Mostrar encerrados'), openPot('Investimento 0')]],
  ].map(([name, steps]) => ({ name, prototype: unlocked('cofrinhos'), app: { path: '/cofrinhos' }, steps })),
  { name: 'cofrinhos-tablet', prototype: unlocked('cofrinhos'), app: { path: '/cofrinhos' }, viewports: ['tablet'] },
  {
    name: 'cofrinhos-encerrados-tablet',
    prototype: unlocked('cofrinhos'),
    app: { path: '/cofrinhos' },
    viewports: ['tablet'],
    steps: [click.button('Mostrar encerrados')],
  },
  ...[
    ['patrimonio-balde-aberto', [click.button(/^Reservado/)]],
    ['patrimonio-empilhado', [click.button('Empilhar por balde')]],
  ].map(([name, steps]) => ({ name, prototype: unlocked('patrimonio'), app: { path: '/patrimonio' }, steps })),
  // The budget. A line is opened by its name and a group by its title; the budget's edit
  // inputs are found by their labels, which both sides share.
  ...[
    ['budget-linha-aberta', [(page) => page.getByText('Delivery', { exact: true }).first().click()]],
    ['budget-grupo-recolhido', [click.button(/Variáveis/)]],
    ['budget-editando', [click.button('Editar budget')]],
    ['budget-remover-linha', [click.button('Editar budget'), click.button('Remover linha')]],
    ['budget-cenarios', [click.button('Cenários')]],
    [
      'budget-cenario-alterado',
      [click.button('Cenários'), (page) => page.getByLabel('Renda extra', { exact: true }).fill('0'), (page) => page.keyboard.press('Tab')],
    ],
    ['budget-mes-fechado', [click.button('Mês anterior')]],
    ['budget-estabelecimento-no-inicio', [(page) => page.getByText('Delivery', { exact: true }).first().click(), click.button(/^iFood/)]],
  ].map(([name, steps]) => ({ name, prototype: unlocked('budget'), app: { path: '/budget' }, steps })),
  // The subscriptions: the list and its menus, the tab of recurring purchases, and a subscription's
  // page. A pill is found by its status and caret, a row by its name.
  ...[
    ['assinaturas-agrupado', [click.button('Agrupar por status')]],
    ['assinaturas-menu-status', [click.button(/^Manter\s*▾$/)]],
    ['assinaturas-menu-nova', [click.button('Nova assinatura')]],
    ['assinaturas-sugestao-aceita', [(page) => page.getByRole('button', { name: 'Sim', exact: true }).click()]],
    ['assinaturas-recorrentes', [(page) => page.getByRole('button', { name: 'Recorrentes', exact: true }).click()]],
    ['assinatura-mensal', [openSubscription('Spotify')]],
    ['assinatura-moeda-estrangeira', [openSubscription('Hospedagem'), click.button(/US\$ 10,50/)]],
  ].map(([name, steps]) => ({ name, prototype: unlocked('assinaturas'), app: { path: '/assinaturas' }, steps })),
  { name: 'assinaturas-tablet', prototype: unlocked('assinaturas'), app: { path: '/assinaturas' }, viewports: ['tablet'] },
  // The accounts, the card and the settings. An account is opened from the list by its bank's name.
  ...[
    ['contas-conta-itau', [(page) => page.getByText('Itaú', { exact: true }).first().click()]],
    ['contas-conta-parada', [(page) => page.getByText('Nubank', { exact: true }).first().click()]],
  ].map(([name, steps]) => ({ name, prototype: unlocked('contas'), app: { path: '/contas' }, steps })),
];
