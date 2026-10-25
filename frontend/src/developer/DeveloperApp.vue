<script setup lang="ts">
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import CaptchaField from "../shared/components/CaptchaField.vue";
import { onMounted, reactive, ref } from "vue";
import { request, downloadText, ApiError } from "../shared/api";
import { branding, loadBranding } from "../shared/branding";
import ProductReleases from "../shared/components/ProductReleases.vue";
import LicensePolicy from "../shared/components/LicensePolicy.vue";

const token = ref(sessionStorage.getItem("yn.developer_token") || "");
const profile = ref<any>(),
  products = ref<any[]>([]),
  selected = ref("");
const error = ref(""),
  busy = ref(false),
  codes = ref<string[]>([]),
  licenses = ref<any>();
const policyRevision = ref(0);
const login = reactive({ username: "", password: "", captcha_id: "", captcha_code: "" });
const captchaField = ref<any>(null);
const product = reactive({
  name: "",
  code: "",
  bind_mode: "domain",
  license_mode: "subscription",
  update_duration_days: 0,
  expire_grace_days: 0,
});
const cards = reactive({
  name: "",
  quantity: 1,
  duration_days: 365,
  is_permanent: false,
});
const password = reactive({ current_password: "", new_password: "" });
function logout() {
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
    if (e instanceof ApiError && e.status === 401) logout();
    throw e;
  }
}
async function run(action: () => Promise<unknown>) {
  busy.value = true;
  error.value = "";
  try {
    await action();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function refresh() {
  products.value = await api("/developer/products");
  if (!products.value.some((p) => p.id === selected.value))
    selected.value = products.value[0]?.id || "";
}
function signIn() {
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
    product.name = "";
    product.code = "";
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
  codes.value = [];
  licenses.value = null;
}
function changePassword() {
  void run(async () => {
    await api("/developer/password", {
      method: "POST",
      body: JSON.stringify(password),
    });
    password.current_password = "";
    password.new_password = "";
    logout();
  });
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
        ><a href="/recover">忘记密码</a>
      </form>
    </section>
    <template v-else>
      <section class="panel">
        <h2>我的产品</h2>
        <form class="developer-form" @submit.prevent="createProduct">
          <label class="field"
            >授权模式<select v-model="product.license_mode">
              <option value="subscription">订阅使用（到期停用）</option>
              <option value="perpetual">永久使用、限期更新</option>
            </select></label
          >
          <label class="field"
            >更新天数（0 跟随卡密期限）<input
              v-model.number="product.update_duration_days"
              type="number"
              min="0"
              max="36500"
              required
          /></label>
          <label v-if="product.license_mode === 'subscription'" class="field"
            >使用到期宽限天数<input
              v-model.number="product.expire_grace_days"
              type="number"
              min="0"
              max="365"
              required
          /></label>
          <label class="field"
            >产品名称<input
              v-model="product.name"
              maxlength="100"
              placeholder="Ebox"
              required
          /></label>
          <label class="field"
            >产品代码<input
              v-model="product.code"
              maxlength="20"
              placeholder="EBOX"
              pattern="[A-Za-z0-9]+"
              required
          /></label>
          <label class="field"
            >授权绑定方式<select v-model="product.bind_mode">
              <option value="domain">域名</option>
              <option value="device">设备</option>
              <option value="ip">IP</option>
              <option value="account">账号</option>
              <option value="none">无绑定</option>
            </select></label
          >
          <button class="btn" :disabled="busy">创建产品</button>
        </form>
        <label v-if="products.length" class="field"
          >选择产品<select v-model="selected" @change="selectProduct">
            <option v-for="p in products" :key="p.id" :value="p.id">
              {{ p.name }}
            </option>
          </select></label
        >
        <p v-else>还没有产品。创建后即可接入授权，也可按需启用升级服务。</p>
        <article
          v-for="p in products.filter((p) => p.id === selected)"
          :key="p.id"
        >
          <p>
            App Key：<code class="wrap">{{ p.app_key }}</code>
          </p>
          <p>绑定方式：{{ p.bind_mode }}</p>
          <details>
            <summary>产品验签公钥</summary>
            <pre class="wrap">{{ p.public_key_pem }}</pre>
          </details>
        </article>
        <LicensePolicy
          v-if="selected"
          :key="'policy-' + selected + '-' + policyRevision"
          :product-id="selected"
          :api="api"
          prefix="/developer"
          :can-write="true"
          @saved="refresh"
        />
        <ProductReleases
          v-if="selected"
          :key="selected"
          :product-id="selected"
          :api="api"
          prefix="/developer"
          :can-write="true"
        />
      </section>
      <section v-if="selected" class="panel">
        <h2>授权发卡</h2>
        <form class="developer-form" @submit.prevent="createCards">
          <label class="field"
            >批次名称<input v-model="cards.name" required
          /></label>
          <label class="field"
            >数量<input
              v-model.number="cards.quantity"
              type="number"
              min="1"
              max="1000"
              required
          /></label>
          <label class="field"
            >有效天数<input
              v-model.number="cards.duration_days"
              type="number"
              min="1"
              :disabled="cards.is_permanent"
              required
          /></label>
          <label
            ><input
              v-model="cards.is_permanent"
              type="checkbox"
            />永久卡（更新期限另见产品规则）</label
          >
          <button class="btn" :disabled="busy">生成卡密</button>
        </form>
        <template v-if="codes.length"
          ><p>本次生成 {{ codes.length }} 张卡密，请及时保存。</p>
          <button
            class="btn secondary"
            @click="downloadText(codes.join('\n'), 'license-cards.txt')"
          >
            导出卡密
          </button></template
        >
        <button class="btn secondary" :disabled="busy" @click="showLicenses()">
          查看已激活授权
        </button>
        <template v-if="licenses"
          ><p>共 {{ licenses.total }} 条授权</p>
          <div class="license-list">
            <p v-for="license in licenses.items" :key="license.id">
              {{ license.license_no }} · {{ license.status }} · 使用到期：{{
                license.usage_expired_at || "永久使用"
              }}
              · 更新到期：{{ license.updates_expired_at || "永久更新" }}
            </p>
          </div>
          <button
            v-if="licenses.page > 1"
            class="btn secondary"
            :disabled="busy"
            @click="showLicenses(licenses.page - 1)"
          >
            上一页</button
          ><button
            v-if="licenses.page * licenses.page_size < licenses.total"
            class="btn secondary"
            :disabled="busy"
            @click="showLicenses(licenses.page + 1)"
          >
            下一页
          </button></template
        >
      </section>
      <section class="panel">
        <details>
          <summary>修改密码</summary>
          <form class="developer-form" @submit.prevent="changePassword">
            <label class="field"
              >当前密码<input
                v-model="password.current_password"
                type="password"
                autocomplete="current-password"
                required /></label
            ><label class="field"
              >新密码<input
                v-model="password.new_password"
                type="password"
                minlength="8"
                autocomplete="new-password"
                required /></label
            ><button class="btn" :disabled="busy">保存并重新登录</button>
          </form>
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
