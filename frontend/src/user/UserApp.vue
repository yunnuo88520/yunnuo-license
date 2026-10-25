<script setup lang="ts">
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import SecurityAppeals from "./SecurityAppeals.vue";
import { onMounted, reactive, ref } from "vue";
import { ApiError, request } from "../shared/api";
import { branding, loadBranding } from "../shared/branding";
import { formatTime } from "../shared/format";
const token = ref(sessionStorage.getItem("yn.user_token") || ""),
  profile = ref<any>(),
  items = ref<any[]>([]),
  error = ref(""),
  message = ref(""),
  busy = ref(false),
  action = ref(""),
  selected = ref<any>();
const login = reactive({ username: "", password: "" });
const claim = reactive({ application_code: "", card_code: "", bind_value: "" });
const renew = reactive({ binding_id: "", card_code: "" });
const rebind = reactive({
  binding_id: "",
  new_value: "",
  reason: "",
  request_id: "",
});
const password = reactive({ current_password: "", new_password: "" });
function logout() {
  token.value = "";
  profile.value = null;
  items.value = [];
  selected.value = null;
  sessionStorage.removeItem("yn.user_token");
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
async function run(fn: () => Promise<unknown>) {
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await fn();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function refresh() {
  items.value = await api("/user/licenses");
}
function signIn() {
  void run(async () => {
    const r = await request<any>("/user/login", {
      method: "POST",
      body: JSON.stringify(login),
    });
    token.value = r.access_token;
    profile.value = r.user;
    login.password = "";
    sessionStorage.setItem("yn.user_token", token.value);
    await refresh();
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
function changePassword() {
  void run(async () => {
    await api("/user/password", {
      method: "POST",
      body: JSON.stringify(password),
    });
    password.current_password = "";
    password.new_password = "";
    logout();
    message.value = "密码已修改，请重新登录。";
  });
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
        <a href="/">公开验真</a
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
        ><a href="/recover">忘记密码</a>
      </form>
    </section>
    <template v-else
      ><section class="panel">
        <h2>激活新授权</h2>
        <p>
          仅未激活卡密可首次认领。已有授权请联系管理员分配，授权归属不能自行转移。
        </p>
        <form class="claim" @submit.prevent="activate">
          <label class="field"
            >应用代码<input
              v-model="claim.application_code"
              placeholder="例如 EBOX"
              required /></label
          ><label class="field"
            >未激活卡密<input
              v-model="claim.card_code"
              type="password"
              autocomplete="off"
              required /></label
          ><label class="field"
            >绑定信息<input
              v-model="claim.bind_value"
              placeholder="域名 / IP / 设备 / 应用账号" /></label
          ><button class="btn" :disabled="busy">激活并绑定到我的账号</button>
        </form>
      </section>
      <div class="heading">
        <h2>我的应用授权</h2>
        <button class="btn secondary" :disabled="busy" @click="run(refresh)">
          刷新授权
        </button>
      </div>
      <p v-if="!items.length">暂未拥有授权，可激活新卡密或联系管理员分配。</p>
      <section
        v-for="item in items"
        :key="item.license.id"
        class="panel license-card"
      >
        <div class="heading">
          <div>
            <h2>{{ item.product.name }}</h2>
            <code>{{ item.license.license_no }}</code>
          </div>
          <span class="badge">{{
            item.license.license_mode === "perpetual"
              ? "永久使用、限期更新"
              : "订阅使用"
          }}</span>
        </div>
        <div class="facts">
          <div>
            <small>使用期限</small
            ><strong>{{
              item.license.usage_expired_at
                ? formatTime(item.license.usage_expired_at)
                : "永久使用"
            }}</strong>
          </div>
          <div>
            <small>更新服务</small
            ><strong>{{
              item.license.updates_expired_at
                ? formatTime(item.license.updates_expired_at)
                : "永久更新"
            }}</strong>
          </div>
          <div>
            <small>换绑规则</small
            ><strong>{{
              !item.policy.allowed
                ? "禁止换绑"
                : item.policy.limit === -1
                  ? "不限次数"
                  : `已用 ${item.changes_used} / ${item.policy.limit} 次`
            }}</strong
            ><small v-if="item.policy.allowed"
              >冷却 {{ item.policy.cooldown_hours }} 小时</small
            >
          </div>
        </div>
        <p class="rights-status">
          <strong>{{ item.can_use ? "应用可用" : "应用暂不可用" }}</strong
          ><span>{{
            item.can_update ? "更新服务有效" : "更新服务不可用"
          }}</span>
        </p>
        <p v-if="item.last_change_at && item.policy.cooldown_hours">
          下次换绑时间：{{
            formatTime(
              new Date(
                new Date(item.last_change_at).getTime() +
                  item.policy.cooldown_hours * 3600000,
              ).toISOString(),
            )
          }}
        </p>
        <div class="bindings">
          <p v-for="b in item.bindings" :key="b.id">
            {{ b.mode }} · {{ b.value }} ·
            {{ b.status === "active" ? "当前生效" : "已失效" }}
          </p>
        </div>
        <div class="actions">
          <button class="btn" :disabled="busy" @click="open('renew', item)">
            卡密续费</button
          ><button
            class="btn secondary"
            :disabled="busy || !item.policy.allowed || !item.can_use"
            @click="open('rebind', item)"
          >
            更换绑定</button
          ><span>归属变更需管理员审批</span>
        </div>
        <form
          v-if="selected?.license.id === item.license.id && action"
          @submit.prevent="save"
        >
          <h3>{{ action === "renew" ? "卡密续费" : "申请更换部署绑定" }}</h3>
          <label class="field"
            >选择当前绑定<select
              v-model="renew.binding_id"
              v-if="action === 'renew'"
              required
            >
              <option
                v-for="b in item.bindings.filter(
                  (b: any) => b.status === 'active',
                )"
                :key="b.id"
                :value="b.id"
              >
                {{ b.value }}
              </option></select
            ><select v-else v-model="rebind.binding_id" required>
              <option
                v-for="b in item.bindings.filter(
                  (b: any) => b.status === 'active',
                )"
                :key="b.id"
                :value="b.id"
              >
                {{ b.value }}
              </option>
            </select></label
          ><label v-if="action === 'renew'" class="field"
            >续费卡密<input
              v-model="renew.card_code"
              type="password"
              required
              autocomplete="off" /></label
          ><template v-else
            ><label class="field"
              >新的绑定信息<input v-model="rebind.new_value" required /></label
            ><label class="field"
              >更换原因<input v-model="rebind.reason" required maxlength="500"
            /></label>
            <p>成功后旧绑定将失效，并按应用规则扣除一次换绑额度。</p></template
          >
          <div class="actions">
            <button class="btn" :disabled="busy">
              {{ action === "renew" ? "确认续费" : "确认换绑" }}</button
            ><button type="button" class="btn secondary" @click="action = ''">
              取消
            </button>
          </div>
        </form>
      </section>
      <SecurityAppeals :token="token" :api="api" />
      <section class="panel">
        <details>
          <summary>修改登录密码</summary>
          <form @submit.prevent="changePassword">
            <label class="field"
              >当前密码<input
                v-model="password.current_password"
                type="password"
                required
                autocomplete="current-password" /></label
            ><label class="field"
              >新密码<input
                v-model="password.new_password"
                type="password"
                minlength="8"
                maxlength="256"
                required
                autocomplete="new-password" /></label
            ><button class="btn" :disabled="busy">保存密码并重新登录</button>
          </form>
        </details>
      </section></template
    >
  </main>
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
