import { defineStore } from 'pinia';
import { ref } from 'vue';

export type SubscriptionsTabId = 'assinaturas' | 'recorrentes';

/** What the subscription screens keep while the user opens a subscription and comes back. */
export const useSubscriptionsViewStore = defineStore('subscriptions-view', () => {
  const tab = ref<SubscriptionsTabId>('assinaturas');
  const grouped = ref(false);

  return { tab, grouped };
});
