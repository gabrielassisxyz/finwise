import type { DOMWrapper, VueWrapper } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import type { ApiErrorKind, RefusalReason } from '@/api/contract/errors';
import {
  READ_ONLY_MESSAGE,
  REFUSAL_SENTENCES,
  SAVE_FAILED_MESSAGE,
  SAVE_UNCONFIRMED_MESSAGE,
} from '@/api/failures';
import { createFakeApi } from '@/api/fake';
import type { FakeState } from '@/api/fake/model';
import { prototypeState } from '@/api/fake/prototype-data';
import type { FinwiseApi, SettingsOverview } from '@/api/types';
import { failingAfter, mark, shownOnce, since, storedButUnanswered } from '@/test/failedSave';
import { failingOne, mountWithFailures, readOnlyApi, type ApiMethodName, type FailureMount } from '@/test/readOnly';
import { useRevisionStore } from '@/stores/revision';
import ConfigScreen from './ConfigScreen.vue';

// A field that saves when it is changed shows what the app holds: when its own write is refused or fails, the text
// typed into it goes, whatever the kind of failure, and the toast of the failure is the one it already was.

const routes = [{ path: '/config/:section?', component: ConfigScreen }];

async function mountConfig(wrap: (api: FinwiseApi) => FinwiseApi, setup: (state: FakeState) => void = () => undefined) {
  const state = prototypeState();
  setup(state);
  const standIn = createFakeApi({ startLocked: false, state });
  const mounted = await mountWithFailures(ConfigScreen, routes, '/config', { api: wrap(standIn) });
  return { ...mounted, state, standIn };
}

type Mounted = Awaited<ReturnType<typeof mountConfig>>;

const failing = (method: ApiMethodName, kind: ApiErrorKind, reason?: RefusalReason) => (api: FinwiseApi) =>
  failingOne(api, method, kind, reason);

const nicknames = (wrapper: VueWrapper) => wrapper.find('[data-section="apelidos"]');
const establishmentNickname = (wrapper: VueWrapper) => nicknames(wrapper).findAll('.row')[1]!.find('input');
const personNickname = (wrapper: VueWrapper) => nicknames(wrapper).findAll('.person')[1]!.find('input');
const cardLabel = (wrapper: VueWrapper) => nicknames(wrapper).findAll('.person')[4]!.find('input');
const minExamples = (wrapper: VueWrapper) => wrapper.find('[data-section="categorizacao"] input[type="number"]');
const closingDay = (wrapper: VueWrapper) => wrapper.find('[aria-label="Dia de fechamento"]');
const slider = (wrapper: VueWrapper) => wrapper.find('input[type="range"]');

const valueOf = (field: DOMWrapper<Element>): string => (field.element as HTMLInputElement).value;

function typeInto(field: DOMWrapper<Element>, text: string): void {
  (field.element as HTMLInputElement).value = text;
}

async function commit(mounted: Pick<FailureMount, 'settle'>, field: DOMWrapper<Element>, text: string): Promise<void> {
  typeInto(field, text);
  await field.trigger('change');
  await mounted.settle();
}

/** The slider as a person moves it: the label follows, and the write waits for the release. */
async function release(mounted: Mounted, percent: number): Promise<void> {
  typeInto(slider(mounted.wrapper), String(percent));
  await slider(mounted.wrapper).trigger('input');
  await slider(mounted.wrapper).trigger('change');
  await mounted.settle();
}

const manual = (state: FakeState): void => {
  state.settings.closingMode = 'manual';
};

interface Field {
  name: string;
  method: ApiMethodName;
  find: (wrapper: VueWrapper) => DOMWrapper<Element>;
  typed: string;
  stored: (settings: SettingsOverview) => unknown;
  setup?: (state: FakeState) => void;
}

const fields: Field[] = [
  {
    name: 'the nickname of an establishment',
    method: 'settings.setNickname',
    find: establishmentNickname,
    typed: 'Gasolina',
    stored: (settings) => settings.establishments[1]!.nickname,
  },
  {
    name: 'the nickname of a person',
    method: 'settings.setNickname',
    find: personNickname,
    typed: 'Anita',
    stored: (settings) => settings.people[1]!.nickname,
  },
  {
    name: 'the label of the card',
    method: 'settings.setCardLabel',
    find: cardLabel,
    typed: 'Virtual',
    stored: (settings) => settings.cardLabel,
  },
  {
    name: 'the minimum of examples',
    method: 'settings.setMinExamples',
    find: minExamples,
    typed: '50',
    stored: (settings) => settings.categorization.minExamples,
  },
  {
    name: 'the closing day',
    method: 'settings.setClosing',
    find: closingDay,
    typed: '20',
    stored: (settings) => settings.closing.manualDay,
    setup: manual,
  },
];

