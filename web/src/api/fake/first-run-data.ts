import type { TransactionKind } from '../types';
import type { FakeAccount, FakeCard, FakeSavingsPot, FakeSettings, FakeState, FakeTransaction } from './model';

// The two days the stand-in can start on besides the prototype's year of use.
// The fresh install is what a read-only backend knows on its first day: what
// the bank reported and nothing a person has authored. The empty ledger is the
// moment before the first synchronisation. Every name and figure is invented.

const TODAY = '2026-11-28';
const CURRENT_MONTH = '2026-11';
const CARD_ACCOUNT = 'C6 Carbon';
const MERCADO_PAGO = 'Mercado Pago';

/** The groups of a budget nobody has filled in: the places its lines will go. */
const EMPTY_BUDGET: FakeState['budget'] = [
  { key: 'rec', name: 'Receita', lines: [] },
  { key: 'fix', name: 'Fixos e essenciais', lines: [] },
  { key: 'var', name: 'Variáveis', lines: [] },
  { key: 'prov', name: 'Provisões', lines: [] },
  { key: 'res', name: 'Reservas de desejo', lines: [] },
];

function bank(
  sequence: number,
  day: string,
  name: string,
  raw: string,
  kind: TransactionKind,
  account: string,
  amount: number,
  extra: Partial<Omit<FakeTransaction, 'id' | 'sequence'>> = {},
): FakeTransaction {
  return {
    id: `f${sequence}`,
    sequence,
    day,
    name,
    establishmentKey: name,
    raw,
    kind,
    subtype: null,
    account,
    amount,
    line: null,
    by: null,
    confidence: null,
    pending: null,
    posted: true,
    tags: [],
    movement: false,
    person: false,
    refund: false,
    refundOffsetsLine: null,
    reimbursementIncome: false,
    excludedReason: null,
    split: null,
    reimbursement: null,
    justification: null,
    askReason: false,
    reasonText: '',
    note: '',
    history: [],
    installment: null,
    bill: kind === 'cartao' ? 'Dez/26' : null,
    originalAmount: null,
    iof: null,
    ...extra,
  };
}

const card = (sequence: number, day: string, name: string, raw: string, amount: number, extra?: Partial<FakeTransaction>) =>
  bank(sequence, day, name, raw, 'cartao', CARD_ACCOUNT, amount, extra);

/** A function rather than a constant, so a bundle that never calls it drops the data. */
const transactions = (): FakeTransaction[] => [
  card(1, '2026-11-27', 'Padaria Aurora', 'PADARIA AURORA LTDA', -18.5, { posted: false }),
  card(2, '2026-11-26', 'Livraria Pau Brasil', 'LIVRARIA PAU BRASIL SP', -129, { installment: '1/3' }),
  card(3, '2026-11-24', 'Serviço de nuvem', 'CLOUDSVC SUBSCRIPTION', -113.85, { originalAmount: 'US$ 20,00', iof: 3.85 }),
  card(4, '2026-11-21', 'Posto Beira Rio', 'POSTO BEIRA RIO AV 9', -180),
  card(5, '2026-11-19', 'Cantina da Praça', 'CANTINA DA PRACA ME', -72.7),
  card(6, '2026-11-12', 'Mercado Bom Preço', 'MERCADO BOM PRECO 021', -148.3, { bill: 'Nov/26' }),
  bank(10, '2026-11-23', 'Cafeteria do Largo', 'PIX ENVIADO CAFETERIA DO LARGO', 'pix', MERCADO_PAGO, -26.5),
  bank(11, '2026-11-19', 'Marina T. Duarte', 'PIX ENVIADO MARINA T DUARTE', 'pix', 'Itaú', -80, { person: true }),
  bank(12, '2026-11-12', 'Paulo R. Nogueira', 'PIX RECEBIDO PAULO R NOGUEIRA', 'pix', 'Itaú', 150, { person: true }),
  bank(13, '2026-11-05', 'Empresa Exemplo Ltda', 'PIX RECEBIDO EMPRESA EXEMPLO', 'pix', 'Itaú', 8000),
  bank(20, '2026-11-10', 'Telecom Alvorada', 'ALVORADA TELECOM S.A.', 'boleto', 'Itaú', -109.9),
  bank(21, '2026-11-08', 'Plano de saúde', 'VIDA PLENA SAUDE', 'boleto', 'Itaú', -480),
  bank(30, '2026-11-16', 'Pet Shop Bicho Bom', 'DEB PET SHOP BICHO BOM', 'outros', 'C6', -96, { subtype: 'Débito' }),
  bank(31, '2026-11-03', 'Tarifa de pacote', 'TAR PACOTE SERVICOS', 'outros', 'Itaú', -25, { subtype: 'Tarifa' }),
  card(40, '2026-10-28', 'Mercado Bom Preço', 'MERCADO BOM PRECO 021', -64.2, { bill: 'Nov/26' }),
  bank(41, '2026-10-05', 'Empresa Exemplo Ltda', 'PIX RECEBIDO EMPRESA EXEMPLO', 'pix', 'Itaú', 8000),
];

