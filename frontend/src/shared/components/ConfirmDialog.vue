<script setup lang="ts">
import { AlertTriangle, X } from "lucide-vue-next";
import { onBeforeUnmount, ref, watch } from "vue";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    description?: string;
    confirmText?: string;
    requireReason?: boolean;
    reasonLabel?: string;
    reasonPlaceholder?: string;
  }>(),
  {
    confirmText: "确认执行",
    requireReason: false,
    reasonLabel: "操作原因",
    reasonPlaceholder: "请填写原因，将记入审计日志",
  },
);
const emit = defineEmits<{ close: []; confirm: [reason: string] }>();
const reason = ref("");

function submit() {
  if (props.requireReason && reason.value.trim() === "") return;
  emit("confirm", reason.value.trim());
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") emit("close");
}
watch(
  () => props.open,
  (open) => {
    if (open) {
      reason.value = "";
      addEventListener("keydown", onKeydown);
    } else {
      removeEventListener("keydown", onKeydown);
    }
  },
);
onBeforeUnmount(() => removeEventListener("keydown", onKeydown));
</script>
<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="open" class="modal-backdrop" @click.self="emit('close')">
        <div class="modal-card confirm-card" role="dialog" aria-modal="true" :aria-label="title">
          <div class="modal-head">
            <div class="confirm-head">
              <span class="confirm-icon"><AlertTriangle :size="18" /></span>
              <div>
                <h3>{{ title }}</h3>
                <p v-if="description">{{ description }}</p>
              </div>
            </div>
            <button class="icon-btn" type="button" title="关闭" @click="emit('close')">
              <X :size="17" />
            </button>
          </div>
          <div class="modal-body">
            <label v-if="requireReason" class="field">
              <span>{{ reasonLabel }}<i class="required">*</i></span>
              <textarea
                v-model="reason"
                rows="2"
                maxlength="200"
                :placeholder="reasonPlaceholder"
                @keydown.enter.prevent="submit"
              />
            </label>
            <p v-else class="confirm-note">该操作不可撤销，请确认后继续。</p>
            <div class="confirm-actions">
              <button class="btn secondary" type="button" @click="emit('close')">取消</button>
              <button
                class="btn danger"
                type="button"
                :disabled="requireReason && reason.trim() === ''"
                @click="submit"
              >
                {{ confirmText }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
