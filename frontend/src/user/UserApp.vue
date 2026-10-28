<script setup lang="ts">
import CustomerLicenses from "./CustomerLicenses.vue";
import InstallationManager from "../shared/components/InstallationManager.vue";
import { termsSummary } from "../shared/authorization";
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import HelpPanel from "../shared/components/HelpPanel.vue";
import { CircleHelp } from "lucide-vue-next";
import CaptchaField from "../shared/components/CaptchaField.vue";
import SecurityAppeals from "./SecurityAppeals.vue";
import { onMounted, reactive, ref } from "vue";
import { isInvalidSession, request } from "../shared/api";
import PasswordChangeForm from "../shared/components/PasswordChangeForm.vue";
import type { PasswordChangeInput } from "../shared/password";
import {
  accountToken,
  saveAccountToken,
  clearAccountToken,
} from "../shared/accountSession";
import { friendlyError } from "../shared/errors";
import { branding, loadBranding } from "../shared/branding";
import { formatTime, expiryInfo, onlineStateLabel } from "../shared/format";
const token = ref(accountToken("user")),
  profile = ref<any>(),
  items = ref<any[]>([]),
  error = ref(""),
  message = ref(""),
  busy = ref(false),
  action = ref(""),
  selected = ref<any>();
const login = reactive({
  username: "",
  password: "",
  captcha_id: "",
  captcha_code: "",
});
const captchaField = ref<any>(null);
const claimToken = ref("");
async function associateLicense() {
  await api("/user/licenses/claim", {
    method: "POST",
    body: JSON.stringify({ claim_token: claimToken.value.trim() }),
  });
  claimToken.value = "";
  message.value = "授权已关联当前账号。";
  await refresh();
}
const claim = reactive({ card_code: "", bind_value: "" });
const renew = reactive({ binding_id: "", card_code: "" });
const rebind = reactive({
  binding_id: "",
  new_value: "",
  reason: "",
  request_id: "",
});
const helpOpen = ref(false);
function logout() {
  token.value = "";
  profile.value = null;
  items.value = [];
  selected.value = null;
  clearAccountToken();
}
async function api<T = any>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  try {
    return await request<T>(path, options, token.value);
  } catch (e) {
    if (isInvalidSession(e)) logout();
    throw e;
  }
}
async function run(fn: () => Promise<unknown>) {
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await fn();
  } catch (e: any) {
    error.value = friendlyError(e);
  } finally {
    busy.value = false;
  }
}
async function refresh() {
  items.value = await api("/v2/user/entitlements");
}
function signIn() {
  void run(async () => {
    login.captcha_id = captchaField.value?.captchaId || "";
    try {
      const r = await request<any>("/user/login", {
        method: "POST",
        body: JSON.stringify(login),
      });
      token.value = r.access_token;
      profile.value = r.user;
      login.password = "";
      saveAccountToken(token.value);
      await refresh();
    } finally {
      login.captcha_code = "";
      captchaField.value?.refresh();
    }
  });
}
function activate() {
  void run(async () => {
    await api("/user/activate", {
      method: "POST",
      body: JSON.stringify(claim),
    });
    claim.card_code = "";
    claim.bind_value = "";
    message.value = "授权已激活并归属当前账号。";
    await refresh();
  });
}
function open(kind: string, item: any) {
  action.value = kind;
  selected.value = item;
  renew.binding_id =
    item.bindings.find((b: any) => b.status === "active")?.id || "";
  renew.card_code = "";
  Object.assign(rebind, {
    binding_id: renew.binding_id,
    new_value: "",
    reason: "",
    request_id: crypto.randomUUID(),
  });
  error.value = "";
  message.value = "";
}
function save() {
  void run(async () => {
    const endpoint = action.value === "renew" ? "renew" : "rebind";
    await api(
      `/user/licenses/${selected.value.license.license_no}/${endpoint}`,
      {
        method: "POST",
        body: JSON.stringify(endpoint === "renew" ? renew : rebind),
      },
    );
    renew.card_code = "";
    action.value = "";
    message.value =
      endpoint === "renew"
        ? "续费成功，期限已更新。"
        : "换绑成功，旧绑定已失效。";
    await refresh();
  });
}
function changePassword(input: PasswordChangeInput) {
  return api<{ changed: boolean }>("/user/password", {
    method: "POST",
    body: JSON.stringify(input),
  });
}
function passwordChanged() {
  logout();
  error.value = "";
  message.value = "密码已修改，请使用新密码重新登录。";
}
onMounted(async () => {
  await loadBranding("用户中心");
  if (token.value)
    await run(async () => {
      profile.value = await api("/user/profile");
      await refresh();
    });
});
</script>
<template>
  <main class="user-page">
    <header>
      <div>
        <h1>{{ branding.site_name }} · 用户中心</h1>
        <p>管理自己的应用授权、续费与部署绑定</p>
      </div>
      <div class="header-actions">
        <AppearanceMenu />
        <button class="btn secondary" @click="helpOpen = true">
          <CircleHelp :size="15" />帮助
        </button>
        <a href="/">公开验真</a
        ><a v-if="profile?.agent_id" href="/agent-console/">代理工作台</a
        ><button v-if="profile" class="btn secondary" @click="logout">
          {{ profile.display_name || profile.username }} · 退出
        </button>
      </div>
    </header>
    <p v-if="error" class="error notice" role="alert">{{ error }}</p>
    <p v-if="message" class="notice" role="status">{{ message }}</p>
    <section v-if="!profile" class="panel login">
      <h2>用户登录</h2>
      <p>账号由平台管理员开通。公开查询不能认领授权。</p>
      <form @submit.prevent="signIn">
        <CaptchaField ref="captchaField" v-model="login.captcha_code" />
        <label class="field"
          >账号<input
            v-model="login.username"
            autocomplete="username"
            required /></label
        ><label class="field"
          >密码<input
            v-model="login.password"
            type="password"
            autocomplete="current-password"
            required /></label
        ><button class="btn" :disabled="busy">登录用户中心</button
        ><a href="/recover?from=user">忘记密码</a>
      </form>
    </section>
    <template v-else>
      <CustomerLicenses :api="api" />
      <SecurityAppeals :token="token" :api="api" />
      <section class="panel">
        <details>
          <summary>修改登录密码</summary>
          <PasswordChangeForm
            :change="changePassword"
            @changed="passwordChanged"
          />
        </details></section
    ></template>
  </main>
  <HelpPanel :open="helpOpen" @close="helpOpen = false" />
