<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  ArrowRight,
  CheckCircle2,
  CircleUserRound,
  Crosshair,
  Globe2,
  KeyRound,
  Network,
  RotateCcw,
  Search,
  ShieldCheck,
  TicketCheck,
} from "lucide-vue-next";
import { request } from "../shared/api";
import { formatTime, statusLabel } from "../shared/format";
import { branding, loadBranding, logoSource } from "../shared/branding";

const query = ref("");
const queryType = ref("auto");
const loading = ref(false);
const error = ref("");
const result = ref<any>(null);
const setupRequired = ref(false);
const setupLoading = ref(false);
const setupError = ref("");
const sqliteSetupDSN = ref("file:yn-license.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)");
const setup = ref({
  database_driver: "sqlite",
  database_dsn: "file:yn-license.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)",
  admin_username: "",
  admin_password: "",
  admin_name: "系统管理员",
});
const queryTypes = [
  { value: "auto", label: "智能识别", icon: Crosshair },
  { value: "domain", label: "域名", icon: Globe2 },
  { value: "ip", label: "IP", icon: Network },
  { value: "account", label: "账号", icon: CircleUserRound },
  { value: "license", label: "许可证", icon: KeyRound },
  { value: "card", label: "卡密", icon: TicketCheck },
];
const placeholders: Record<string, string> = {
  auto: "输入域名、IP、账号或授权凭证",
  domain: "例如 example.com",
  ip: "例如 203.0.113.42",
  account: "输入 QQ、手机号或业务账号",
  license: "输入 lic_ 开头的授权编号",
  card: "输入完整卡密",
};
const displayResults = computed(() =>
  result.value?.results?.length ? result.value.results : result.value ? [result.value] : [],
);
onMounted(async () => {
  await loadBranding("授权查询").catch(() => undefined);
  const status: { initialized: boolean; suggested_sqlite_dsn?: string } = await request<{
    initialized: boolean;
    suggested_sqlite_dsn?: string;
  }>("/v1/setup/status").catch(() => ({ initialized: true }));
  if (status.suggested_sqlite_dsn) {
    sqliteSetupDSN.value = status.suggested_sqlite_dsn;
    setup.value.database_dsn = status.suggested_sqlite_dsn;
  }
  setupRequired.value = !status.initialized;
});
async function submit() {
  const value = query.value.trim();
  if (!value) return;
  loading.value = true;
  error.value = "";
  result.value = null;
  try {
    result.value = await request("/v1/licenses/query", {
      method: "POST",
      body: JSON.stringify({ query: value, query_type: queryType.value }),
    });
  } catch (e: any) {
    error.value =
      e.code === "AUTHORIZATION_NOT_FOUND"
        ? "未查询到该授权"
        : "查询失败，请稍后重试";
  } finally {
    loading.value = false;
  }
}
function clear() {
  query.value = "";
  result.value = null;
  error.value = "";
}
function selectQueryType(value: string) {
  queryType.value = value;
  result.value = null;
  error.value = "";
}
async function initialize() {
  setupLoading.value = true;
  setupError.value = "";
  try {
    await request("/v1/setup/initialize", {
      method: "POST",
      body: JSON.stringify(setup.value),
    });
    setupRequired.value = false;
    setup.value.admin_password = "";
  } catch (e: any) {
    setupError.value = e.message || "初始化失败，请检查数据库配置";
  } finally {
    setupLoading.value = false;
  }
}
</script>
<template>
  <main class="query-canvas">
    <header class="query-nav">
      <a class="query-brand" href="/"
        ><img :src="logoSource" alt="" /><span>{{ branding.site_name }}</span></a
      >
      <div>
        <a href="/agent-console/">代理入口 <ArrowRight :size="15" /></a
        ><a href="/admin-console/">管理控制台</a>
      </div>
    </header>
    <section class="query-stage">
      <div class="stage-index mono">
        YN / LICENSE INFRASTRUCTURE<br />PUBLIC VERIFY NODE 01
      </div>
      <div class="query-copy">
        <span class="eyebrow"
          ><ShieldCheck :size="15" /> AUTHORIZATION LOOKUP</span
        >
        <h1>软件授权<br /><em>即时查询。</em></h1>
        <p>通过域名、IP、QQ、手机号、业务账号或授权凭证，查询软件授权状态。</p>
      </div>
      <form class="query-form" @submit.prevent="submit">
        <label for="license-query">授权查询</label>
        <div class="query-types" role="group" aria-label="查询类型">
          <button
            v-for="type in queryTypes"
            :key="type.value"
            type="button"
            :class="{ active: queryType === type.value }"
            :aria-pressed="queryType === type.value"
            @click="selectQueryType(type.value)"
          >
            <component :is="type.icon" :size="15" />{{ type.label }}
          </button>
        </div>
        <div class="query-input">
          <Search :size="22" /><input
            id="license-query"
            v-model="query"
            autocomplete="off"
            :placeholder="placeholders[queryType]"
            autofocus
          /><button :disabled="loading || !query.trim()" aria-label="查询">
            <RotateCcw v-if="loading" class="spin" :size="20" /><ArrowRight
              v-else
              :size="22"
            />
          </button>
        </div>
        <p class="error">{{ error }}</p>
      </form>
      <div class="stage-rule">
        <span></span
        ><small class="mono">ENCRYPTED / TRACEABLE / VERIFIABLE</small>
      </div>
    </section>
    <Transition name="result">
      <section v-if="result" class="query-result">
        <div class="result-title">
          <div>
            <span class="eyebrow"
              ><CheckCircle2 :size="15" /> QUERY COMPLETE</span
            >
            <h2>
              {{ result.results?.length ? `找到 ${result.match_count} 项授权` : result.product_name }}
            </h2>
            <code>{{ result.query_masked || result.product_code }}</code>
          </div>
          <span
            v-if="!result.results?.length"
            class="status"
            :class="result.license_status || result.card_status"
            >{{
              statusLabel(result.license_status || result.card_status)
            }}</span
          >
        </div>
        <div class="result-list">
          <article v-for="(item, index) in displayResults" :key="item.license_no || index">
            <header v-if="result.results?.length">
              <div><strong>{{ item.product_name }}</strong><code>{{ item.product_code }}</code></div>
              <span class="status" :class="item.license_status">{{ statusLabel(item.license_status) }}</span>
            </header>
            <dl>
              <div v-if="item.license_no">
                <dt>授权编号</dt>
                <dd class="mono">{{ item.license_no }}</dd>
              </div>
              <div v-if="item.card_status">
                <dt>卡密状态</dt>
                <dd>{{ statusLabel(item.card_status) }}</dd>
              </div>
              <div>
                <dt>授权状态</dt>
                <dd>{{ statusLabel(item.license_status || item.card_status) }}</dd>
              </div>
              <div>
                <dt>授权时长</dt>
                <dd>{{ item.is_permanent ? "永久" : item.duration_days ? `${item.duration_days} 天` : "按到期时间" }}</dd>
              </div>
              <div>
                <dt>激活时间</dt>
                <dd>{{ formatTime(item.activated_at) }}</dd>
              </div>
              <div>
                <dt>到期时间</dt>
                <dd>{{ item.is_permanent ? "永久" : formatTime(item.expired_at) }}</dd>
              </div>
              <div>
                <dt>最近验证</dt>
                <dd>{{ formatTime(item.last_verify_at) }}</dd>
              </div>
              <div v-if="!result.results?.length">
                <dt>服务器时间</dt>
                <dd>{{ formatTime(result.server_time) }}</dd>
              </div>
            </dl>
          </article>
        </div>
        <p v-if="result.results?.length" class="result-time">查询时间 {{ formatTime(result.server_time) }}</p>
        <button class="btn secondary" @click="clear">
          <KeyRound :size="16" />查询其他授权
        </button>
      </section>
    </Transition>
    <div v-if="setupRequired" class="setup-overlay">
      <section class="setup-panel" role="dialog" aria-modal="true" aria-labelledby="setup-title">
        <div class="setup-heading">
          <span class="eyebrow"><ShieldCheck :size="15" /> FIRST BOOT</span>
          <h2 id="setup-title">初始化授权中心</h2>
          <p>首次启动需要设置数据库连接和首个超级管理员。完成后才开放登录入口。</p>
        </div>
        <form class="setup-form" @submit.prevent="initialize">
          <div class="setup-section">
            <span class="setup-label">01 / 数据库</span>
            <div class="setup-choice">
              <button type="button" :class="{ active: setup.database_driver === 'sqlite' }" @click="setup.database_driver = 'sqlite'; setup.database_dsn = sqliteSetupDSN">SQLite</button>
              <button type="button" :class="{ active: setup.database_driver === 'mysql' }" @click="setup.database_driver = 'mysql'; setup.database_dsn = 'user:password@tcp(127.0.0.1:3306)/yunnuo_license?charset=utf8mb4&parseTime=false'">MySQL</button>
            </div>
            <label class="field">连接 DSN<textarea v-model="setup.database_dsn" rows="3" required></textarea></label>
            <small class="setup-hint">SQLite 适合单机试用；生产环境可填写外部 MySQL 连接串。</small>
          </div>
          <div class="setup-section">
            <span class="setup-label">02 / 管理员</span>
            <div class="setup-grid">
              <label class="field">管理员账号<input v-model="setup.admin_username" autocomplete="username" required minlength="3" /></label>
              <label class="field">显示名称<input v-model="setup.admin_name" required /></label>
              <label class="field full">管理员密码<input v-model="setup.admin_password" type="password" autocomplete="new-password" required minlength="8" /></label>
            </div>
          </div>
          <p class="error">{{ setupError }}</p>
          <button class="btn setup-submit" :disabled="setupLoading">
            <RotateCcw v-if="setupLoading" class="spin" :size="17" /><ArrowRight v-else :size="17" />完成初始化
          </button>
        </form>
      </section>
    </div>
  </main>
</template>
