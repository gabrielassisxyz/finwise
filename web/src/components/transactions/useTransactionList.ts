import { computed, reactive, ref, watch, type Ref } from 'vue';
import { useRoute } from 'vue-router';
import { useApi } from '@/api/context';
import type { Transaction, TransactionFilterOptions, TransactionKind, TransactionPeriod, YearMonth } from '@/api/types';
import { brl } from '@/lib/format';
import { normalize, plural } from '@/lib/text';
import { useRevisionStore } from '@/stores/revision';

export type TransactionTab = 'todas' | TransactionKind;

export interface TransactionFilters {
  query: string;
  account: string | null;
  line: string | null;
  tag: string | null;
  establishment: string | null;
  period: TransactionPeriod;
  onlyPending: boolean;
  uncategorised: boolean;
}

export interface FilterChip {
  label: string;
  remove: () => void;
}

const TABS: { id: TransactionTab; label: string }[] = [
  { id: 'todas', label: 'Todas' },
  { id: 'pix', label: 'PIX' },
  { id: 'boleto', label: 'Boletos' },
  { id: 'cartao', label: 'Cartão' },
  { id: 'outros', label: 'Outros' },
];

const PERIOD_CHIPS: Partial<Record<TransactionPeriod, string>> = {
  year: 'Período: este ano',
  'last-3-months': 'Período: últimos 3 meses',
};

function emptyFilters(): TransactionFilters {
  return {
    query: '',
    account: null,
    line: null,
    tag: null,
    establishment: null,
    period: 'month',
    onlyPending: false,
    uncategorised: false,
  };
}

/** Everything a search may match: names, the bank's text, tags and the amount as typed or as shown. */
function searchText(transaction: Transaction): string {
  const typedAmount = Math.abs(transaction.amount).toFixed(2).replace('.', ',');
  return normalize(
    [
      transaction.name,
      transaction.originalName,
      transaction.rawDescription,
      transaction.tags.join(' '),
      typedAmount,
      brl(transaction.amount),
    ].join(' '),
  );
}

function matcher(filters: TransactionFilters): (transaction: Transaction) => boolean {
  const wanted = normalize(filters.query.trim());
  return (transaction) =>
    (!filters.account || transaction.account === filters.account) &&
    (!filters.line || transaction.line === filters.line) &&
    (!filters.tag || transaction.tags.includes(filters.tag)) &&
    (!filters.establishment || transaction.establishmentKey === filters.establishment) &&
    (!filters.onlyPending || !!transaction.pending) &&
    (!filters.uncategorised || (!transaction.line && !transaction.movement)) &&
    (!wanted || searchText(transaction).includes(wanted));
}

/** What the visible rows add up to: money out shows plain, a net inflow shows with a plus. */
function totalLine(rows: Transaction[]): string {
  const counted = rows.filter((transaction) => transaction.countsInTotals);
  const net = Math.round(-counted.reduce((total, transaction) => total + transaction.effectiveAmount, 0) * 100) / 100;
  return `${plural(rows.length, 'transação', 'transações')} · ${net < 0 ? '+ ' : ''}${brl(net)}`;
}

/**
 * The state behind a list of transactions: what was fetched for the month,
 * the filters over it, and what they leave.
 */
export function useTransactionList(month: Ref<YearMonth>) {
  const api = useApi();
  const revision = useRevisionStore();

  const transactions = ref<Transaction[]>([]);
  const options = ref<TransactionFilterOptions>({ accounts: [], tags: [], lines: [] });
  const loading = ref(true);
  const tab = ref<TransactionTab>('todas');
  const filters = reactive<TransactionFilters>(emptyFilters());
  // The budget sends a person here already looking at one establishment.
  const arrivedAt = useRoute().query.establishment;
  if (typeof arrivedAt === 'string' && arrivedAt) filters.establishment = arrivedAt;

  /** A refetch after a write keeps the rows on screen instead of flashing the skeleton. */
  async function load(mode: { quiet?: boolean } = {}): Promise<void> {
    if (!mode.quiet) loading.value = true;
    const requested = { month: month.value, period: filters.period };
    const [list, filterOptions] = await Promise.all([api.transactions.list(requested), api.transactions.filterOptions()]);
    // A slower answer for a month the user has already left must not land.
    if (requested.month !== month.value || requested.period !== filters.period) return;
    transactions.value = list;
    options.value = filterOptions;
    loading.value = false;
  }

  watch([month, () => filters.period], () => void load(), { immediate: true });
  watch(
    () => revision.current,
    () => void load({ quiet: true }),
  );

  const filtered = computed(() => transactions.value.filter(matcher(filters)));
  const rows = computed(() =>
    tab.value === 'todas' ? filtered.value : filtered.value.filter((transaction) => transaction.kind === tab.value),
  );
  const tabs = computed(() =>
    TABS.map(({ id, label }) => ({
      id,
      label,
      count: id === 'todas' ? filtered.value.length : filtered.value.filter((transaction) => transaction.kind === id).length,
    })),
  );

  const chips = computed<FilterChip[]>(() => {
    const found: FilterChip[] = [];
    const chip = (label: string, remove: () => void) => found.push({ label, remove });
    const periodChip = PERIOD_CHIPS[filters.period];
    if (periodChip) chip(periodChip, () => (filters.period = 'month'));
    if (filters.account) chip(`Conta: ${filters.account}`, () => (filters.account = null));
    if (filters.line) chip(`Linha: ${filters.line}`, () => (filters.line = null));
    if (filters.tag) chip(`Tag: ${filters.tag}`, () => (filters.tag = null));
    if (filters.establishment) {
      const sample = transactions.value.find((transaction) => transaction.establishmentKey === filters.establishment);
      chip(`Estabelecimento: ${sample?.name ?? filters.establishment}`, () => (filters.establishment = null));
    }
    if (filters.onlyPending) chip('Só pendentes', () => (filters.onlyPending = false));
    if (filters.uncategorised) chip('Sem categoria', () => (filters.uncategorised = false));
    return found;
  });

  const hasFilter = computed(() => filters.query.trim() !== '' || chips.value.length > 0);
  const total = computed(() => totalLine(rows.value));
  const countLabel = computed(() => plural(rows.value.length, 'transação', 'transações'));

  return { transactions, options, loading, tab, filters, rows, tabs, chips, hasFilter, total, countLabel, reload: load };
}

export type TransactionListState = ReturnType<typeof useTransactionList>;
