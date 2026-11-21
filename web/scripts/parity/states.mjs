// Every state the design is checked in. A state names where its reference
// comes from in the prototype and how the app reaches the same state.
//
//   prototype  how the prototype reaches it: the value seeded into its
//              localStorage key, plus optional steps. Absent when the state
//              reuses another state's reference through `reference`.
//   app        how the app reaches it: a path, optional steps. `null` while
//              the screen has not been ported.
//   clip       compare only this rectangle, per viewport. Used to check a
//              part of a screen before the rest of it exists.

export const VIEWPORTS = {
  desktop: { width: 1440, height: 900 },
  phone: { width: 390, height: 844 },
};

export const PROTOTYPE_STATE_ITEM = 'finwise-proto-v1';
export const PROTOTYPE_PIN = '123456';

const unlocked = (screen, extra = {}) => ({ seed: { locked: false, screen, ...extra } });

export const STATES = [
  { name: 'pin', prototype: { seed: { locked: true, screen: 'inicio' } }, app: { path: '/', locked: true } },
  { name: 'inicio', prototype: unlocked('inicio'), app: null },
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
