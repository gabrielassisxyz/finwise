import { computed, inject, provide, ref, watch, type InjectionKey, type Ref } from 'vue';
import { useApi } from '@/api/context';
import type { ChangeResult, SplitPart, TransactionDetail, TransactionFilterOptions, YearMonth } from '@/api/types';
import { useLineChange } from '@/components/transactions/useLineChange';
import { createLatestLoad } from '@/lib/latestLoad';
import { useOverlayStore } from '@/stores/overlay';
import { usePanelStore } from '@/stores/panel';
import { useRevisionStore } from '@/stores/revision';
import { useShellStore } from '@/stores/shell';

interface PanelMenus {
  month: YearMonth;
  ofTransaction: TransactionFilterOptions;
  ofCurrent: TransactionFilterOptions;
}

/**
 * The line menus of the open transaction. Its own line, a split part's and the refund's are the lines of its
 * month's budget; the establishment's default line belongs to no month and is resolved in the current one.
 */
function usePanelMenus(month: Ref<YearMonth | null>) {
  const api = useApi();
  const shell = useShellStore();
  const revision = useRevisionStore();
  const currentMonth = computed(() => shell.overview?.today.slice(0, 7) ?? null);
  const menus = ref<PanelMenus | null>(null);

  const loads = createLatestLoad();
  async function load(): Promise<void> {
    const [transaction, current] = [month.value, currentMonth.value];
    if (!transaction || !current) {
      loads.cancel();
      menus.value = null;
      return;
    }
    const isLatest = loads.start();
    const months = [...new Set([transaction, current])];
    const answers = await Promise.all(months.map((asked) => api.transactions.filterOptions(asked)));
    // A slower answer for a transaction or a revision the panel already left must not land.
    if (!isLatest()) return;
    const answerFor = (asked: YearMonth) => answers[months.indexOf(asked)]!;
    menus.value = { month: transaction, ofTransaction: answerFor(transaction), ofCurrent: answerFor(current) };
  }
  watch([month, currentMonth, () => revision.current], () => void load(), { immediate: true });

  // Until the menus of the open transaction's month arrived, a menu would open empty or on another month's lines.
  const ready = computed(() => month.value !== null && menus.value?.month === month.value);
  const held = (pick: (loaded: PanelMenus) => TransactionFilterOptions) =>
    computed(() => (ready.value && menus.value ? pick(menus.value) : { accounts: [], tags: [], lines: [] }));
  return { ready, ofTransaction: held((loaded) => loaded.ofTransaction), ofCurrent: held((loaded) => loaded.ofCurrent) };
}

/**
 * What the panel's sections can do to the open transaction. Every write is
 * followed by a revision bump so the list, the summary and the detail itself
 * fetch again; only a change of line toasts, as in the design.
 */