</template>
<style scoped>
.user-page {
  max-width: 1160px;
  margin: auto;
  padding: 30px 22px;
}
.heading > div {
  min-width: 0;
}
header,
.heading,
.actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  flex-wrap: wrap;
}
header {
  margin-bottom: 28px;
}
header a {
  margin-right: 18px;
}
.panel {
  padding: 26px;
  margin-bottom: 24px;
  border-radius: 12px;
}
.login {
  max-width: 520px;
  margin: 60px auto;
}
form {
  display: grid;
  gap: 15px;
  margin: 18px 0;
  max-width: 720px;
}
.claim {
  max-width: none;
  grid-template-columns: 1fr 1.6fr 1.4fr;
  align-items: end;
}
.claim button {
  grid-column: 1/-1;
}
.facts {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 22px;
  margin: 22px 0;
}
.facts small,
.facts strong {
  display: block;
}
.facts small,
.actions span {
  color: var(--muted);
}
.badge {
  background: var(--green-soft);
  padding: 8px 14px;
  border-radius: 24px;
}
.facts strong.expiry-warn {
  color: #d97706;
}
.facts strong.expiry-urgent {
  color: #dc2626;
}
.facts strong.expiry-expired {
  color: var(--red, #dc2626);
}
.expiry-tag {
  display: inline-block;
  margin-left: 8px;
  padding: 2px 9px;
  border-radius: 20px;
  font-size: 12px;
  vertical-align: 1px;
}
.expiry-tag-ok {
  background: var(--green-soft);
  color: var(--green);
}
.expiry-tag-warn {
  background: rgba(217, 119, 6, 0.14);
  color: #d97706;
}
.expiry-tag-urgent,
.expiry-tag-expired {
  background: rgba(220, 38, 38, 0.12);
  color: #dc2626;
}
.actions {
  justify-content: flex-start;
}
.notice {
  padding: 15px;
  background: var(--green-soft);
}
.error {
  color: var(--red);
}
.rights-status {
  display: flex;
  gap: 20px;
  padding: 12px 0;
  color: var(--green);
  flex-wrap: wrap;
}
.bindings {
  padding: 8px 16px;
  background: var(--surface);
  border-radius: 7px;
  margin-bottom: 18px;
}
.heartbeat-tag {
  display: inline-block;
  margin-left: 8px;
  padding: 1px 8px;
  border-radius: 20px;
  font-size: 12px;
}
.heartbeat-online {
  background: var(--green-soft);
  color: var(--green);
}
.heartbeat-idle {
  background: rgba(217, 119, 6, 0.14);
  color: #d97706;
}
.heartbeat-offline {
  background: rgba(220, 38, 38, 0.12);
  color: #dc2626;
}
@media (max-width: 700px) {
  .claim,
  .facts {
    grid-template-columns: 1fr;
  }
  .panel {
    padding: 18px;
  }
}
</style>
