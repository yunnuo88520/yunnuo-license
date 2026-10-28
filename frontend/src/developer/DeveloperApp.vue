<script setup lang="ts">
import ApplicationWorkspace from "../shared/components/ApplicationWorkspace.vue";
import InstallationManager from "../shared/components/InstallationManager.vue";
import { termsSummary } from "../shared/authorization";
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import CaptchaField from "../shared/components/CaptchaField.vue";
import { onMounted, reactive, ref } from "vue";
import { request, downloadText, isInvalidSession } from "../shared/api";
import PasswordChangeForm from "../shared/components/PasswordChangeForm.vue";
import type { PasswordChangeInput } from "../shared/password";
import { friendlyError } from "../shared/errors";
import { branding, loadBranding } from "../shared/branding";
import ProductReleases from "../shared/components/ProductReleases.vue";
import LicensePolicy from "../shared/components/LicensePolicy.vue";
import IntegrationGuide from "../shared/components/IntegrationGuide.vue";

const token = ref(sessionStorage.getItem("yn.developer_token") || "");
const profile = ref<any>(),
  products = ref<any[]>([]),
  selected = ref("");
const packages = ref<any[]>([]);
async function loadPackages() {
  packages.value = selected.value
    ? await api(`/developer/products/${selected.value}/packages`)
    : [];
  cards.package_id = "";
}
const error = ref(""),
  busy = ref(false),
  codes = ref<string[]>([]),
  licenses = ref<any>();
const message = ref("");
const policyRevision = ref(0);
const createdSecrets = reactive<Record<string, string>>({});
const login = reactive({
  username: "",
  password: "",
  captcha_id: "",
  captcha_code: "",
});
const captchaField = ref<any>(null);
const product = reactive({
  name: "",
  bind_mode: "domain",
  license_mode: "subscription",
  update_duration_days: 0,
  expire_grace_days: 0,
});
const cards = reactive({
  package_id: "",
  name: "",
  quantity: 1,
  duration_days: 365,
  is_permanent: false,
});
function logout() {
  Object.keys(createdSecrets).forEach((id) => delete createdSecrets[id]);
  token.value = "";
  profile.value = null;
  products.value = [];
  selected.value = "";
  codes.value = [];
  licenses.value = null;
  sessionStorage.removeItem("yn.developer_token");
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
async function run(action: () => Promise<unknown>) {
  busy.value = true;
  error.value = "";
  try {
    await action();
  } catch (e: any) {
    error.value = friendlyError(e);
  } finally {
    busy.value = false;
  }
}
async function refresh() {
  products.value = await api("/developer/products");
  if (!products.value.some((p) => p.id === selected.value))
    selected.value = products.value[0]?.id || "";
  await loadPackages();
}
function signIn() {
  message.value = "";
  void run(async () => {
    login.captcha_id = captchaField.value?.captchaId || "";
    try {
      const result = await request<any>("/developer/login", {
        method: "POST",
        body: JSON.stringify(login),
      });
      token.value = result.access_token;
      sessionStorage.setItem("yn.developer_token", token.value);
      login.password = "";
      profile.value = await api("/developer/profile");
      await refresh();
    } finally {
      login.captcha_code = "";
      captchaField.value?.refresh();
    }
  });
}
function createProduct() {
  void run(async () => {
    const result = await api("/developer/products", {
      method: "POST",
      body: JSON.stringify(product),
    });
    createdSecrets[result.id] = result.app_secret;
    product.name = "";
    await refresh();
    selected.value = result.id;
  });
}
function createCards() {
  void run(async () => {
    const result = await api(
      `/developer/products/${selected.value}/card-batches`,
      { method: "POST", body: JSON.stringify(cards) },
    );
    codes.value = result.codes;
    policyRevision.value++;
  });
}
function showLicenses(page = 1) {
  void run(async () => {
    licenses.value = await api(
      `/developer/products/${selected.value}/licenses?page=${page}`,
    );
  });
}
function selectProduct() {
  void run(loadPackages);
  codes.value = [];
  licenses.value = null;
}
function changePassword(input: PasswordChangeInput) {
  return api<{ changed: boolean }>("/developer/password", {
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
  await loadBranding("开发者控制台");
  if (token.value)
    await run(async () => {
      profile.value = await api("/developer/profile");
      await refresh();
    });
});
</script>

<template>
  <main class="developer-page">
    <header class="developer-header">
      <div>
        <h1>{{ branding.site_name }} · 开发者控制台</h1>
        <p>管理自己的产品、授权与升级版本</p>
      </div>
      <div class="header-actions">
        <AppearanceMenu />
        <button v-if="profile" class="btn secondary" @click="logout">
          {{ profile.display_name || profile.username }} · 退出
        </button>
      </div>
    </header>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="message" class="notice" role="status">{{ message }}</p>
    <section v-if="!profile" class="panel login-box">
      <h2>开发者登录</h2>
      <p>请使用站点管理员为你创建的开发者账号。</p>
      <form class="developer-form" @submit.prevent="signIn">
        <CaptchaField ref="captchaField" v-model="login.captcha_code" />
        <label class="field"
          >账号<input v-model="login.username" autocomplete="username" required
        /></label>
        <label class="field"
          >密码<input
            v-model="login.password"
            type="password"
            autocomplete="current-password"
            required
        /></label>
        <button class="btn" :disabled="busy">登录</button
        ><a href="/recover?from=developer">忘记密码</a>
      </form>
    </section>
    <template v-else>
      <ApplicationWorkspace :api="api" :can-write="true" :developer="true" />
      <section class="panel">
        <details>
          <summary>修改密码</summary>
          <PasswordChangeForm
            :change="changePassword"
            @changed="passwordChanged"
          />
        </details>
      </section>
    </template>
  </main>
</template>

<style scoped>
.developer-page {
  max-width: 1100px;
  margin: 0 auto;
  padding: 28px 18px;
}
.developer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 24px;
}
.developer-page .panel {
  margin-bottom: 24px;
  padding: 24px;
}
.developer-form {
  display: grid;
  gap: 14px;
  max-width: 650px;
  margin: 20px 0;
}
.login-box {
  max-width: 520px;
  margin: 60px auto;
}
.wrap {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.license-list {
  overflow-x: auto;
}
</style>