const ACCOUNTS: FakeAccount[] = [
  { id: 'itau', bank: 'Itaú', name: 'Conta corrente', kind: 'Conta', balance: 5320.4 },
  { id: 'mp', bank: MERCADO_PAGO, name: 'Conta', kind: 'Conta', balance: 210.9 },
  { id: 'c6', bank: 'C6', name: 'Conta corrente', kind: 'Conta', balance: 640.25 },
  { id: 'card', bank: 'C6', name: 'C6 Carbon · final 1234', kind: 'Cartão', balance: 1820.7 },
];

const CARD_STATE: FakeCard = {
  limit: 9000,
  bankClosingDay: 28,
  dueDay: 7,
  bills: [
    {
      month: '2026-11',
      closedOn: '2026-10-28',
      dueOn: '2026-11-07',
      items: [
        { name: 'Mercado Bom Preço', day: '2026-10-28', amount: 64.2 },
        { name: 'Outras 22 compras', day: null, amount: 1388.4 },
      ],
    },
    {
      month: '2026-10',
      closedOn: '2026-09-28',
      dueOn: '2026-10-07',
      items: [{ name: 'Outras 27 compras', day: null, amount: 1612.9 }],
    },
  ],
};

/**
 * What the bank reports about a pot; the target and the linked lines are the person's to give.
 * The bucket starts as what the bank says the pot is, a reserve as reserved and an investment as
 * invested, until the person says otherwise: a pot counted in no bucket would be missing from the
 * net worth on the first day.
 */
const pot = (
  id: string,
  name: string,
  bankName: string,
  balance: number,
  cdiPercent: number | null,
  extra: Partial<FakeSavingsPot> = {},
): FakeSavingsPot => ({
  id,
  name,
  bank: bankName,
  bucket: extra.investment ? 'Investido' : 'Reservado',
  balance,
  cdiPercent,
  target: null,
  linkedLines: [],
  lineShares: null,
  maturity: null,
  closed: false,
  investment: null,
  movements: [],
  ...extra,
});

const savingsPots = (): FakeSavingsPot[] => [
  pot('p1', 'Caixinha da casa', MERCADO_PAGO, 1800, null),
  pot('p2', '', MERCADO_PAGO, 450, null),
  pot('p3', 'Reserva', 'Nubank', 12000, 100, {
    movements: [{ day: '2026-10-05', description: 'Depósito', amount: 1000 }],
  }),
  pot('i1', 'CDB 115% do CDI', 'Nubank', 25000, 115, {
    maturity: '2028-03-15',
    investment: { applied: 22400, tax: 455 },
    movements: [{ day: '2025-03-15', description: 'Aplicação', amount: 22400 }],
  }),
  pot('i0', 'CDB encerrado', 'Nubank', 0, null, { maturity: '2026-09-10', closed: true, investment: { applied: 8000, tax: 0 } }),
];

const FRESH_SETTINGS: FakeSettings = {
  minExamples: 5,
  calibration: [null, null, null, null, null],
  cardLabel: 'Físico',
  closingMode: 'auto',
  manualClosingDay: 28,
  connections: [
    { bank: 'Itaú', consentExpired: false },
    { bank: MERCADO_PAGO, consentExpired: false },
    { bank: 'C6', consentExpired: false },
    { bank: 'Nubank', consentExpired: false },
  ],
  lastSync: { day: TODAY, time: '02:00' },
  lastBackup: null,
};

function noReview(): FakeState['review'] {
  // The prototype's report is sentences about invented spending; over a real ledger they would be false statements.
  return { proposals: [], awaiting: [], warnings: [], resolved: 0, stepsDone: {}, report: [] };
}

/** The part of the state no bank record fills in. */
function authoredNothing() {
  return {
    currentMonth: CURRENT_MONTH,
    confidenceThreshold: 85,
    establishments: {},
    budget: structuredClone(EMPTY_BUDGET),
    scenarios: [],
    spentBaseline: {},
    plannedContributions: [],
    tags: [],
    weeks: [],
    payAhead: null,
    subscriptions: [],
    recurringPurchases: [],
    subscriptionSuggestion: null,
    review: noReview(),
  } satisfies Partial<FakeState>;
}

/** The bank's records have arrived and nothing the person authors exists yet. */
export function freshInstallState(): FakeState {
  return structuredClone({
    ...authoredNothing(),
    shell: {
      today: TODAY,
      sync: { state: 'ok', lastSyncDay: TODAY, lastSyncTime: '02:00' },
      firstMonth: '2026-10',
      lastMonth: '2026-12',
      closedMonths: [],
    },
    budgetVersions: [{ from: '2026-10', groups: structuredClone(EMPTY_BUDGET) }],
    transactions: transactions(),
    savingsPots: savingsPots(),
    accounts: ACCOUNTS,
    card: CARD_STATE,
    settings: FRESH_SETTINGS,
  } satisfies FakeState);
}

/** Nothing at all: the app before its first synchronisation. */
export function emptyLedgerState(): FakeState {
  return structuredClone({
    ...authoredNothing(),
    shell: {
      today: TODAY,
      sync: { state: 'warning', message: 'Aguardando a primeira sincronização' },
      firstMonth: CURRENT_MONTH,
      lastMonth: '2026-12',
      closedMonths: [],
    },
    budgetVersions: [{ from: CURRENT_MONTH, groups: structuredClone(EMPTY_BUDGET) }],
    transactions: [],
    savingsPots: [],
    accounts: [],
    card: null,
    settings: { ...FRESH_SETTINGS, connections: [], lastSync: null },
  } satisfies FakeState);
}
