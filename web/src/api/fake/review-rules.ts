import { brl } from '@/lib/format';
import type {
  ApprovalRequest,
  ClosingContribution,
  ClosingStep,
  ClosingStepId,
  PlannedActualGroup,
  PlannedActualLine,
  ProposalRecord,
  ProposedChange,
  ReportSection,
  ReviewQueueItem,
  ReviewWarning,
  SubscriptionToReview,
  YearMonth,
} from '../types';
import { budgetFor } from './budget-versions';
import type { FakeBudgetGroup, FakeState, FakeTransaction } from './model';
import { PROTOTYPE_CARD_FULL_LABEL } from './prototype-data';
import { groupTotalOf, pendingCount, spentByLine, toContract } from './rules';
import { addDays, addMonths, suggestedPurchase } from './subscription-rules';
import { newestFirst } from './transactions';

// What the three review tabs show, worked out from the ledger. The browser
// shows these figures and computes none of them.

const cents = (amount: number): number => Math.round(amount * 100) / 100;
const sum = (amounts: number[]): number => amounts.reduce((total, amount) => total + amount, 0);

export const isMonthClosed = (state: FakeState, month: YearMonth): boolean =>
  state.shell.closedMonths.some((closed) => closed.month === month);

export function monthEnd(month: YearMonth): string {
  return addDays(addMonths(`${month}-01`, 1), -1);
}

/** Newest first, ties by entry order. */
export function reviewQueue(state: FakeState): ReviewQueueItem[] {
  return state.transactions
    .filter((transaction) => transaction.pending)
    .sort(newestFirst)
    .flatMap((transaction) => {
      const { pending } = transaction;
      if (!pending) return [];
      return [
        {
          transactionId: transaction.id,
          day: transaction.day,
          name: toContract(state, transaction).name,
          rawDescription: transaction.raw,
          amount: transaction.amount,
          accountLabel: transaction.kind === 'cartao' ? PROTOTYPE_CARD_FULL_LABEL : transaction.account,
          person: transaction.person,
          pending: { ...pending },
        },
      ];
    });
}

export function awaitingApprovals(state: FakeState): ApprovalRequest[] {
  return state.review.awaiting.flatMap((id) => {
    const proposal = state.review.proposals.find((candidate) => candidate.id === id);
    return proposal ? [{ id, title: proposal.title, reason: proposal.reason, proposer: proposal.proposer ?? null }] : [];
  });
}

function proposedChangeOf(effect: FakeState['review']['proposals'][number]['effect']): ProposedChange | null {
  if (effect.kind === 'exclude') return { method: 'transactions.setExclusionReason', arguments: [effect.transactionId, effect.reason] };
  if (effect.kind === 'status') return { method: 'subscriptions.setStatus', arguments: [effect.subscriptionId, effect.status] };
  return null;
}

/** Every proposal in the stand-in, newest first. */
export function proposalRecords(state: FakeState): ProposalRecord[] {
  const defaultSubmittedAt = `${state.shell.today}T05:00:00.000Z`;
  return [...state.review.proposals].reverse().map((proposal) => {
    const waiting = state.review.awaiting.includes(proposal.id);
    const outcome = proposal.outcome ?? (waiting ? 'waiting' : 'waiting');
    return {
      id: proposal.id,
      title: proposal.title,
      change: proposedChangeOf(proposal.effect),
      reason: proposal.reason,
      proposer: proposal.proposer ?? null,
      submittedAt: proposal.submittedAt ?? defaultSubmittedAt,
      outcome,
      answeredAt: proposal.answeredAt ?? null,
      refusal: proposal.refusal ?? null,
      staleReason: proposal.staleReason ?? null,
      changeId: proposal.changeId ?? null,
      undone: false,
    };
  });
}

/** A warning about a recurring charge to confirm goes away once the suggestion has been answered. */
export function visibleWarnings(state: FakeState): ReviewWarning[] {
  return state.review.warnings
    .filter((warning) => warning.kind !== 'new-recurring' || suggestedPurchase(state))
    .map(({ id, text, subscriptionId, actions }) => ({ id, text, subscriptionId, actions: [...actions] }));
}

function differenceOf(planned: number, actual: number, income: boolean): number {
  return cents(income ? actual - planned : planned - actual);
}

const comparesContributions = (group: FakeBudgetGroup): boolean => group.key === 'prov' || group.key === 'res';

/** A closed month is taken as having made every contribution it planned. */
function actualOf(state: FakeState, group: FakeBudgetGroup, closed: boolean, spent: Record<string, number>) {
  return (line: FakeBudgetGroup['lines'][number]): number => {
    if (!comparesContributions(group)) return spent[line.name] ?? 0;
    const made = state.plannedContributions.find((item) => item.name === line.name)?.state === 'done';
    return made || closed ? line.amount : 0;
  };
}

