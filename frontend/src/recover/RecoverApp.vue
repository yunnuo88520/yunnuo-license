<script setup lang="ts">
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import { onMounted, reactive, ref } from "vue";
import { CheckCheck } from "lucide-vue-next";
import { request } from "../shared/api";
import { entries } from "../shared/entries";
import { branding, loadBranding } from "../shared/branding";
const form = reactive({ username: "", code: "", password: "", confirm: "" }),
  busy = ref(false),
  error = ref(""),
  done = ref(false),
  requesting = ref(false),
  requestTip = ref("");
// 从哪个登录页进来就回到哪个登录页；直接访问时回首页。
const fromKey = new URLSearchParams(location.search).get("from") || "";
const backURL =
  ({ admin: entries.value.admin, agent: entries.value.agent, user: entries.value.user, developer: entries.value.developer } as Record<string, string>)[fromKey] || "/";
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
onMounted(() => loadBranding("密码恢复").catch(() => {}));
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
    <section class="panel recovery-panel">
      <template v-if="!done">
        <span class="eyebrow">ACCOUNT RECOVERY</span>
        <h1>重新设置登录密码</h1>
        <p class="recovery-intro">
          输入登录账号获取邮箱验证码；持管理员签发的恢复码可直接使用。
        </p>
        <p v-if="error" role="alert" class="error">{{ error }}</p>
        <form @submit.prevent="submit">
          <section class="recover-step">
            <header class="recover-step-head">
              <b class="recover-step-no">1</b>
              <span class="recover-step-title"
                ><b>获取验证码</b><small>发送到账号绑定邮箱，30 分钟内有效</small></span
              >
            </header>
            <div class="recover-request">
              <input
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
              </button>
            </div>
            <p v-if="requestTip" role="status" class="recover-tip">
              {{ requestTip }}
            </p>
          </section>
          <section class="recover-step">
            <header class="recover-step-head">
              <b class="recover-step-no">2</b>
              <span class="recover-step-title"
                ><b>设置新密码</b><small>验证码校验通过后立即生效</small></span
              >
            </header>
            <label class="field">
              <span>恢复码 / 验证码</span>
              <input
                v-model="form.code"
                type="password"
                autocomplete="off"
                required
                maxlength="100"
                placeholder="6 位邮箱验证码，或管理员提供的恢复码"
            /></label>
            <div class="recover-grid">
              <label class="field">
                <span>新密码</span>
                <input
                  v-model="form.password"
                  type="password"
                  autocomplete="new-password"
                  required
                  minlength="8"
                  maxlength="256"
              /></label>
              <label class="field">
                <span>确认新密码</span>
                <input
                  v-model="form.confirm"
                  type="password"
                  autocomplete="new-password"
                  required
                  minlength="8"
                  maxlength="256"
              /></label>
            </div>
          </section>
          <button class="btn recover-submit" :disabled="busy">
            {{ busy ? "正在重置…" : "确认重置密码" }}
          </button>
        </form>
      </template>
      <div v-else class="recover-done">
        <CheckCheck :size="44" />
        <h1>密码已重置</h1>
        <p>原登录会话已失效，请使用新密码重新登录。</p>
        <a class="btn" :href="backURL">返回登录</a>
      </div>
    </section>
  </main>
</template>
<style scoped>
main {
  max-width: 640px;
  margin: 48px auto 60px;
  padding: 24px;
}
.brand {
  font-size: 19px;
  font-weight: 600;
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
.panel {
  padding: 32px;
  margin-top: 26px;
  border-radius: 16px;
}
h1 {
  font-size: 26px;
  margin: 10px 0;
}
.recovery-intro {
  margin: 0 0 4px;
  color: var(--muted);
  line-height: 1.7;
}
form {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 14px;
  margin: 22px 0 0;
}
.recover-step {
  min-width: 0;
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 16px 18px;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  background: var(--field);
}
.recover-step-head {
  display: flex;
  gap: 10px;
  align-items: center;
}
.recover-step-no {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--green);
  color: #fff;
  display: grid;
  place-items: center;
  font-size: 12px;
  flex: none;
}
.recover-step-title {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 1px;
}
.recover-step-title b {
  font-size: 13px;
  color: var(--ink, inherit);
}
.recover-step-title small {
  font-size: 11px;
  color: var(--muted);
}
.recover-request {
  display: flex;
  min-width: 0;
  gap: 8px;
}
.recover-request input {
  flex: 1;
  min-width: 0;
}
.recover-request .btn {
  flex: none;
}
.recover-tip {
  margin: 0;
  font-size: 12px;
  color: var(--muted);
}
.recover-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.recover-submit {
  width: 100%;
  min-height: 46px;
  margin-top: 2px;
}
.recover-done {
  display: grid;
  gap: 10px;
  justify-items: center;
  text-align: center;
  padding: 26px 0 12px;
  color: var(--green);
}
.recover-done h1 {
  margin: 4px 0 0;
  color: var(--ink, inherit);
}
.recover-done p {
  margin: 0;
  color: var(--muted);
}
.recover-done .btn {
  margin-top: 10px;
  min-width: 200px;
}
p {
  color: var(--muted);
  line-height: 1.8;
}
.error {
  color: var(--red);
  background: var(--red-soft);
  padding: 12px 14px;
  border-radius: var(--radius-sm);
  line-height: 1.6;
}
@media (max-width: 560px) {
  .recover-request {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }
  main {
    margin: 24px auto 32px;
    padding: 16px;
  }
  .panel {
    padding: 20px;
  }
  .recover-step {
    padding: 14px;
  }
  .recover-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