export function createPanelActions(month: Ref<YearMonth | null>) {
  const api = useApi();
  const overlay = useOverlayStore();
  const panel = usePanelStore();
  const revision = useRevisionStore();

  const menus = usePanelMenus(month);
  const lineChange = useLineChange(computed(() => menus.ofTransaction.value.lines));
  const defaultLineChange = useLineChange(computed(() => menus.ofCurrent.value.lines));

  const id = (): string => panel.openId ?? '';

  async function write(result: Promise<ChangeResult | null>): Promise<void> {
    await result;
    revision.bump();
  }

  function pickLineMenu(anchor: HTMLElement, current: string | null, pick: (line: string) => Promise<void>, from = lineChange): void {
    from.openLineMenu(anchor, current, (line) => void pick(line));
  }

  const splitFrom = (detail: TransactionDetail): SplitPart[] => detail.splitParts ?? [];
  const setSplit = (parts: SplitPart[] | null) => write(api.transactions.setSplit(id(), parts));

  return {
    menusReady: menus.ready,
    confirm: () => write(api.transactions.confirmLine(id())),
    pickLine: (anchor: HTMLElement, detail: TransactionDetail) => lineChange.pickLine(anchor, detail),
    setReason: (reason: string) => write(api.transactions.setReason(id(), reason)),

    saveNickname: (nickname: string) => write(api.transactions.setNickname(id(), nickname)),
    pickDefaultLine: (anchor: HTMLElement, current: string | null) =>
      pickLineMenu(anchor, current, (line) => write(api.transactions.setDefaultLine(id(), line)), defaultLineChange),
    removeFixedTag: (tag: string) => write(api.transactions.removeFixedTag(id(), tag)),
    pickFixedTag(anchor: HTMLElement, detail: TransactionDetail): void {
      const taken = detail.counterparty?.fixedTags ?? [];
      overlay.openMenu(anchor, {
        title: `Tag fixa para ${detail.name}`,
        items: menus.ofTransaction.value.tags
          .filter((tag) => !taken.includes(tag))
          .map((tag) => ({ label: tag, onPick: () => void write(api.transactions.addFixedTag(id(), tag)) })),
      });
    },

    addTag: (tag: string) => write(api.transactions.addTag(id(), tag)),
    removeTag: (tag: string) => write(api.transactions.removeTag(id(), tag)),
    setNote: (note: string) => write(api.transactions.setNote(id(), note)),

    toggleSplit: (detail: TransactionDetail) =>
      setSplit(
        detail.splitParts
          ? null
          : [
              { line: detail.line, amount: Math.abs(detail.amount) },
              { line: null, amount: 0 },
            ],
      ),
    pickSplitLine(anchor: HTMLElement, detail: TransactionDetail, index: number): void {
      const parts = splitFrom(detail);
      pickLineMenu(anchor, parts[index]?.line ?? null, (line) =>
        setSplit(parts.map((part, at) => (at === index ? { ...part, line } : part))),
      );
    },
    setSplitAmount(detail: TransactionDetail, index: number, amount: number) {
      return setSplit(splitFrom(detail).map((part, at) => (at === index ? { ...part, amount } : part)));
    },
    removeSplitPart(detail: TransactionDetail, index: number) {
      const parts = splitFrom(detail);
      return setSplit(parts.length > 2 ? parts.filter((_, at) => at !== index) : null);
    },
    addSplitPart: (detail: TransactionDetail) =>
      setSplit([...splitFrom(detail), { line: null, amount: Math.max(0, detail.splitRemaining) }]),

    setReimbursable: (on: boolean) => write(api.transactions.setReimbursable(id(), on)),
    setExpected: (amount: number | null) => write(api.transactions.setExpectedReimbursement(id(), amount)),
    link: (creditId: string) => write(api.transactions.linkReimbursement(id(), creditId)),
    unlink: () => write(api.transactions.unlinkReimbursement(id())),

    setExcluded: (on: boolean) => write(api.transactions.setExcluded(id(), on)),
    setExclusionReason: (reason: string) => write(api.transactions.setExclusionReason(id(), reason)),

    setRefund: (on: boolean) => write(api.transactions.setRefund(id(), on)),
    pickRefundLine: (anchor: HTMLElement, current: string | null) =>
      pickLineMenu(anchor, current, (line) => write(api.transactions.setRefundLine(id(), line))),

    undoStep: (stepId: string) => write(api.transactions.undoHistoryStep(id(), stepId)),
  };
}

export type PanelActions = ReturnType<typeof createPanelActions>;

const panelActionsKey: InjectionKey<PanelActions> = Symbol('panel-actions');

/** `month` is the month of the open transaction, null while none is open. */
export function providePanelActions(month: Ref<YearMonth | null>): PanelActions {
  const actions = createPanelActions(month);
  provide(panelActionsKey, actions);
  return actions;
}

export function usePanelActions(): PanelActions {
  const actions = inject(panelActionsKey);
  if (!actions) throw new Error('usePanelActions needs a TransactionPanel above it');
  return actions;
}
