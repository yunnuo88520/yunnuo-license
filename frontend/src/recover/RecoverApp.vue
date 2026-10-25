<script setup lang="ts">
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import { onMounted, reactive, ref } from "vue";
import { request } from "../shared/api";
import { entries } from "../shared/entries";
import { branding, loadBranding } from "../shared/branding";
const form = reactive({ username: "", code: "", password: "", confirm: "" }),
  busy = ref(false),
  error = ref(""),
  done = ref(false),
  requesting = ref(false),
  requestTip = ref("");
async function requestCode() {
  if (requesting.value || !form.username.trim()) return;
  requesting.value = true;
  requestTip.value = "";
  try {
    const r = await request<any>("/v1/accounts/recovery/request", {
      method: "POST",
      body: JSON.stringify({ username: form.username.trim() }),
    });
    requestTip.value = r.message;
  } catch (e: any) {
    requestTip.value = e.message;
  } finally {
    requesting.value = false;
  }
}
// 从哪个登录页进来就回到哪个登录页；直接访问时回首页。
const fromKey = new URLSearchParams(location.search).get("from") || "";
const backURL =
  ({ admin: entries.value.admin, agent: entries.value.agent, user: entries.value.user, developer: entries.value.developer } as Record<string, string>)[fromKey] || "/";
onMounted(() => loadBranding("密码恢复").catch(() => {}));
async function submit() {
  error.value = "";
  if (form.password !== form.confirm) {
    error.value = "两次输入的密码不一致。";
    return;
  }
  if (/^\d{6}$/.test(form.code.trim()) && !form.username.trim()) {
    error.value = "使用 6 位邮箱验证码时，请填写登录账号；长恢复码无需填写。";
    return;
  }
  busy.value = true;
  try {
    await request("/v1/accounts/recovery", {
      method: "POST",
      body: JSON.stringify({
        username: form.username.trim(),
        code: form.code,
        password: form.password,
      }),
    });
    form.username = "";
    form.code = "";
    form.password = "";
    form.confirm = "";
    done.value = true;
  } catch (e: any) {
    error.value =
      e.code === "RECOVERY_INVALID"
        ? "恢复码或验证码无效、已过期或已使用，请联系管理员重新核实。"
        : e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <main>
    <header class="recovery-header">
      <a href="/" class="brand">{{ branding.site_name }}</a>
      <span class="recovery-header-right">
        <a :href="backURL" class="back-link">← 返回登录</a>
        <AppearanceMenu />
      </span>
    </header>
    <section class="panel">
      <span class="eyebrow">账号恢复</span>
      <h1>{{ done ? "密码已重置" : "重新设置登录密码" }}</h1>
      <p v-if="!done">输入登录账号获取邮箱验证码；持管理员签发的恢复码可直接使用。</p>
      <p v-else>原登录会话已失效，请使用新密码重新登录。</p>
      <p v-if="error" role="alert" class="error">{{ error }}</p>
      <form v-if="!done" @submit.prevent="submit">
        <label class="field"
          >登录账号<div class="recover-request"
            ><input
              v-model="form.username"
              autocomplete="username"
              maxlength="80"
              placeholder="登录账号"
          /><button
              type="button"
              class="btn secondary"
              :disabled="requesting || !form.username.trim()"
              @click="requestCode"
            >
              {{ requesting ? "发送中…" : "发送验证码" }}
            </button></div
          ></label
        ><p v-if="requestTip" role="status" class="recover-tip">
          {{ requestTip }}
        </p>
        <label class="field"
          >恢复码 / 验证码<input
            v-model="form.code"
            type="password"
            autocomplete="off"
            required
            maxlength="100" /></label
        ><label class="field"
          >新密码<input
            v-model="form.password"
            type="password"
            autocomplete="new-password"
            required
            minlength="8"
            maxlength="256" /></label
        ><label class="field"
          >再次输入新密码<input
            v-model="form.confirm"
            type="password"
            autocomplete="new-password"
            required
            minlength="8"
            maxlength="256" /></label
        ><button class="btn" :disabled="busy">
          {{ busy ? "正在重置…" : "确认重置密码" }}
        </button>
      </form>
      <nav class="recovery-nav">
        <a class="btn" :href="backURL">{{
          done ? "使用新密码返回登录" : "返回登录"
        }}</a>
      </nav>
    </section>
  </main>
</template>
<style scoped>
main {
  max-width: 620px;
  margin: 60px auto;
  padding: 24px;
}
.brand {
  font-size: 19px;
  font-weight: 600;
}
.panel {
  padding: 32px;
  margin-top: 28px;
  border-radius: 16px;
}
h1 {
  font-size: 27px;
  margin: 12px 0;
}
p {
  color: var(--muted);
  line-height: 1.8;
}
form {
  display: grid;
  gap: 20px;
  margin: 26px 0;
}
.recovery-nav {
  display: flex;
  margin-top: 30px;
  padding-top: 18px;
  border-top: 1px solid var(--line);
}
.recovery-header-right {
  display: inline-flex;
  gap: 14px;
  align-items: center;
}
.back-link {
  font-size: 13px;
  color: var(--muted);
}
.back-link:hover {
  color: var(--ink, inherit);
}
.error {
  color: var(--red);
  background: var(--red-soft);
  padding: 14px;
}
.recover-request {
  display: flex;
  gap: 8px;
}
.recover-request input {
  flex: 1;
  min-width: 0;
}
.recover-request button {
  flex: none;
}
.recover-tip {
  margin: -8px 0 0;
  font-size: 12px;
  color: var(--muted);
}
</style>
