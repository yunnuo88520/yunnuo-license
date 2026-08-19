<script setup lang="ts">
import { statusLabel } from "../format";
defineProps<{ status?: string }>();
</script>
<template>
  <span class="status" :class="status">{{ statusLabel(status) }}</span>
</template>
