<script setup lang="ts">
import { reactive, ref } from "vue";
import { friendlyError } from "../errors";
import type { PasswordChangeInput } from "../password";

const props = withDefaults(defineProps<{
  change: (input: PasswordChangeInput) => Promise<{ changed: boolean }>;
  submitLabel?: string;
}>(), { submitLabel: "保存并重新登录" });
const emit = defineEmits<{ changed: [] }>();
const fields = reactive<PasswordChangeInput>({ current_password: "", new_password: "", confirm_password: "" });
const busy = ref(false);
const error = ref("");

async function submit() {
  if (busy.value) return;
  error.value = "";
  if (fields.new_password !== fields.confirm_password) {
    error.value = "两次输入的新密码不一致";
    return;
  }
  busy.value = true;
  try {
    const result = await props.change({ ...fields });
    if (result.changed !== true) throw new Error("密码未修改，请稍后重试");
    Object.assign(fields, { current_password: "", new_password: "", confirm_password: "" });
    emit("changed");
  } catch (cause) {
    error.value = friendlyError(cause);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <form class="password-change-form" @submit.prevent="submit">
    <label class="field">当前密码<input v-model="fields.current_password" name="current_password"
      type="password" autocomplete="current-password" required :disabled="busy" /></label>
    <label class="field">新密码<input v-model="fields.new_password" name="new_password"
      type="password" autocomplete="new-password" minlength="8" maxlength="256" required :disabled="busy" /></label>
    <label class="field">确认新密码<input v-model="fields.confirm_password" name="confirm_password"
      type="password" autocomplete="new-password" minlength="8" maxlength="256" required :disabled="busy" /></label>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p class="password-change-note">修改成功后，原登录会话会退出，请使用新密码重新登录。</p>
    <div class="form-actions"><button class="btn" :disabled="busy">{{ busy ? "正在修改…" : submitLabel }}</button></div>
  </form>
</template>

<style scoped>
.password-change-form { display: grid; gap: 16px; max-width: 520px; }
.password-change-note { margin: 0; color: var(--muted); font-size: 13px; line-height: 1.7; }
.password-change-form .error { margin: 0; }
.modal-body .password-change-form { max-width: none; padding: 20px; }
.password-change-form .form-actions { justify-content: flex-end; }
</style>
