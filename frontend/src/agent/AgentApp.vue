<script setup lang="ts">
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import CaptchaField from "../shared/components/CaptchaField.vue";
import PagedRecords from "../shared/components/PagedRecords.vue";
import { computed, onMounted, reactive, ref, watch } from "vue";
import {
  BadgeCheck,
  ChartNoAxesCombined,
  Clipboard,
  Download,
  KeyRound,
  ListRestart,
  LoaderCircle,
  LogOut,
  RefreshCw,
  Sparkles,
  TicketCheck,
  Wallet,
} from "lucide-vue-next";
import { ApiError, downloadText, request } from "../shared/api";
import { accountToken, saveAccountToken, clearAccountToken } from "../shared/accountSession";
import { friendlyError } from "../shared/errors";
import { formatTime, roleLabel } from "../shared/format";
import AppToast from "../shared/components/AppToast.vue";
import StatusBadge from "../shared/components/StatusBadge.vue";
import UserMenu from "../shared/components/UserMenu.vue";
import { toast } from "../shared/useToast";
import { branding, loadBranding, logoSource } from "../shared/branding";

const token = ref(accountToken("agent"));
const profile = ref<any>();
const loading = ref(false);
const loginError = ref("");
const captchaField = ref<any>(null);
const page = ref(location.hash.slice(1) || "overview");
const products = ref<any[]>([]),
  quotas = ref<any[]>([]),
  ledgers = ref<any[]>([]),
  batches = ref<any[]>([]),
  licenses = ref<any[]>([]),
  cards = ref<any[]>([]),
  codes = ref<string[]>([]);
const packages = ref<any[]>([]),
  account = ref<any>(),
  issuing = ref(false);
const money = (n: number) => `¥${(n / 100).toFixed(2)}`;
const login = reactive({
  username: "",
  password: "",
  captcha_id: "",
  captcha_code: "",
});
const batch = reactive({
  package_id: "",
  name: "代理发卡",
  quantity: 1,
  request_id: crypto.randomUUID(),
});
const currentPackage = computed(() =>
  packages.value.find((q) => q.package.id === batch.package_id),
);
watch(
  () => [batch.package_id, batch.name, batch.quantity],
  () => {
    batch.request_id = crypto.randomUUID();
  },
);
const navGroups = [
  {
    label: "总览",
    items: [
      { id: "overview", label: "业务概览", icon: ChartNoAxesCombined },
    ],
  },
  {
    label: "发卡",
    items: [
      { id: "generate", label: "生成卡密", icon: Sparkles, manage: true },
      { id: "batches", label: "卡密批次", icon: TicketCheck },
    ],
  },
  {
    label: "记录",
    items: [
      { id: "licenses", label: "授权记录", icon: BadgeCheck },
      { id: "ledgers", label: "额度流水", icon: ListRestart },
    ],
  },
];
const canManage = computed(() =>
  ["agent_owner", "agent_manager"].includes(profile.value?.role),
);
const metrics = computed(() => [
  quotas.value.reduce((n, q) => n + Math.max(0, q.balance), 0),
  batches.value.length,
  batches.value.reduce((n, b) => n + b.quantity, 0),
  licenses.value.length,
]);
const productName = (id: string) =>
  products.value.find((p) => p.id === id)?.name || id;
