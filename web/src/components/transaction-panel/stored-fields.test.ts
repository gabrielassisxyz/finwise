import type { DOMWrapper, VueWrapper } from '@vue/test-utils';
import { beforeEach, describe, expect, it } from 'vitest';
import { defineComponent } from 'vue';
import type { ApiErrorKind, RefusalReason } from '@/api/contract/errors';
import { READ_ONLY_MESSAGE, REFUSAL_SENTENCES, SAVE_FAILED_MESSAGE, SAVE_UNCONFIRMED_MESSAGE } from '@/api/failures';
import { createFakeApi } from '@/api/fake';
import type { FakeState, FakeTransaction } from '@/api/fake/model';
import { prototypeState } from '@/api/fake/prototype-data';
import type { FinwiseApi, TransactionDetail } from '@/api/types';
import { usePanelStore } from '@/stores/panel';
import { mark, shownOnce, since, storedButUnanswered } from '@/test/failedSave';
import { failingOne, mountWithFailures, readOnlyApi, type ApiMethodName } from '@/test/readOnly';
import TransactionPanel from './TransactionPanel.vue';

// A field of the panel that saves when it is changed shows what the app holds: when its own write is refused or
// fails, the text typed into it goes, whatever the kind of failure, and the toast of the failure is the one it
// already was. The editor of the establishment, which has an edit mode, keeps its draft, and so does the tag box.

const PanelOnly = defineComponent({ components: { TransactionPanel }, template: '<TransactionPanel />' });

beforeEach(() => {
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});

type Setup = (transaction: FakeTransaction, state: FakeState) => void;

async function mountPanel(id: string, wrap: (api: FinwiseApi) => FinwiseApi, setup: Setup = () => undefined) {
  const state = prototypeState();
  setup(state.transactions.find((transaction) => transaction.id === id)!, state);
  const api = wrap(createFakeApi({ startLocked: false, state }));
  const mounted = await mountWithFailures(PanelOnly, [{ path: '/', component: { template: '<div />' } }], '/', {
    api,
    mounting: { attachTo: document.body },
  });
  usePanelStore(mounted.pinia).open(id);
  await mounted.settle();
  return { ...mounted, state };
}

type Mounted = Awaited<ReturnType<typeof mountPanel>>;

const failing = (method: ApiMethodName, kind: ApiErrorKind, reason?: RefusalReason) => (api: FinwiseApi) =>
  failingOne(api, method, kind, reason);

async function detailOf(mounted: Mounted, id: string): Promise<TransactionDetail> {
  const detail = await mounted.api.transactions.detail(id);
  if (!detail) throw new Error(`the stand-in has no transaction ${id}`);
  return detail;
}

const valueOf = (field: DOMWrapper<Element>): string => (field.element as HTMLInputElement).value;

function typeInto(field: DOMWrapper<Element>, text: string): void {
  (field.element as HTMLInputElement).value = text;
}

async function commit(mounted: Mounted, field: DOMWrapper<Element>, text: string): Promise<void> {
  typeInto(field, text);
  await field.trigger('change');
  await mounted.settle();
}

const person = (wrapper: VueWrapper) => wrapper.find('.panel .field');
const note = (wrapper: VueWrapper) => wrapper.find('.panel .note');
const exclusionReason = (wrapper: VueWrapper) => wrapper.find('.panel .reason');
const expected = (wrapper: VueWrapper) => wrapper.find('.panel .expected input');
const splitAmount = (index: number) => (wrapper: VueWrapper) => wrapper.findAll('.panel .value')[index]!;

const excluded: Setup = (transaction) => {
  transaction.excludedReason = 'Compra da empresa';
};
const noted: Setup = (transaction) => {
  transaction.note = 'levar o carro na sexta';
};
const reimbursable: Setup = (transaction) => {
  transaction.reimbursement = { expected: 90, link: null };
};
const split: Setup = (transaction) => {
  transaction.split = [
    { line: 'Transporte', amount: 100 },
    { line: 'Lazer', amount: 80 },
  ];
};
const asked: Setup = (transaction) => {
  transaction.askReason = true;
  transaction.reasonText = 'é do apartamento';
};
const everything: Setup = (transaction, state) => {
  noted(transaction, state);
  excluded(transaction, state);
  reimbursable(transaction, state);
};

interface Field {
  name: string;
  id: string;
  method: ApiMethodName;
  find: (wrapper: VueWrapper) => DOMWrapper<Element>;
  typed: string;
  stored: (detail: TransactionDetail) => unknown;
  /** What the field shows once the typed value was stored. */
  shown: (detail: TransactionDetail) => string;
  setup: Setup;
}

