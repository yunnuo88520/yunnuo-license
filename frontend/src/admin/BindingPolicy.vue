<script setup lang="ts">
import { reactive, ref, watch } from "vue";
const props = defineProps<{
  productId: string;
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  canWrite: boolean;
}>();
const form = reactive({
    allowed: false,
    limit: 1,
    cooldown_hours: 24,
    allow_owner_transfer: false,
  }),
  mode = ref(""),
  error = ref(""),
  message = ref(""),
  busy = ref(false);
async function load() {
  error.value = "";
  message.value = "";
  try {
    const result = await props.api(
      `/admin/products/${props.productId}/binding-policy`,
    );
    Object.assign(form, result);
    mode.value = result.bind_mode;
  } catch (e: any) {
    error.value = e.message;
  }
}
watch(() => props.productId, load, { immediate: true });
async function save() {
  busy.value = true;
  error.value = "";
  try {
    await props.api(`/admin/products/${props.productId}/binding-policy`, {
      method: "POST",
      body: JSON.stringify({
        allowed: form.allowed,
        limit: form.limit,
        cooldown_hours: form.cooldown_hours,
        allow_owner_transfer: form.allow_owner_transfer,
      }),
    });
    message.value = "换绑与归属规则已保存并记录审计。";
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section class="binding-policy">
    <h3>换绑与归属规则</h3>
    <p>
      当前绑定类型：{{
        {
          domain: "域名",
          ip: "IP",
          device: "设备",
          account: "应用账号",
          none: "无绑定",
        }[mode] || mode
      }}。换绑不改变授权所属用户。
    </p>
    <form @submit.prevent="save">
      <label
        ><input
          type="checkbox"
          v-model="form.allowed"
          :disabled="!canWrite || mode === 'none'"
        />允许用户更换绑定</label
      ><label class="field"
        >换绑总次数（-1 不限次数，0 禁止）<input
          v-model.number="form.limit"
          type="number"
          min="-1"
          max="10000"
          required
          :disabled="!canWrite" /></label
      ><label class="field"
        >换绑冷却小时<input
          v-model.number="form.cooldown_hours"
          type="number"
          min="0"
          max="87600"
          required
          :disabled="!canWrite" /></label
      ><label
        ><input
          type="checkbox"
          v-model="form.allow_owner_transfer"
          :disabled="!canWrite"
        />允许管理员转移授权所属用户</label
      >
      <p>
        归属转移默认关闭。开启后仍只有管理员能执行，用户和代理不能自行转移。
      </p>
      <button v-if="canWrite" class="btn secondary" :disabled="busy">
        保存换绑规则
      </button>
    </form>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>
  </section>
</template>
<style scoped>
.binding-policy {
  padding-top: 20px;
  margin-top: 24px;
  border-top: 1px solid var(--line);
}
form {
  display: grid;
  gap: 14px;
  max-width: 680px;
  margin: 16px 0;
}
.error {
  color: var(--red);
}
</style>
