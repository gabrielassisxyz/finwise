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
  ].map(([name, steps]) => ({ name, prototype: unlocked('inicio'), app: { path: '/' }, steps })),
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
