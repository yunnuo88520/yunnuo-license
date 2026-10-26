<script setup lang="ts">
import { onMounted, ref } from "vue";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
}>();
const emit = defineEmits<{ (e: "signed-out"): void }>();
const state = ref<any>(),
  password = ref(""),
  code = ref(""),
  secret = ref(""),
  codes = ref<string[]>([]),
  error = ref(""),
  busy = ref(false);
async function load() {
  try {
    state.value = await props.api("/admin/mfa");
  } catch (e: any) {
    error.value = e.message;
  }
}
onMounted(load);
async function act(action: string) {
  busy.value = true;
  error.value = "";
  try {
    const result = await props.api("/admin/mfa/" + action, {
      method: "POST",
      body: JSON.stringify({ password: password.value, code: code.value }),
    });
    if (action === "begin") {
      secret.value = result.secret;
    } else {
      secret.value = "";
      password.value = "";
      code.value = "";
      if (action === "confirm") {
        codes.value = result.recovery_codes;
        state.value = { enabled: true };
      } else {
        emit("signed-out");
      }
    }
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <div class="mfa-card">
    <h4>管理员动态验证</h4>
    <p>
      使用验证器的六位动态码保护登录。启用或关闭后，已有登录会话全部失效。丢失验证器时使用一次性恢复码。
    </p>
    <p v-if="error" role="alert" class="error">{{ error }}</p>
    <template v-if="codes.length"
      ><p class="notice">
        动态验证已启用。请离线保存以下恢复码，每个只能使用一次；关闭此页后不会再次展示。
      </p>
      <pre class="backup-codes">{{ codes.join("\n") }}</pre>
      <button class="btn" @click="emit('signed-out')">
        已保存恢复码，重新登录
      </button></template
    >
    <form
      v-else-if="state"
      @submit.prevent="
        act(state.enabled ? 'disable' : secret ? 'confirm' : 'begin')
      "
    >
      <p>
        {{
          state.enabled
            ? "已启用 · 剩余 " + state.recovery_remaining + " 个恢复码"
            : "尚未启用，建议公网部署前开启"
        }}
      </p>
      <label class="field"
        >当前登录密码<input
          v-model="password"
          type="password"
          autocomplete="current-password"
          required /></label
      ><template v-if="secret"
        ><p>
          在验证器中手动添加账户：名称 Yunnuo，时间型 TOTP、六位、30
          秒。安装密钥十分钟内有效。
        </p>
        <label class="field"
          >验证器安装密钥<input :value="secret" readonly /></label></template
      ><label v-if="secret || state.enabled" class="field"
        >验证器动态码或恢复码<input
          v-model="code"
          autocomplete="one-time-code"
          maxlength="32"
          required /></label
      ><button class="btn" :disabled="busy">
        {{
          busy
            ? "处理中…"
            : state.enabled
              ? "关闭动态验证"
              : secret
                ? "验证并启用"
                : "配置动态验证"
        }}
      </button>
    </form>
  </div>
</template>
<style scoped>
.mfa-card {
  padding: 24px;
  border-bottom: 1px solid var(--line);
}
.mfa-card p {
  color: var(--muted);
  line-height: 1.8;
}
.mfa-card form {
  display: grid;
  gap: 16px;
  max-width: 520px;
}
.backup-codes {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  padding: 18px;
  background: var(--surface-alt);
}
</style>