export function plannedActual(state: FakeState, original: FakeTransaction[], month: YearMonth): PlannedActualGroup[] {
  const closed = isMonthClosed(state, month);
  const spent = spentByLine(state, original, month);
  return budgetFor(state, month).map((group) => {
    const income = group.key === 'rec';
    const actualFor = actualOf(state, group, closed, spent);
    const lines: PlannedActualLine[] = group.lines.map((line) => {
      const actual = actualFor(line);
      return { name: line.name, planned: line.amount, actual, difference: differenceOf(line.amount, actual, income) };
    });
    const planned = sum(group.lines.map((line) => line.amount));
    const actual = cents(sum(group.lines.map(actualFor)));
    return {
      name: group.name,
      planned,
      actual,
      difference: differenceOf(planned, actual, income),
      contributions: comparesContributions(group),
      lines,
    };
  });
}

/** What the variable lines spent in the month. */
export function variableSpent(state: FakeState, original: FakeTransaction[], month: YearMonth): number {
  const spent = spentByLine(state, original, month);
  const lines = budgetFor(state, month).find((group) => group.key === 'var')?.lines ?? [];
  return cents(sum(lines.map((line) => spent[line.name] ?? 0)));
}

export function leftover(state: FakeState, original: FakeTransaction[], month: YearMonth): number {
  return cents(groupTotalOf(budgetFor(state, month), 'var') - variableSpent(state, original, month));
}

export function closingContributions(state: FakeState, month: YearMonth): ClosingContribution[] {
  const closed = isMonthClosed(state, month);
  return state.plannedContributions.map((item) => {
    if (item.state === 'on-target') return { name: item.name, amount: item.amount, state: 'on-target' };
    return { name: item.name, amount: item.amount, state: closed || item.state === 'done' ? 'done' : 'not-found' };
  });
}

/** Subscriptions whose review date has come by the end of the month and that are not already being cancelled. */
export function subscriptionsToReview(state: FakeState, month: YearMonth): SubscriptionToReview[] {
  const end = monthEnd(month);
  return state.subscriptions
    .filter((subscription) => subscription.reviewOn <= end && subscription.status !== 'Cancelar')
    .map(({ id, name, amount, cycle, reviewOn }) => ({ id, name, amount, cycle, reviewOn }));
}

const STEP_ORDER: ClosingStepId[] = [
  'pending',
  'planned-actual',
  'leftover',
  'contributions',
  'card-bill',
  'subscriptions',
  'report',
  'close',
];

/** The steps a person marks by hand; the first follows the pending count and the last is closing itself. */
export const MARKABLE_STEPS: ClosingStepId[] = STEP_ORDER.slice(1, 7);

export function closingSteps(state: FakeState, month: YearMonth): ClosingStep[] {
  const closed = isMonthClosed(state, month);
  const marked = state.review.stepsDone[month] ?? [];
  return STEP_ORDER.map((id) => {
    const markable = !closed && MARKABLE_STEPS.includes(id);
    if (closed) return { id, state: 'done', markable };
    if (id === 'pending') return { id, state: pendingCount(state) === 0 ? 'done' : 'pending', markable };
    return { id, state: marked.includes(id) ? 'done' : 'todo', markable };
  });
}

/** The text the model wrote about the month. Figures in it come from the same rules as the tables above. */
export function monthReport(state: FakeState, original: FakeTransaction[], month: YearMonth): ReportSection[] {
  if (state.review.report) return structuredClone(state.review.report);
  const cap = groupTotalOf(budgetFor(state, month), 'var');
  const spent = variableSpent(state, original, month);
  const sent = (id: string | null) => !!id && state.review.awaiting.includes(id);
  const text = (value: string, proposalId: string | null = null) => ({ text: value, proposalId, sent: sent(proposalId) });
  return [
    {
      heading: 'Orçado × realizado',
      paragraphs: [
        text(
          `As linhas variáveis somam ${brl(spent)} de ${brl(cap)}. Fixos e essenciais ficaram perto do esperado, com Contas da casa R$ 59,90 acima por causa da internet.`,
        ),
      ],
    },
    {
      heading: 'Onde dá para cortar',
      paragraphs: [
        text('Delivery passou do teto pelo terceiro mês seguido. Dois planos do Assistente de IA foram cobrados no mesmo mês: um deles parece sobrar.'),
      ],
    },
    {
      heading: 'Fora do padrão',
      paragraphs: [
        text('Farmácia São João: R$ 212,40, quatro vezes a sua média. Pousada Serra Azul: R$ 420,00, coberta pela reserva Viagens.'),
      ],
    },
    {
      heading: 'Sugestões de teto',
      paragraphs: [
        text('Lazer fechou bem abaixo do teto nos dois últimos meses. Baixar de R$ 400 para R$ 300 liberaria R$ 100 para investimento.', 'lazer'),
        text('Delivery vem estourando. Subir o teto de R$ 200 para R$ 250 deixaria o budget mais perto do que acontece.', 'teto'),
      ],
    },
    {
      heading: 'Comparado aos meses anteriores',
      paragraphs: [
        text('Só outubro está fechado. Em relação a ele, Mercado caiu R$ 80,00 e Lazer caiu R$ 306,90. A comparação ganha um mês a cada fechamento.'),
      ],
    },
  ];
}
