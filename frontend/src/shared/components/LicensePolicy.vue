<script setup lang="ts">
import { reactive, ref, watch } from "vue";
const props = defineProps<{
  productId: string;
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  prefix: "/admin" | "/developer";
  canWrite: boolean;
}>();
const emit = defineEmits<{ saved: [] }>();
const form = reactive({
  license_mode: "subscription",
  update_duration_days: 0,
  expire_grace_days: 0,
});
const locked = ref(true),
  busy = ref(false),
  error = ref("");
let generation = 0;
const path = () =>
  `${props.prefix}/products/${encodeURIComponent(props.productId)}/license-policy`;
async function load() {
  const current = ++generation;
  locked.value = true;
  error.value = "";
  if (!props.productId) return;
  try {
    const result = await props.api(path());
    if (current !== generation) return;
    Object.assign(form, result);
    locked.value = result.locked;
  } catch (e: any) {
    if (current === generation) error.value = e.message;
  }
}
watch(() => props.productId, load, { immediate: true });
async function save() {
  busy.value = true;
  error.value = "";
  try {
    await props.api(path(), {
      method: "POST",
      body: JSON.stringify({
        license_mode: form.license_mode,
        update_duration_days: form.update_duration_days,
        expire_grace_days: form.expire_grace_days,
      }),
    });
    await load();
    emit("saved");
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section v-if="productId" class="policy-panel">
    <h3>使用与更新期限</h3>
    <p>
      订阅使用：使用到期后停用。永久使用：保留已有版本，更新资格按期限计算。
    </p>
    <p v-if="locked">
      此产品已发卡或创建授权，期限规则已锁定。需要不同销售规则时请创建新产品。
    </p>
    <form class="policy-form" @submit.prevent="save">
      <label class="field"
        >授权模式<select
          v-model="form.license_mode"
          :disabled="locked || !canWrite || busy"
        >
          <option value="subscription">订阅使用（到期停用）</option>
          <option value="perpetual">永久使用、限期更新</option>
        </select></label
      >
      <label class="field"
        >每次发卡/续期的更新天数<input
          v-model.number="form.update_duration_days"
          type="number"
          min="0"
          max="36500"
          required
          :disabled="locked || !canWrite || busy"
      /></label>
      <small
        >0 表示跟随卡密期限；永久卡在此设置为 0
        时包含永久更新。订阅的更新资格最晚在使用到期时结束。</small
      >
      <label class="field"
        >使用到期宽限天数<input
          v-model.number="form.expire_grace_days"
          type="number"
          min="0"
          max="365"
          required
          :disabled="
            locked || !canWrite || busy || form.license_mode === 'perpetual'
          "
      /></label>
      <small>0 表示到期即停用。使用宽限期不会延长更新资格。</small>
      <button v-if="canWrite && !locked" class="btn secondary" :disabled="busy">
        保存期限规则
      </button>
    </form>
    <p v-if="error" role="alert" class="error">{{ error }}</p>
  </section>
</template>
<style scoped>
.policy-panel {
  border-top: 1px solid var(--line);
  padding-top: 20px;
  margin-top: 24px;
}
.policy-form {
  display: grid;
  gap: 14px;
  max-width: 650px;
  margin: 20px 0;
}
</style>