describe('a field whose own write failed', () => {
  it.each(fields)('shows the stored value of $name again', async (field) => {
    const mounted = await mountConfig(failing(field.method, 'internal'), field.setup);
    const shown = valueOf(field.find(mounted.wrapper));
    const stored = field.stored(await mounted.api.settings.overview());
    const at = mark(mounted);

    await commit(mounted, field.find(mounted.wrapper), field.typed);

    expect(valueOf(field.find(mounted.wrapper))).toBe(shown);
    expect(since(mounted, at)).toEqual(shownOnce(SAVE_FAILED_MESSAGE));
    expect(field.stored(await mounted.api.settings.overview())).toEqual(stored);
  });

  it('shows the stored threshold again, in the label, on the chart and in the legend', async () => {
    const mounted = await mountConfig(failing('settings.setThreshold', 'internal'));
    const label = () => mounted.wrapper.find('.slider > span:nth-of-type(2)').text();
    const marker = () => mounted.wrapper.find('.marker.current').attributes('style');
    const legend = () => mounted.wrapper.find('.legend .key').text();
    const shown = { label: label(), marker: marker(), legend: legend(), slider: valueOf(slider(mounted.wrapper)) };
    const at = mark(mounted);

    await release(mounted, 60);

    expect({ label: label(), marker: marker(), legend: legend(), slider: valueOf(slider(mounted.wrapper)) }).toEqual(shown);
    expect(since(mounted, at)).toEqual(shownOnce(SAVE_FAILED_MESSAGE));
    expect(mounted.state.confidenceThreshold).toBe(85);
  });

  it('brings the screen of a read-only backend back to what is stored', async () => {
    const api = readOnlyApi(createFakeApi({ startLocked: false, state: prototypeState() }));
    const mounted = await mountWithFailures(ConfigScreen, routes, '/config', { api });
    const at = mark(mounted);

    await commit(mounted, establishmentNickname(mounted.wrapper), 'Gasolina');

    expect(valueOf(establishmentNickname(mounted.wrapper))).toBe('');
    expect(since(mounted, at)).toEqual(shownOnce(READ_ONLY_MESSAGE));
  });

  it('goes back to empty when nothing was stored', async () => {
    const mounted = await mountConfig(failing('settings.setNickname', 'internal'));
    expect(valueOf(establishmentNickname(mounted.wrapper))).toBe('');

    await commit(mounted, establishmentNickname(mounted.wrapper), 'Gasolina');

    expect(valueOf(establishmentNickname(mounted.wrapper))).toBe('');
  });

  const kinds: { kind: ApiErrorKind; reason?: RefusalReason; message: string }[] = [
    { kind: 'read_only', message: READ_ONLY_MESSAGE },
    { kind: 'refused', reason: 'not_found', message: REFUSAL_SENTENCES.not_found },
    { kind: 'internal', message: SAVE_FAILED_MESSAGE },
    { kind: 'network', message: SAVE_UNCONFIRMED_MESSAGE },
  ];

  it.each(kinds)('does so when the write is answered $kind', async ({ kind, reason, message }) => {
    const mounted = await mountConfig(failing('settings.setNickname', kind, reason));
    const at = mark(mounted);

    await commit(mounted, personNickname(mounted.wrapper), 'Anita');

    expect(valueOf(personNickname(mounted.wrapper))).toBe('Ana');
    expect(since(mounted, at)).toEqual(shownOnce(message));
  });

  it('ends on the typed value when the write was stored and only its answer was lost', async () => {
    const mounted = await mountConfig((api) => storedButUnanswered(api, 'settings.setNickname'));
    const at = mark(mounted);

    await commit(mounted, establishmentNickname(mounted.wrapper), 'Gasolina');

    expect(valueOf(establishmentNickname(mounted.wrapper))).toBe('Gasolina');
    expect((await mounted.api.settings.overview()).establishments[1]!.nickname).toBe('Gasolina');
    expect(since(mounted, at)).toEqual(shownOnce(SAVE_UNCONFIRMED_MESSAGE));
  });

  it('ends on the typed threshold when the write was stored and only its answer was lost', async () => {
    const mounted = await mountConfig((api) => storedButUnanswered(api, 'settings.setThreshold'));

    await release(mounted, 60);

    expect(valueOf(slider(mounted.wrapper))).toBe('60');
    expect(mounted.wrapper.find('.slider > span:nth-of-type(2)').text()).toBe('60');
    expect(mounted.state.confidenceThreshold).toBe(60);
  });
});

