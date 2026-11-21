<script setup lang="ts">
// Stands in for a screen that has not been ported from the design yet, so the
// navigation around it can already be used and checked.
defineProps<{ title: string }>();
</script>

<template>
  <div class="screen">
    <h1 class="screen-title">{{ title }}</h1>
  </div>
</template>