const fields: Field[] = [
  {
    name: 'the nickname of a person',
    id: 't31',
    method: 'transactions.setNickname',
    find: person,
    typed: 'Marcos',
    stored: (detail) => detail.counterparty?.nickname,
    shown: (detail) => detail.counterparty?.nickname ?? '',
    setup: () => undefined,
  },
  {
    name: 'the note',
    id: 't2',
    method: 'transactions.setNote',
    find: note,
    typed: 'abastecer na sexta',
    stored: (detail) => detail.note,
    shown: (detail) => detail.note,
    setup: noted,
  },
  {
    name: 'the reason of an exclusion',
    id: 't2',
    method: 'transactions.setExclusionReason',
    find: exclusionReason,
    typed: 'Outro motivo',
    stored: (detail) => detail.exclusion?.reason,
    shown: (detail) => detail.exclusion?.reason ?? '',
    setup: excluded,
  },
  {
    name: 'the why of a line',
    id: 't4',
    method: 'transactions.setReason',
    find: exclusionReason,
    typed: 'é da casa de praia',
    stored: (detail) => detail.reason,
    shown: (detail) => detail.reason,
    setup: asked,
  },
  {
    name: 'the expected amount',
    id: 't2',
    method: 'transactions.setExpectedReimbursement',
    find: expected,
    typed: '120',
    stored: (detail) => detail.reimbursement?.expected,
    shown: (detail) => String(detail.reimbursement?.expected ?? ''),
    setup: reimbursable,
  },
  {
    name: 'the amount of a part',
    id: 't2',
    method: 'transactions.setSplit',
    find: splitAmount(0),
    typed: '60',
    stored: (detail) => detail.splitParts,
    shown: (detail) => String(detail.splitParts?.[0]?.amount),
    setup: split,
  },
];

describe('a field whose own write failed', () => {
  it.each(fields)('shows the stored value of $name again', async (field) => {
    const mounted = await mountPanel(field.id, failing(field.method, 'internal'), field.setup);
    const shown = valueOf(field.find(mounted.wrapper));
    const stored = field.stored(await detailOf(mounted, field.id));
    const at = mark(mounted);

    await commit(mounted, field.find(mounted.wrapper), field.typed);

    expect(valueOf(field.find(mounted.wrapper))).toBe(shown);
    expect(since(mounted, at)).toEqual(shownOnce(SAVE_FAILED_MESSAGE));
    expect(field.stored(await detailOf(mounted, field.id))).toEqual(stored);
  });

  it('brings the panel of a read-only backend back to what is stored', async () => {
    const mounted = await mountPanel('t2', (api) => readOnlyApi(api));
    const at = mark(mounted);

    await commit(mounted, note(mounted.wrapper), 'abastecer na sexta');

    expect(valueOf(note(mounted.wrapper))).toBe('');
    expect(since(mounted, at)).toEqual(shownOnce(READ_ONLY_MESSAGE));
  });

  it('goes back to empty when nothing was stored', async () => {
    const mounted = await mountPanel('t2', failing('transactions.setNote', 'internal'));
    expect(valueOf(note(mounted.wrapper))).toBe('');

    await commit(mounted, note(mounted.wrapper), 'abastecer na sexta');

    expect(valueOf(note(mounted.wrapper))).toBe('');
  });

  it('goes back to empty when the expected amount was never set', async () => {
    const never: Setup = (transaction) => {
      transaction.reimbursement = { expected: null, link: null };
    };
    const mounted = await mountPanel('t2', failing('transactions.setExpectedReimbursement', 'internal'), never);
    expect(valueOf(expected(mounted.wrapper))).toBe('');

    await commit(mounted, expected(mounted.wrapper), '120');

    expect(valueOf(expected(mounted.wrapper))).toBe('');
  });

  const kinds: { kind: ApiErrorKind; reason?: RefusalReason; message: string }[] = [
    { kind: 'read_only', message: READ_ONLY_MESSAGE },
    { kind: 'refused', reason: 'not_found', message: REFUSAL_SENTENCES.not_found },
    { kind: 'internal', message: SAVE_FAILED_MESSAGE },
    { kind: 'network', message: SAVE_UNCONFIRMED_MESSAGE },
  ];

  it.each(kinds)('does so when the write is answered $kind', async ({ kind, reason, message }) => {
    const mounted = await mountPanel('t2', failing('transactions.setNote', kind, reason), noted);
    const at = mark(mounted);

    await commit(mounted, note(mounted.wrapper), 'abastecer na sexta');

    expect(valueOf(note(mounted.wrapper))).toBe('levar o carro na sexta');
    expect(since(mounted, at)).toEqual(shownOnce(message));
  });

  it('ends on the typed value when the write was stored and only its answer was lost', async () => {
    const mounted = await mountPanel('t2', (api) => storedButUnanswered(api, 'transactions.setNote'), noted);
    const at = mark(mounted);

    await commit(mounted, note(mounted.wrapper), 'abastecer na sexta');

    expect(valueOf(note(mounted.wrapper))).toBe('abastecer na sexta');
    expect((await detailOf(mounted, 't2')).note).toBe('abastecer na sexta');
    expect(since(mounted, at)).toEqual(shownOnce(SAVE_UNCONFIRMED_MESSAGE));
  });
});

