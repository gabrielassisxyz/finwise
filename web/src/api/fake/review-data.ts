import type { FakeReview } from './review-model';

// The review screens' own dataset: the proposals and warnings of the design
// prototype. Every figure and name is invented.

export function prototypeReview(): FakeReview {
  return {
    proposals: [
      {
        id: 'teto',
        title: 'Subir o teto de Delivery de R$ 200 para R$ 250',
        reason: 'estourou três meses seguidos',
        effect: { kind: 'cap', line: 'Delivery', amount: 250 },
      },
      {
        id: 'estorno',
        title: 'Marcar como "não conta": Estorno de anuidade R$ 120,00',
        reason: 'é a devolução de uma tarifa, não uma receita',
        effect: { kind: 'exclude', transactionId: 't10', reason: 'Estorno de anuidade' },
      },
      {
        id: 'revista',
        title: 'Mudar status de Revista digital para Cancelar',
        reason: 'revisão vencida há dois meses',
        effect: { kind: 'status', subscriptionId: 'rv', status: 'Cancelar' },
      },
      {
        id: 'lazer',
        title: 'Baixar o teto de Lazer de R$ 400 para R$ 300',
        reason: 'sobrou mais de R$ 100 nos últimos dois meses',
        effect: { kind: 'cap', line: 'Lazer', amount: 300 },
      },
    ],
    awaiting: ['teto', 'estorno', 'revista'],
    warnings: [
      {
        id: 'preco',
        kind: 'price',
        text: 'Preço subiu: Spotify de R$ 19,90 para R$ 21,90',
        subscriptionId: 'sp',
        actions: [],
      },
      {
        id: 'nova',
        kind: 'new-recurring',
        text: 'Nova cobrança recorrente: Serviço de nuvem · parece assinatura?',
        subscriptionId: null,
        actions: [
          { id: 'accept-suggestion', label: 'Sim' },
          { id: 'reject-suggestion', label: 'Não' },
        ],
      },
      {
        id: 'sobre',
        kind: 'overlap',
        text: 'Cobrança sobreposta: dois planos do Assistente de IA no mesmo mês',
        subscriptionId: 'ai',
        actions: [{ id: 'silence-pair', label: 'Silenciar este par' }],
      },
      {
        id: 'renov',
        kind: 'renewal',
        text: 'Renovação anual em 9 dias: Amazon Prime · R$ 166,80',
        subscriptionId: 'ap',
        actions: [],
      },
      {
        id: 'cobrou',
        kind: 'charged-after-cancel',
        text: 'Marcada para cancelar e cobrou de novo: Editor de fotos',
        subscriptionId: null,
        actions: [],
      },
    ],
    resolved: 0,
    stepsDone: {},
  };
}
