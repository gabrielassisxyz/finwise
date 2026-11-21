import type { PendingReview, TransactionKind } from '../types';
import type {
  FakeAccount,
  FakeBudgetGroup,
  FakeEstablishment,
  FakeHistoryEntry,
  FakePlannedContribution,
  FakeSavingsPot,
  FakeRecurringPurchase,
  FakeState,
  FakeSubscription,
  FakeTransaction,
} from './model';

// The dataset of the design prototype. Every figure and name in it is
// invented. The parity check renders these exact values against the
// prototype's screenshots, so a change here is a change to that baseline.

export const PROTOTYPE_PIN = '123456';

export const PROTOTYPE_TODAY = '2026-11-28';

const CARD = 'C6 Carbon';
const MERCADO_PAGO = 'Mercado Pago';

type Extra = Partial<Omit<FakeTransaction, 'id' | 'sequence'>>;

function transaction(
  sequence: number,
  day: number,
  name: string,
  raw: string,
  kind: TransactionKind,
  account: string,
  amount: number,
  line: string | null,
  extra: Extra = {},
): FakeTransaction {
  const built: FakeTransaction = {
    id: `t${sequence}`,
    sequence,
    day: `2026-11-${String(day).padStart(2, '0')}`,
    name,
    establishmentKey: name,
    raw,
    kind,
    subtype: null,
    account,
    amount,
    line,
    by: 'Regra',
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
  return { ...built, history: built.history.length ? built.history : [firstHistoryEntry(built)] };
}

/** How the prototype opens every history: with what put the transaction where it is. */
function firstHistoryEntry(built: FakeTransaction): FakeHistoryEntry {
  let text = `${built.by} classificou como ${built.line}`;
  if (built.movement) text = 'Regra marcou como Movimentação';
  else if (built.pending) text = `IA sugeriu ${built.pending.suggestion} (${built.pending.confidence}%)`;
  return { id: `${built.id}-h0`, text, day: built.day, reason: null, restore: null };
}

const card = (sequence: number, day: number, name: string, raw: string, amount: number, line: string | null, extra?: Extra) =>
  transaction(sequence, day, name, raw, 'cartao', CARD, amount, line, extra);

const pending = (reason: string, suggestion: string, confidence: number, justification: string): PendingReview => ({
  reason,
  suggestion,
  confidence,
  justification,
});

const PAO_DE_ACUCAR = 'PAO DE ACUCAR LJ 1187';
const NEXT_BILL = 'Jan/27';

const TRANSACTIONS: FakeTransaction[] = [
  card(1, 28, 'Cinema Belas Artes', 'CINE BELAS ARTES SP', -65.1, 'Lazer', { posted: false, bill: NEXT_BILL }),
  card(2, 28, 'Posto Shell', 'POSTO SHELL AV PAULISTA', -180, 'Transporte', { bill: NEXT_BILL }),
  card(3, 28, 'iFood', 'IFOOD *IFOOD OSASCO', -54.9, 'Delivery', { bill: NEXT_BILL }),
  card(4, 28, 'Pão de Açúcar', PAO_DE_ACUCAR, -86.4, 'Mercado', {
    bill: NEXT_BILL,
    by: 'IA',
    confidence: 62,
    justification: 'Compras anteriores neste estabelecimento foram todas em Mercado.',
  }),
  card(5, 27, 'Bar do Zé', 'BAR E LANCH DO ZE LTDA', -64, null, {
    pending: pending('Baixa confiança', 'Restaurantes e bares', 58, 'O nome sugere bar, mas a compra foi no horário do almoço.'),
  }),
  card(6, 26, 'Amazon', 'AMAZON MARKETPLACE BR', -129, 'Casa', { installment: '2/3' }),
  card(26, 25, 'Hortifruti', 'HORTIFRUTI VILA MADALENA', -14, 'Mercado'),
  card(7, 24, 'Serviço de nuvem', 'CLOUDSVC SUBSCRIPTION', -113.85, 'Ferramentas de trabalho', {
    originalAmount: 'US$ 20,00',
    iof: 3.85,
  }),
  card(8, 22, 'Spotify', 'SPOTIFY BR', -23.9, 'Assinaturas'),
  card(9, 21, 'Pão de Açúcar', PAO_DE_ACUCAR, -31.2, 'Mercado'),
  card(10, 20, 'Estorno de anuidade', 'ESTORNO ANUIDADE CARBON', 120, 'Tarifas e juros', { refund: true }),
  card(11, 19, 'Rappi', 'RAPPI BRASIL INTERMEDIACAO', -72.7, 'Delivery'),
  card(12, 18, 'Uber', 'UBER *TRIP HELP.UBER.COM', -38.7, null, {
    pending: pending('Baixa confiança', 'Transporte', 66, 'Corridas neste horário já caíram em Transporte e em Transporte para o trabalho.'),
  }),
  card(13, 17, 'Pão de Açúcar', PAO_DE_ACUCAR, -18.9, 'Mercado'),
  card(14, 16, 'Mercado Livre', 'MERCADOLIVRE*3PRODUTOS', -149.9, null, {
    pending: pending('Baixa confiança', 'Casa', 52, 'Compras neste marketplace se dividem entre Casa e Compras.'),
  }),
  card(15, 15, 'iFood', 'IFOOD *IFOOD OSASCO', -24.15, 'Restaurantes e bares', {
    by: 'IA',
    confidence: 71,
    justification: 'O pedido veio de um restaurante onde você costuma comer no local.',
  }),
  card(16, 14, 'Pousada Serra Azul', 'POUSADA SERRA AZUL ME', -420, 'Viagens', { tags: ['#viagem'] }),
  card(17, 13, 'Farmácia São João', 'FARMACIA SAO JOAO 0412', -212.4, null, {
    pending: pending('Valor fora do padrão', 'Saúde', 77, 'O valor é quatro vezes a média das suas compras nesta farmácia.'),
  }),
  card(18, 12, 'Pão de Açúcar', PAO_DE_ACUCAR, -27.5, 'Mercado'),
  card(19, 12, 'iFood', 'IFOOD *IFOOD OSASCO', -78.4, 'Delivery'),
  card(20, 10, 'Sal & Brasa', 'REST SAL E BRASA LTDA', -300, 'Restaurantes e bares', { tags: ['#com-ana'] }),
  card(21, 9, 'Loja Verde Vida', 'VERDE VIDA COM PLANTAS', -89.9, null, {
    pending: pending('Estabelecimento novo', 'Casa', 44, 'Primeira compra aqui. O nome lembra uma loja de plantas.'),
  }),
  card(22, 7, 'Pão de Açúcar', PAO_DE_ACUCAR, -22, 'Mercado'),
  card(23, 6, 'Hortifruti', 'HORTIFRUTI VILA MADALENA', -20, 'Mercado'),
  card(24, 5, 'Assistente de IA', 'AI ASSISTANT PRO', -110, 'Assinaturas'),
  card(25, 3, 'Pão de Açúcar', PAO_DE_ACUCAR, -26, 'Mercado'),
  transaction(30, 23, 'Padaria Estrela', 'PIX ENVIADO PADARIA ESTRELA', 'pix', MERCADO_PAGO, -26.5, null, {
    pending: pending('Estabelecimento novo', 'Mercado', 49, 'Primeiro PIX para este recebedor. Padarias costumam cair em Mercado.'),
  }),
  transaction(31, 19, 'Marcos A. Lima', 'PIX ENVIADO MARCOS A LIMA', 'pix', 'Itaú', -80, null, {
    person: true,
    pending: pending(
      'PIX para pessoa sem apelido',
      'Lazer',
      38,
      'Não há histórico com esta pessoa. O valor parece com outros gastos de Lazer.',
    ),
  }),
  transaction(32, 12, 'Ana C. Ribeiro', 'PIX RECEBIDO ANA C RIBEIRO', 'pix', 'Itaú', 150, 'Reembolsos', {
    person: true,
    reimbursementIncome: true,
  }),
  transaction(33, 11, 'Carlos Mendes', 'PIX RECEBIDO CARLOS MENDES', 'pix', 'Itaú', 200, 'Renda extra', { person: true }),
  transaction(34, 5, 'João P. Santos', 'PIX ENVIADO JOAO P SANTOS', 'pix', 'Itaú', -1500, 'Contas da casa', { person: true }),
  transaction(35, 5, 'Salário', 'PIX RECEBIDO EMPRESA LTDA', 'pix', 'Itaú', 10000, 'Salário'),
  transaction(36, 3, 'Hortifruti', 'PIX ENVIADO HORTIFRUTI VM', 'pix', 'Itaú', -62, 'Mercado', {
    by: 'Você',
    history: [
      { id: 't36-h0', text: 'IA classificou como Lazer', day: '2026-11-03', reason: null, restore: null },
      {
        id: 't36-h1',
        text: 'Você alterou para Mercado',
        day: '2026-11-04',
        reason: 'é o mercado do bairro',
        restore: { line: 'Lazer', by: 'IA', confidence: 54 },
      },
    ],
  }),
  transaction(40, 10, 'Internet', 'VELOX TELECOM S.A.', 'boleto', 'Itaú', -109.9, 'Contas da casa'),
  transaction(41, 10, 'Condomínio', 'COND ED JARDIM DAS ACACIAS', 'boleto', 'Itaú', -450, 'Contas da casa'),
  transaction(42, 8, 'Plano de saúde', 'VIDA PLENA SAUDE', 'boleto', 'Itaú', -480, 'Saúde'),
  transaction(50, 16, 'Pet Shop Bicho Bom', 'DEB PET SHOP BICHO BOM', 'outros', 'C6', -96, 'Pets', { subtype: 'Débito' }),
  transaction(52, 7, 'Pagamento da fatura', 'TEF PAGTO FATURA CARBON', 'outros', 'Itaú', -2140, null, {
    subtype: 'TEF',
    movement: true,
  }),
  transaction(51, 5, 'Transferência para C6', 'TED MESMA TITULARIDADE', 'outros', 'Itaú', -1200, null, {
    subtype: 'TED',
    movement: true,
  }),
  transaction(53, 3, 'Tarifa de pacote', 'TAR PACOTE SERVICOS', 'outros', 'Itaú', -25, 'Tarifas e juros', { subtype: 'Tarifa' }),
  card(60, 0, 'Pão de Açúcar', PAO_DE_ACUCAR, -64.2, 'Mercado', { day: '2026-10-28', bill: 'Nov/26' }),
  card(61, 0, 'Posto Shell', 'POSTO SHELL AV PAULISTA', -175, 'Transporte', { day: '2026-10-20', bill: 'Nov/26' }),
  transaction(62, 0, 'Condomínio', 'COND ED JARDIM DAS ACACIAS', 'boleto', 'Itaú', -450, 'Contas da casa', { day: '2026-10-10' }),
  transaction(63, 0, 'João P. Santos', 'PIX ENVIADO JOAO P SANTOS', 'pix', 'Itaú', -1500, 'Contas da casa', {
    day: '2026-10-05',
    person: true,
  }),
  transaction(64, 0, 'Salário', 'PIX RECEBIDO EMPRESA LTDA', 'pix', 'Itaú', 10000, 'Salário', { day: '2026-10-05' }),
];

const ESTABLISHMENTS: Record<string, FakeEstablishment> = {
  'João P. Santos': { nickname: 'Aluguel' },
  'Ana C. Ribeiro': { nickname: 'Ana' },
  'Carlos Mendes': { nickname: 'Carlos' },
  'Pão de Açúcar': { line: 'Mercado' },
  Hortifruti: { line: 'Mercado' },
  'Posto Shell': { line: 'Transporte' },
  'Pousada Serra Azul': { tags: ['#viagem'] },
  Spotify: { line: 'Assinaturas' },
};

const BUDGET: FakeBudgetGroup[] = [
  {
    key: 'rec',
    name: 'Receita',
    lines: [
      { name: 'Salário', amount: 10000 },
      { name: 'Renda extra', amount: 1500, flag: 'Não garantida' },
      { name: 'Reembolsos', amount: 500 },
    ],
  },
  {
    key: 'fix',
    name: 'Fixos e essenciais',
    lines: [
      { name: 'Contas da casa', amount: 2000 },
      { name: 'Saúde', amount: 500 },
      { name: 'Pets', amount: 200 },
      { name: 'Transporte para o trabalho', amount: 1200 },
    ],
  },
  {
    key: 'var',
    name: 'Variáveis',
    lines: [
      { name: 'Mercado', amount: 400 },
      { name: 'Restaurantes e bares', amount: 450 },
      { name: 'Delivery', amount: 200 },
      { name: 'Transporte', amount: 280 },
      { name: 'Casa', amount: 220 },
      { name: 'Assinaturas', amount: 180 },
      { name: 'Ferramentas de trabalho', amount: 600 },
      { name: 'Lazer', amount: 400 },
      { name: 'Educação', amount: 150 },
    ],
  },
  {
    key: 'prov',
    name: 'Provisões',
    lines: [
      { name: 'Gás e IPTU', amount: 120 },
      { name: 'Assinaturas anuais', amount: 50 },
      { name: 'Remédios', amount: 150 },
    ],
  },
  {
    key: 'res',
    name: 'Reservas de desejo',
    lines: [
      { name: 'Compras', amount: 500 },
      { name: 'Viagens', amount: 600 },
    ],
  },
];

const SPENT_BASELINE: FakeState['spentBaseline'] = {
  '2026-11': {
    Salário: 10000,
    'Renda extra': 200,
    Reembolsos: 150,
    'Contas da casa': 2059.9,
    Saúde: 480,
    Pets: 96,
    'Transporte para o trabalho': 1200,
    Mercado: 308,
    'Restaurantes e bares': 324.15,
    Delivery: 206,
    Transporte: 180,
    Casa: 129,
    Assinaturas: 133.9,
    'Ferramentas de trabalho': 113.85,
    Lazer: 65.1,
    Educação: 0,
    Viagens: 420,
    Compras: 0,
    'Tarifas e juros': 25,
  },
  '2026-10': {
    Salário: 10000,
    'Renda extra': 1500,
    Reembolsos: 320,
    'Contas da casa': 2012.4,
    Saúde: 500,
    Pets: 214,
    'Transporte para o trabalho': 1200,
    Mercado: 388,
    'Restaurantes e bares': 412,
    Delivery: 248,
    Transporte: 265,
    Casa: 190,
    Assinaturas: 155.8,
    'Ferramentas de trabalho': 113.85,
    Lazer: 372,
    Educação: 150,
    Viagens: 0,
    Compras: 310,
    'Tarifas e juros': 25,
  },
};

type PotDetails = Partial<Omit<FakeSavingsPot, 'id' | 'name' | 'bank' | 'bucket' | 'balance' | 'cdiPercent'>>;

const pot = (
  id: string,
  name: string,
  bank: string,
  bucket: FakeSavingsPot['bucket'],
  balance: number,
  cdiPercent: number,
  details: PotDetails = {},
): FakeSavingsPot => ({
  id,
  name,
  bank,
  bucket,
  balance,
  cdiPercent,
  target: null,
  linkedLines: [],
  lineShares: null,
  maturity: null,
  closed: false,
  investment: null,
  movements: [],
  ...details,
});

const SAVINGS_POTS: FakeSavingsPot[] = [
  pot('rfs', 'RF Saúde', MERCADO_PAGO, 'Reservado', 2000, 120, { target: 2000, linkedLines: ['Rainy Fund · Saúde'] }),
  pot('rfp', 'RF Pets', MERCADO_PAGO, 'Reservado', 4200, 120, {
    target: 5000,
    linkedLines: ['Rainy Fund · Pets'],
    movements: [{ day: '2026-10-14', description: 'Retirada · veterinário', amount: -800 }],
  }),
  pot('rfc', 'RF Casa', MERCADO_PAGO, 'Reservado', 3000, 120, { target: 3000, linkedLines: ['Rainy Fund · Casa'] }),
  pot('rft', 'RF Tech', MERCADO_PAGO, 'Reservado', 2000, 120, { target: 2000, linkedLines: ['Rainy Fund · Tech'] }),
  pot('gas', 'Gás e IPTU', MERCADO_PAGO, 'Reservado', 600, 120, {
    nextCharge: { description: 'IPTU', day: '2027-01-10' },
    linkedLines: ['Gás', 'IPTU'],
    lineShares: [
      { line: 'Gás', amount: 240 },
      { line: 'IPTU', amount: 360 },
    ],
  }),
  pot('asa', 'Assinaturas anuais', MERCADO_PAGO, 'Reservado', 310, 120, {
    linkedLines: ['Assinaturas anuais'],
    nextCharge: { description: 'Amazon Prime', day: '2026-12-07' },
  }),
  pot('rem', 'Remédios', MERCADO_PAGO, 'Reservado', 450, 120, { linkedLines: ['Remédios'] }),
  pot('com', 'Compras', MERCADO_PAGO, 'Reservado', 1240, 120, { linkedLines: ['Compras'] }),
  pot('via', 'Viagens', MERCADO_PAGO, 'Reservado', 2350, 120, {
    linkedLines: ['Viagens'],
    movements: [{ day: '2026-11-14', description: 'Retirada · Pousada Serra Azul', amount: -420 }],
  }),
  pot('emg', 'Reserva de emergência', 'Nubank', 'Reservado', 18000, 100, {
    movements: [{ day: '2026-10-05', description: 'Depósito', amount: 1000 }],
  }),
  pot('i1', 'Investimento 1', 'Nubank', 'Investido', 25000, 115, {
    maturity: '2028-03-15',
    linkedLines: ['Investimento'],
    investment: { applied: 22400, tax: 455 },
    movements: [{ day: '2025-03-15', description: 'Aplicação', amount: 22400 }],
  }),
  pot('i2', 'Investimento 2', 'Nubank', 'Investido', 12800, 115, {
    maturity: '2027-08-02',
    linkedLines: ['Investimento'],
    investment: { applied: 12000, tax: 160 },
    movements: [{ day: '2025-08-02', description: 'Aplicação', amount: 12000 }],
  }),
  pot('sn', '', 'C6', null, 500, 100),
  pot('i0', 'Investimento 0', 'Nubank', 'Investido', 0, 110, {
    maturity: '2026-09-10',
    closed: true,
    investment: { applied: 8000, tax: 0 },
  }),
];

const PLANNED_CONTRIBUTIONS: FakePlannedContribution[] = [
  { name: 'RF Casa', amount: 0, state: 'on-target', doneOn: null },
  { name: 'Gás e IPTU', amount: 120, state: 'done', doneOn: '2026-11-05' },
  { name: 'Assinaturas anuais', amount: 50, state: 'done', doneOn: '2026-11-05' },
  { name: 'Remédios', amount: 150, state: 'done', doneOn: '2026-11-05' },
  { name: 'Compras', amount: 500, state: 'pending', doneOn: null },
  { name: 'Viagens', amount: 600, state: 'pending', doneOn: null },
  { name: 'Investimento', amount: 3800, state: 'pending', doneOn: null },
];

const ACCOUNTS: FakeAccount[] = [
  { id: 'itau', bank: 'Itaú', name: 'Conta corrente', kind: 'Conta', balance: 7020 },
  { id: 'mp', bank: MERCADO_PAGO, name: 'Conta', kind: 'Conta', balance: 312.4 },
  { id: 'c6', bank: 'C6', name: 'Conta corrente', kind: 'Conta', balance: 1180.25 },
  { id: 'nu', bank: 'Nubank', name: 'Conta', kind: 'Conta', balance: 96.1 },
  { id: 'card', bank: 'C6', name: 'C6 Carbon · final 1234', kind: 'Cartão', balance: 2310 },
];

type SubscriptionDetails = Partial<Omit<FakeSubscription, 'id' | 'name' | 'amount' | 'cycle' | 'nextCharge' | 'line' | 'status' | 'reviewOn'>>;

const subscription = (
  id: string,
  name: string,
  amount: number,
  cycle: FakeSubscription['cycle'],
  nextCharge: string,
  line: string,
  status: FakeSubscription['status'],
  reviewOn: string,
  details: SubscriptionDetails = {},
): FakeSubscription => ({
  id,
  name,
  amount,
  cycle,
  nextCharge,
  line,
  status,
  reviewOn,
  note: '',
  flags: [],
  foreign: null,
  previousAmount: null,
  alternatives: [],
  ...details,
});

const SUBSCRIPTIONS: FakeSubscription[] = [
  subscription('ai', 'Assistente de IA', 110, 'Mensal', '2026-12-05', 'Assinaturas', 'Manter', '2027-03-01', {
    flags: ['overlapping'],
    alternatives: [
      'Plano anual: R$ 1.100,00, economia de R$ 220,00 por ano.',
      'Dois planos foram cobrados neste mês. Manter só um economiza R$ 110,00 por mês.',
    ],
  }),
  subscription('sp', 'Spotify', 21.9, 'Mensal', '2026-12-22', 'Assinaturas', 'Manter', '2027-01-15', {
    previousAmount: 19.9,
    alternatives: ['Plano anual: R$ 219,00, economia de R$ 43,80 por ano.', 'Plano para duas pessoas: R$ 27,90, se for dividir com alguém.'],
  }),
  subscription('ap', 'Amazon Prime', 166.8, 'Anual', '2026-12-07', 'Assinaturas anuais', 'Analisar', '2026-11-30', {
    alternatives: ['Você já está no plano anual: R$ 166,80, economia de R$ 72,00 por ano em relação ao mensal.'],
  }),
  subscription('ho', 'Hospedagem', 60, 'Trimestral', '2027-01-10', 'Ferramentas de trabalho', 'Manter', '2027-06-01', {
    foreign: { originalAmount: 'US$ 10,50', iof: 2.03 },
    alternatives: ['Plano anual: US$ 36,00, economia de cerca de R$ 34,00 por ano.'],
  }),
  subscription('rv', 'Revista digital', 29.9, 'Mensal', '2026-12-14', 'Assinaturas', 'Analisar', '2026-09-15', {
    alternatives: ['Assinatura avulsa por edição: R$ 9,90, compensa se você lê menos de três por mês.'],
  }),
  subscription('do', 'Domínio', 80, 'Anual', '2027-04-18', 'Assinaturas anuais', 'Manter', '2027-04-01'),
];

const RECURRING_PURCHASES: FakeRecurringPurchase[] = [
  { name: 'Areia do gato', cadence: { every: 3, unit: 'weeks' }, lastPurchase: '2026-11-08', amount: 54.9 },
  { name: 'Filtro de água', cadence: { every: 6, unit: 'months' }, lastPurchase: '2026-05-14', amount: 89 },
  { name: 'Remédio de uso contínuo', cadence: { every: 30, unit: 'days' }, lastPurchase: '2026-10-27', amount: 112.4 },
  { name: 'Ração', cadence: { every: 5, unit: 'weeks' }, lastPurchase: '2026-11-02', amount: 139.9 },
];

export const PROTOTYPE_CARD_ACCOUNT = CARD;
export const PROTOTYPE_CARD_LABEL = 'C6 Carbon · 1234';
export const PROTOTYPE_CARD_FULL_LABEL = 'C6 Carbon · final 1234';

const CARD_STATE: FakeState['card'] = {
  limit: 12000,
  bankClosingDay: 28,
  dueDay: 7,
  bills: [
    {
      month: '2026-11',
      closedOn: '2026-10-28',
      dueOn: '2026-11-07',
      items: [
        { name: 'Pão de Açúcar', day: '2026-10-28', amount: 64.2 },
        { name: 'Posto Shell', day: '2026-10-20', amount: 175 },
        { name: 'Assistente de IA', day: '2026-10-05', amount: 110 },
        { name: 'Outras 31 compras', day: null, amount: 1790.8 },
      ],
    },
    {
      month: '2026-10',
      closedOn: '2026-09-28',
      dueOn: '2026-10-07',
      items: [
        { name: 'Posto Shell', day: '2026-09-22', amount: 168 },
        { name: 'Spotify', day: '2026-09-22', amount: 19.9 },
        { name: 'Outras 28 compras', day: null, amount: 1797.4 },
      ],
    },
    {
      month: '2026-09',
      closedOn: '2026-08-28',
      dueOn: '2026-09-07',
      items: [
        { name: 'Amazon', day: '2026-08-26', amount: 129 },
        { name: 'Outras 35 compras', day: null, amount: 2283.75 },
      ],
    },
  ],
};

const SETTINGS_STATE: FakeState['settings'] = {
  minExamples: 5,
  calibration: [46, 31, 18, 7, 2],
  cardLabel: 'Físico',
  closingMode: 'auto',
  manualClosingDay: 28,
  connections: [
    { bank: 'Itaú', consentExpired: false },
    { bank: MERCADO_PAGO, consentExpired: false },
    { bank: 'C6', consentExpired: false },
    { bank: 'Nubank', consentExpired: false },
  ],
  lastSync: { day: PROTOTYPE_TODAY, time: '02:00' },
  lastBackup: { day: PROTOTYPE_TODAY, time: '02:15' },
};

/** A fresh copy of the prototype's dataset, safe to mutate. */
export function prototypeState(): FakeState {
  return structuredClone({
    shell: {
      today: PROTOTYPE_TODAY,
      sync: { state: 'ok', lastSyncDay: PROTOTYPE_TODAY, lastSyncTime: '02:00' },
      firstMonth: '2026-10',
      lastMonth: '2026-12',
      closedMonths: [{ month: '2026-10', closedOn: '2026-11-02' }],
    },
    currentMonth: '2026-11',
    confidenceThreshold: 85,
    transactions: TRANSACTIONS,
    establishments: ESTABLISHMENTS,
    budget: BUDGET,
    budgetVersions: [{ from: '2026-10', groups: structuredClone(BUDGET) }],
    scenarios: [],
    spentBaseline: SPENT_BASELINE,
    savingsPots: SAVINGS_POTS,
    accounts: ACCOUNTS,
    plannedContributions: PLANNED_CONTRIBUTIONS,
    tags: ['#viagem', '#com-ana', '#trabalho', '#presente', '#casa-nova'],
    weeks: [
      { label: '01–07', guide: 500, spent: 620, current: false },
      { label: '08–14', guide: 470, spent: 410, current: false },
      { label: '15–21', guide: 400, spent: 220, current: false },
      { label: '22–28', guide: 355, spent: 210, current: true },
      { label: '29–30', guide: 142, spent: null, current: false },
    ],
    payAhead: { amount: 386.4, cardLabel: 'C6 Carbon · final 1234' },
    subscriptions: SUBSCRIPTIONS,
    recurringPurchases: RECURRING_PURCHASES,
    subscriptionSuggestion: { transactionId: 't7', dismissed: false },
    card: CARD_STATE,
    settings: SETTINGS_STATE,
  } satisfies FakeState);
}
