<script setup lang="ts">
import { computed } from 'vue';
import { brl } from '@/lib/format';

const props = defineProps<{ leftover: number }>();

const text = computed(() =>
  props.leftover >= 0
    ? `Sobraram ${brl(props.leftover)} nas linhas variáveis. Destino: Investimento.`
    : `As linhas variáveis passaram ${brl(props.leftover)} do total. O valor sai do Investimento do mês.`,
);
</script>

<template>
  <span class="text">{{ text }}</span>
</template>

<style scoped>
.text {
  text-wrap: pretty;
}
</style>
