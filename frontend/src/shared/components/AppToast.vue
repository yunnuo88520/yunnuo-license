<script setup lang="ts">
import { CircleAlert } from "lucide-vue-next";
import { toastMessage } from "../useToast";
</script>
<template>
  <Transition name="toast"
    ><div v-if="toastMessage" class="toast" role="status">
      <CircleAlert :size="17" />{{ toastMessage }}
    </div></Transition
  >
</template>
