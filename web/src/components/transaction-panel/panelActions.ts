import { computed, inject, onMounted, provide, ref, type InjectionKey } from 'vue';
import { useApi } from '@/api/context';
import type { ChangeResult, SplitPart, TransactionDetail, TransactionFilterOptions } from '@/api/types';
import { useLineChange } from '@/components/transactions/useLineChange';
import { useOverlayStore } from '@/stores/overlay';
import { usePanelStore } from '@/stores/panel';
import { useRevisionStore } from '@/stores/revision';

/**
 * What the panel's sections can do to the open transaction. Every write is
 * followed by a revision bump so the list, the summary and the detail itself
 * fetch again; only a change of line toasts, as in the design.
 */
export function createPanelActions() {
  const api = useApi();
  const overlay = useOverlayStore();
  const panel = usePanelStore();
  const revision = useRevisionStore();

  const options = ref<TransactionFilterOptions>({ accounts: [], tags: [], lines: [] });
  onMounted(async () => {
    options.value = await api.transactions.filterOptions();
  });
  const lineChange = useLineChange(computed(() => options.value.lines));

  const id = (): string => panel.openId ?? '';

  async function write(result: Promise<ChangeResult | null>): Promise<void> {
    await result;
    revision.bump();
  }

  function pickLineMenu(anchor: HTMLElement, current: string | null, pick: (line: string) => Promise<void>): void {
    lineChange.openLineMenu(anchor, current, (line) => void pick(line));
  }

  const splitFrom = (detail: TransactionDetail): SplitPart[] => detail.splitParts ?? [];
  const setSplit = (parts: SplitPart[] | null) => write(api.transactions.setSplit(id(), parts));

  return {
    confirm: () => write(api.transactions.confirmLine(id())),
    pickLine: (anchor: HTMLElement, detail: TransactionDetail) => lineChange.pickLine(anchor, detail),
    setReason: (reason: string) => write(api.transactions.setReason(id(), reason)),

    saveNickname: (nickname: string) => write(api.transactions.setNickname(id(), nickname)),
    pickDefaultLine: (anchor: HTMLElement, current: string | null) =>
      pickLineMenu(anchor, current, (line) => write(api.transactions.setDefaultLine(id(), line))),
    removeFixedTag: (tag: string) => write(api.transactions.removeFixedTag(id(), tag)),
    pickFixedTag(anchor: HTMLElement, detail: TransactionDetail): void {
      const taken = detail.counterparty?.fixedTags ?? [];
      overlay.openMenu(anchor, {
        title: `Tag fixa para ${detail.name}`,
        items: options.value.tags
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

export function providePanelActions(): void {
  provide(panelActionsKey, createPanelActions());
}

export function usePanelActions(): PanelActions {
  const actions = inject(panelActionsKey);
  if (!actions) throw new Error('usePanelActions needs a TransactionPanel above it');
  return actions;
}