async function api<T>(path: string, options: RequestInit = {}) {
  try {
    return await request<T>(
      path,
      options,
      path === "/agent/login" ? "" : token.value,
    );
  } catch (e) {
    if (e instanceof ApiError && e.status === 401 && path !== "/agent/login")
      logout("登录状态已失效，请重新登录");
    throw e;
  }
}
async function signIn() {
  login.captcha_id = captchaField.value?.captchaId || "";
  loading.value = true;
  loginError.value = "";
  try {
    const data = await api<any>("/agent/login", {
      method: "POST",
      body: JSON.stringify(login),
    });
    token.value = data.access_token;
    saveAccountToken(token.value);
    profile.value = await api("/agent/profile");
    login.password = "";
    await refresh();
  } catch (e: any) {
    loginError.value = friendlyError(e);
    captchaField.value?.refresh();
  } finally {
    loading.value = false;
  }
}
function logout(message = "") {
  token.value = "";
  profile.value = null;
  clearAccountToken();
  loginError.value = message;
}
async function refresh() {
  loading.value = true;
  try {
    const [p, q, l, b, li, pk, ac] = await Promise.all([
      api<any[]>("/agent/products"),
      api<any[]>("/agent/quotas"),
      api<any[]>("/agent/quota-ledgers"),
      api<any[]>("/agent/card-batches"),
      api<any[]>("/agent/licenses"),
      api<any[]>("/agent/packages"),
      api<any>("/agent/account"),
    ]);
    products.value = p || [];
    quotas.value = q || [];
    ledgers.value = l || [];
    batches.value = b || [];
    licenses.value = li || [];
    packages.value = pk || [];
    account.value = ac;
    if (!packages.value.some((q) => q.package.id === batch.package_id))
      batch.package_id = packages.value[0]?.package.id || "";
  } catch (e) {
    toast(friendlyError(e));
  } finally {
    loading.value = false;
  }
}
function navigate(id: string) {
  if (id === "generate" && !canManage.value) return;
  page.value = id;
  location.hash = id;
}
async function generate() {
  if (issuing.value || !currentPackage.value) return;
  issuing.value = true;
  try {
    const data = await api<any>("/agent/card-batches", {
      method: "POST",
      body: JSON.stringify({
        ...batch,
        expected_unit_cost: currentPackage.value.unit_cost,
      }),
    });
    codes.value = data.codes || [];
    batch.request_id = crypto.randomUUID();
    toast("卡密已生成，成本已结算");
    await refresh();
    await showCards(data.batch.id);
  } catch (e) {
    toast(friendlyError(e));
  } finally {
    issuing.value = false;
  }
}
async function showCards(id: string) {
  try {
    cards.value = await api<any[]>(
      `/agent/card-batches/${encodeURIComponent(id)}/cards`,
    );
    navigate("generate");
    document
      .querySelector(".console-main")
      ?.scrollTo({ top: 9999, behavior: "smooth" });
  } catch (e) {
    toast(friendlyError(e));
  }
}
async function exportBatch(id: string) {
  try {
    const data = await api<any>(
      `/agent/card-batches/${encodeURIComponent(id)}/export`,
      { method: "POST" },
    );
    codes.value = data.codes || [];
    navigate("generate");
    toast("批次明文已加载");
  } catch (e) {
    toast(friendlyError(e));
  }
}
async function copyCodes() {
  try {
    await navigator.clipboard.writeText(codes.value.join("\n"));
    toast("卡密已复制");
  } catch {
    toast("复制失败，请使用下载");
  }
}
function download() {
  downloadText(
    `${codes.value.join("\n")}\n`,
    `yn-cards-${new Date().toISOString().slice(0, 10)}.txt`,
  );
}
onMounted(async () => {
  loadBranding("代理工作台").catch(() => undefined);
  addEventListener(
    "hashchange",
    () => (page.value = location.hash.slice(1) || "overview"),
  );
  if (token.value) {
    try {
      profile.value = await api("/agent/profile");
      await refresh();
    } catch (e) {
      loginError.value = friendlyError(e);
    }
  }
});
</script>
<template>
  <div v-if="!profile" class="login-page">
    <AppearanceMenu class="login-appearance" />
    <section class="login-art">
      <div class="login-copy">
        <small>PARTNER / DISTRIBUTION NODE</small>
        <h1>额度清晰。<br />发卡迅速。</h1>
        <p>代理业务从一个更安静、更敏捷的工作台开始。</p>
      </div>
    </section>
    <section class="login-panel">
      <div class="login-box">
        <div class="brand">
          <img :src="logoSource" alt="" />
          <div>
            <strong>{{ branding.site_name }}</strong
            ><span>PARTNER CONSOLE</span>
          </div>
        </div>
        <h2>代理工作台</h2>
        <p>使用用户中心的同一账号登录，无需代理代码。代理资格由管理员开通。</p>
        <form class="login-form" @submit.prevent="signIn">
          <CaptchaField ref="captchaField" v-model="login.captcha_code" />
          <label class="field"
            >登录名<input
              v-model="login.username"
              autocomplete="username"
              placeholder="账号或绑定邮箱"
              required /></label
          ><label class="field"
            >密码<input
              v-model="login.password"
              type="password"
              autocomplete="current-password"
              required /></label
          ><button class="btn" :disabled="loading">
            <LoaderCircle v-if="loading" class="spin" :size="17" /><KeyRound
              v-else
              :size="17"
            />进入工作台</button
          ><a href="/recover?from=agent">忘记密码</a><a href="/user-console/">进入用户中心</a>
          <p class="error">{{ loginError }}</p>
        </form>
      </div>
    </section>
  </div>
  <div v-else class="console">
    <aside class="sidebar">
      <div class="brand">
        <img :src="logoSource" alt="" />
        <div>
          <strong>代理工作台</strong
          ><span>{{ profile.agent_name || profile.agent_no }}</span>
        </div>
      </div>
      <nav class="nav" aria-label="代理功能">
        <div v-for="g in navGroups" :key="g.label" class="nav-group">
          <div class="nav-group-label">{{ g.label }}</div>
          <a
            v-for="item in g.items"
            v-show="!item.manage || canManage"
            :key="item.id"
            href="#"
            :class="{ active: page === item.id }"
            @click.prevent="navigate(item.id)"
            ><component :is="item.icon" :size="18" /><span>{{
              item.label
            }}</span></a
          >
        </div>
      </nav>
      <div class="sidebar-foot">PARTNER NODE / ONLINE</div>
    </aside>
    <main class="console-main">
      <header class="topbar">
        <div>
          <h1>{{
            navGroups
              .flatMap((g) => g.items)
              .find((p) => p.id === page)?.label
          }}</h1>
          <p>服务正常 · SECURE CONNECTION</p>
        </div>
        <div class="top-actions">
          <AppearanceMenu />
          <button class="icon-btn" title="刷新" @click="refresh">
            <RefreshCw :size="17" />
          </button>
          <UserMenu
            :name="profile.display_name || profile.username"
            :role="roleLabel(profile.role)"
            @logout="logout()"
          />
        </div>
      </header>
      <div class="workspace">
        <template v-if="page === 'overview'"
          ><section class="panel">
            <div class="panel-body agent-notice">
              <p>
                公开代理编号：<code>{{ profile.agent_no }}</code>
              </p>
              <a href="/" target="_blank" rel="noopener">打开公开验真首页</a>
              <a href="/user-console/">管理我购买的授权</a>
            </div>
          </section>
          <section class="hero-card">
            <div>
              <span class="hero-eyebrow">PARTNER NODE</span>
              <h2>业务脉搏</h2>
              <p>额度、卡密与授权状态保持实时同步。</p>
            </div>
          </section>
          <div class="metrics">
            <div v-for="(m, i) in metrics" :key="i" class="metric">
              <span class="metric-icon"
                ><component
                  :is="[Wallet, TicketCheck, Sparkles, BadgeCheck][i]"
                  :size="16"
              /></span>
              <strong>{{ m }}</strong>
              <small>{{
                ["可用额度", "卡密批次", "已生成卡密", "已激活授权"][i]
              }}</small>
            </div>
          </div>
          <section class="panel">
            <div class="panel-head"><h3>可售产品与额度</h3></div>
            <div class="table-wrap">
              <table class="table">
                <thead>
                  <tr>
                    <th>产品</th>
                    <th>可售时长</th>
                    <th>剩余额度</th>
                    <th>单批上限</th>
                    <th>明文导出</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="p in products" :key="p.id">
                    <td>
                      <b>{{ p.name }}</b
                      ><code>{{ p.id }}</code>
                    </td>
                    <td>
                      {{
                        [
                          ...(p.policy.allowed_duration_days || []).map(
                            (d: number) => `${d} 天`,
                          ),
                          ...(p.policy.allow_permanent ? ["永久"] : []),
                        ].join("、") || "未配置"
                      }}
                    </td>
                    <td>
                      <span
                        v-for="q in quotas.filter((q) => q.product_id === p.id)"
                        :key="q.id"
                        class="mono"
                        >{{
                          q.is_permanent ? "永久" : `${q.duration_days} 天`
                        }}: {{ q.balance }}<br
                      /></span>
                    </td>
                    <td>{{ p.policy.max_batch_quantity }}</td>
                    <td>
                      {{ p.policy.can_export_plain_code ? "允许" : "禁止" }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section></template
        >
        <div v-if="page === 'generate'" class="page-stack">
          <section class="section-intro">
            <div>
              <h2>生成卡密</h2>
              <p>
                选择管理员授予的套餐，按代理实际成本扣款；规格与价格由服务端核验。
              </p>
            </div>
          </section>
          <section class="panel">
            <form class="form-grid" @submit.prevent="generate">
              <label class="field"
                >授权套餐<select v-model="batch.package_id" required>
                  <option v-for="q in packages" :value="q.package.id">
                    {{ productName(q.package.product_id) }} /
                    {{ q.package.name }} ·
                    {{
                      q.package.is_permanent
                        ? "永久"
                        : `${q.package.duration_days} 天`
                    }}
                  </option>
                </select></label
              >
              <label class="field"
                >批次名称<input v-model="batch.name" required maxlength="160"
              /></label>
              <label class="field"
                >数量<input
                  v-model.number="batch.quantity"
                  type="number"
                  min="1"
                  :max="
                    Math.min(
                      products.find(
                        (p) => p.id === currentPackage?.package.product_id,
                      )?.policy.max_batch_quantity || 1000,
                      1000,
                    )
                  "
                  required
              /></label>
              <p>
                单张成本
                <strong>{{ money(currentPackage?.unit_cost || 0) }}</strong> ·
                本次合计
                <strong>{{
                  money((currentPackage?.unit_cost || 0) * batch.quantity)
                }}</strong>
                · 可用余额 <strong>{{ money(account?.balance || 0) }}</strong>
              </p>
              <p v-if="!packages.length">暂无已授权套餐，请联系管理员配置。</p>
              <div class="form-actions">
                <button class="btn" :disabled="issuing || !currentPackage">
                  <Sparkles :size="16" />{{ issuing ? "正在生成" : "生成卡密" }}
                </button>
              </div>
            </form>
            <div class="panel-head">
              <h3>本次明文卡密</h3>
              <div class="row-actions">
                <button
                  class="btn secondary small"
                  :disabled="!codes.length"
                  @click="copyCodes"
                >
                  <Clipboard :size="14" />复制</button
                ><button
                  class="btn secondary small"
                  :disabled="!codes.length"
                  @click="download"
                >
                  <Download :size="14" />下载
                </button>
              </div>
            </div>
            <pre class="code-box">{{
              codes.length ? codes.join("\n") : "生成或导出后显示明文卡密"
            }}</pre>
          </section>
          <section class="panel">
            <div class="panel-head"><h3>批次卡密状态</h3></div>
            <div class="table-wrap">
              <table class="table">
                <thead>
                  <tr>
                    <th>卡密前缀</th>
                    <th>状态</th>
                    <th>时长</th>
                    <th>创建时间</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="c in cards" :key="c.id">
                    <td>
                      <code>{{ c.code_prefix }}...</code>
                    </td>
                    <td><StatusBadge :status="c.status" /></td>
                    <td>
                      {{ c.is_permanent ? "永久" : `${c.duration_days} 天` }}
                    </td>
                    <td>{{ formatTime(c.created_at) }}</td>
                  </tr>
                </tbody>
              </table>
              <div v-if="!cards.length" class="empty">
                选择批次后查看卡密状态
              </div>
            </div>
          </section>
        </div>
        <section v-if="page === 'batches'" class="panel">
          <div class="panel-head"><h3>卡密批次</h3></div>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>批次</th>
                  <th>产品</th>
                  <th>数量</th>
                  <th>时长</th>
                  <th>创建时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="b in batches" :key="b.id">
                  <td>{{ b.name }}</td>
                  <td>{{ productName(b.product_id) }}</td>
                  <td>{{ b.quantity }}</td>
                  <td>
                    {{ b.is_permanent ? "永久" : `${b.duration_days} 天` }}
                  </td>
                  <td>{{ formatTime(b.created_at) }}</td>
                  <td>
                    <div class="row-actions">
                      <button
                        class="btn secondary small"
                        @click="showCards(b.id)"
                      >
                        状态</button
                      ><button
                        v-if="
                          canManage &&
                          products.find((p) => p.id === b.product_id)?.policy
                            .can_export_plain_code
                        "
                        class="btn small"
                        @click="exportBatch(b.id)"
                      >
                        导出
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
        <section v-if="page === 'licenses'" class="panel">
          <div class="panel-head"><h3>授权记录</h3></div>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>授权号</th>
                  <th>产品</th>
                  <th>状态</th>
                  <th>激活时间</th>
                  <th>使用到期</th>
                  <th>更新到期</th>
                  <th>最近验证</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="l in licenses" :key="l.id">
                  <td>
                    <code>{{ l.license_no }}</code>
                  </td>
                  <td>{{ productName(l.product_id) }}</td>
                  <td><StatusBadge :status="l.status" /></td>
                  <td>{{ formatTime(l.activated_at) }}</td>
                  <td>
                    {{ l.expired_at ? formatTime(l.expired_at) : "永久" }}
                  </td>
                  <td>
                    {{
                      l.updates_expired_at
                        ? formatTime(l.updates_expired_at)
                        : "永久更新"
                    }}
                  </td>
                  <td>{{ formatTime(l.last_verify_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
        <section v-if="page === 'ledgers'" class="panel">
          <div class="panel-head">
            <h3>资金流水</h3>
            <b>可用余额 {{ money(account?.balance || 0) }}</b>
          </div>
          <PagedRecords
            :api="api"
            :token="token"
            endpoint="/agent/history/money"
            kind="money"
          />
        </section>
        <section v-if="page === 'ledgers'" class="panel">
          <div class="panel-head"><h3>额度流水</h3></div>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>时间</th>
                  <th>产品</th>
                  <th>规格</th>
                  <th>类型</th>
                  <th>变动</th>
                  <th>余额</th>
                  <th>备注</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="l in ledgers" :key="l.id">
                  <td>{{ formatTime(l.created_at) }}</td>
                  <td>{{ productName(l.product_id) }}</td>
                  <td>
                    {{ l.is_permanent ? "永久" : `${l.duration_days} 天` }}
                  </td>
                  <td>
                    {{
                      (
                        {
                          grant: "额度发放",
                          revoke: "额度扣减",
                          generate_cards: "生成卡密",
                        } as any
                      )[l.change_type] || l.change_type
                    }}
                  </td>
                  <td
                    :style="{
                      color:
                        l.change_quantity >= 0 ? 'var(--green)' : 'var(--red)',
                    }"
                  >
                    {{ l.change_quantity >= 0 ? "+" : ""
                    }}{{ l.change_quantity }}
                  </td>
                  <td>{{ l.balance_after }}</td>
                  <td>{{ l.remark || "-" }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </main>
    <AppToast />
  </div>
</template>
<style scoped>
.agent-notice {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.agent-notice p {
  margin: 0;
}
</style>