describe('a field whose stored value moved since it was drawn', () => {
  const personKey = (state: FakeState) => state.transactions.find((transaction) => transaction.person && transaction.name === 'Ana C. Ribeiro')!.establishmentKey;

  it('shows the value its last save stored when a later save fails', async () => {
    const mounted = await mountConfig((api) => failingAfter(api, 'settings.setNickname', 'internal', 1));
    await commit(mounted, personNickname(mounted.wrapper), 'Anita');
    expect(valueOf(personNickname(mounted.wrapper))).toBe('Anita');
    const at = mark(mounted);

    await commit(mounted, personNickname(mounted.wrapper), 'Anitinha');

    expect(valueOf(personNickname(mounted.wrapper))).toBe('Anita');
    expect(since(mounted, at)).toEqual(shownOnce(SAVE_FAILED_MESSAGE));
    expect((await mounted.api.settings.overview()).people[1]!.nickname).toBe('Anita');
  });

  it('shows the value a fetch brought when a later save fails', async () => {
    const mounted = await mountConfig(failing('settings.setNickname', 'internal'));
    await mounted.standIn.settings.setNickname(personKey(mounted.state), 'Bia');
    useRevisionStore(mounted.pinia).bump();
    await mounted.settle();
    expect(valueOf(personNickname(mounted.wrapper))).toBe('Bia');

    await commit(mounted, personNickname(mounted.wrapper), 'Anitinha');

    expect(valueOf(personNickname(mounted.wrapper))).toBe('Bia');
  });
});

describe('a field being typed in while another one saves', () => {
  interface Pair {
    name: string;
    written: Field;
    meanwhile: (wrapper: VueWrapper) => DOMWrapper<Element>;
  }

  const pairs: Pair[] = [
    { name: 'a nickname beside another', written: fields[0]!, meanwhile: personNickname },
    { name: 'the minimum of examples beside a nickname', written: fields[0]!, meanwhile: minExamples },
    { name: 'the closing day beside the card label', written: fields[2]!, meanwhile: closingDay },
    { name: 'the minimum of examples beside the closing day', written: fields[4]!, meanwhile: minExamples },
  ];

  it.each(pairs)('keeps the text typed in $name when the write fails', async ({ written, meanwhile }) => {
    const mounted = await mountConfig(failing(written.method, 'internal'), manual);
    typeInto(meanwhile(mounted.wrapper), '17');

    await commit(mounted, written.find(mounted.wrapper), written.typed);

    expect(valueOf(meanwhile(mounted.wrapper))).toBe('17');
  });

  it.each(pairs)('keeps the text typed in $name when the write succeeds', async ({ written, meanwhile }) => {
    const mounted = await mountConfig((api) => api, manual);
    typeInto(meanwhile(mounted.wrapper), '17');

    await commit(mounted, written.find(mounted.wrapper), written.typed);

    expect(valueOf(meanwhile(mounted.wrapper))).toBe('17');
  });

  it('keeps the minimum of examples being typed when the threshold write fails', async () => {
    const mounted = await mountConfig(failing('settings.setThreshold', 'internal'));
    typeInto(minExamples(mounted.wrapper), '17');

    await release(mounted, 60);

    expect(valueOf(minExamples(mounted.wrapper))).toBe('17');
  });
});

describe('a field whose write went through', () => {
  it.each(fields)('shows what was stored in $name', async (field) => {
    const mounted = await mountConfig((api) => api, field.setup);

    await commit(mounted, field.find(mounted.wrapper), field.typed);

    expect(valueOf(field.find(mounted.wrapper))).toBe(String(field.stored(await mounted.api.settings.overview()) ?? ''));
  });
});
