<script setup lang="ts">
import { X } from "lucide-vue-next";
import { onBeforeUnmount, watch } from "vue";

const props = defineProps<{
  open: boolean;
  title: string;
  description?: string;
}>();
const emit = defineEmits<{ close: [] }>();

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") emit("close");
}
watch(
  () => props.open,
  (open) => {
    if (open) addEventListener("keydown", onKeydown);
    else removeEventListener("keydown", onKeydown);
  },
);
onBeforeUnmount(() => removeEventListener("keydown", onKeydown));
</script>
<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="open"
        class="modal-backdrop"
        @click.self="emit('close')"
      >
        <div class="modal-card" role="dialog" aria-modal="true" :aria-label="title">
          <div class="modal-head">
            <div>
              <h3>{{ title }}</h3>
              <p v-if="description">{{ description }}</p>
            </div>
            <button class="icon-btn" type="button" title="关闭" @click="emit('close')">
              <X :size="17" />
            </button>
          </div>
          <div class="modal-body">
            <slot />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