describe('a field being typed in while another one saves', () => {
  interface Pair {
    name: string;
    id: string;
    setup: Setup;
    written: Field;
    meanwhile: (wrapper: VueWrapper) => DOMWrapper<Element>;
  }

  const [, theNote, theExclusion, , theExpected, theSplit] = fields as [Field, Field, Field, Field, Field, Field];

  const pairs: Pair[] = [
    { name: 'the exclusion reason beside the note', id: 't2', setup: everything, written: theNote, meanwhile: exclusionReason },
    { name: 'the note beside the exclusion reason', id: 't2', setup: everything, written: theExclusion, meanwhile: note },
    { name: 'the note beside the expected amount', id: 't2', setup: everything, written: theExpected, meanwhile: note },
    { name: 'a part beside another part', id: 't2', setup: split, written: theSplit, meanwhile: splitAmount(1) },
  ];

  it.each(pairs)('keeps the text typed in $name when the write fails', async ({ id, setup, written, meanwhile }) => {
    const mounted = await mountPanel(id, failing(written.method, 'internal'), setup);
    typeInto(meanwhile(mounted.wrapper), '17');

    await commit(mounted, written.find(mounted.wrapper), written.typed);

    expect(valueOf(meanwhile(mounted.wrapper))).toBe('17');
  });

  it.each(pairs)('keeps the text typed in $name when the write succeeds', async ({ id, setup, written, meanwhile }) => {
    const mounted = await mountPanel(id, (api) => api, setup);
    typeInto(meanwhile(mounted.wrapper), '17');

    await commit(mounted, written.find(mounted.wrapper), written.typed);

    expect(valueOf(meanwhile(mounted.wrapper))).toBe('17');
  });
});

describe('a field whose write went through', () => {
  it.each(fields)('shows what was stored in $name', async (field) => {
    const mounted = await mountPanel(field.id, (api) => api, field.setup);

    await commit(mounted, field.find(mounted.wrapper), field.typed);

    expect(valueOf(field.find(mounted.wrapper))).toBe(field.shown(await detailOf(mounted, field.id)));
  });
});

describe('the fields that are not changed by a failed save', () => {
  it('keeps the draft of the establishment editor open when its save fails', async () => {
    const mounted = await mountPanel('t2', failing('transactions.setNickname', 'internal'));
    const at = mark(mounted);
    await mounted.wrapper.find('.panel .nickname .link').trigger('click');
    const draft = mounted.wrapper.find('.panel .nickname-field');
    typeInto(draft, 'Gasolina');
    await draft.trigger('input');

    await mounted.wrapper.find('.panel .nickname .save').trigger('click');
    await mounted.settle();

    expect(valueOf(mounted.wrapper.find('.panel .nickname-field'))).toBe('Gasolina');
    expect(since(mounted, at)).toEqual(shownOnce(SAVE_FAILED_MESSAGE));
  });

  it('leaves the tag that was being added in the box when the write fails', async () => {
    const mounted = await mountPanel('t2', failing('transactions.addTag', 'internal'));
    const box = mounted.wrapper.find('.panel input[placeholder="Adicionar tag"]');
    typeInto(box, 'viagem');
    await box.trigger('input');
    const at = mark(mounted);

    await box.trigger('keydown', { key: 'Enter' });
    await mounted.settle();

    expect(valueOf(mounted.wrapper.find('.panel input[placeholder="Adicionar tag"]'))).toBe('viagem');
    expect(since(mounted, at)).toEqual(shownOnce(SAVE_FAILED_MESSAGE));
  });
});
