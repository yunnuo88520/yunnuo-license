<script setup lang="ts">
import { onMounted, ref } from "vue";
import { RefreshCw } from "lucide-vue-next";
import { request } from "../api";

const props = withDefaults(
  defineProps<{ modelValue: string; endpoint?: string }>(),
  { endpoint: "/v1/captcha" },
);
const emit = defineEmits<{ "update:modelValue": [string] }>();

const enabled = ref(false);
const captchaId = ref("");
const image = ref("");
const busy = ref(false);

async function refresh() {
  if (busy.value) return;
  busy.value = true;
  try {
    const r = await request<any>(props.endpoint);
    enabled.value = !!r?.enabled;
    captchaId.value = r?.id || "";
    image.value = r?.image || "";
    emit("update:modelValue", "");
  } catch {
    enabled.value = false;
  } finally {
    busy.value = false;
  }
}
onMounted(refresh);
defineExpose({ captchaId, refresh });
</script>
<template>
  <label v-if="enabled" class="field captcha-field">
    <span>图形验证码</span>
    <div class="captcha-row">
      <input
        :value="modelValue"
        maxlength="4"
        autocomplete="off"
        placeholder="不区分大小写"
        required
        @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
      /><img
        v-if="image"
        :src="image"
        alt="图形验证码"
        title="看不清？点击刷新"
        @click="refresh"
      /><button
        type="button"
        class="captcha-refresh"
        :disabled="busy"
        aria-label="刷新验证码"
        @click="refresh"
      >
        <RefreshCw :size="15" :class="{ spin: busy }" />
      </button>
    </div>
  </label>
</template>
<style scoped>
.captcha-row {
  display: flex;
  gap: 8px;
  align-items: stretch;
}
.captcha-row input {
  flex: 1;
  min-width: 0;
}
.captcha-row img {
  height: 42px;
  border: 1px solid var(--line);
  border-radius: var(--radius-sm, 8px);
  cursor: pointer;
  flex: none;
}
.captcha-refresh {
  flex: none;
  width: 38px;
  display: grid;
  place-items: center;
  border: 1px solid var(--line);
  border-radius: var(--radius-sm, 8px);
  background: var(--field, transparent);
  cursor: pointer;
  color: var(--muted);
}
.captcha-refresh:hover {
  color: var(--ink, inherit);
}
.spin {
  animation: captcha-spin 0.8s linear infinite;
}
@keyframes captcha-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
