<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
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
} from "lucide-vue-next";
import { ApiError, downloadText, request } from "../shared/api";
import { formatTime, roleLabel } from "../shared/format";
import AppToast from "../shared/components/AppToast.vue";
import StatusBadge from "../shared/components/StatusBadge.vue";
import { toast } from "../shared/useToast";

const token = ref(sessionStorage.getItem("yn.agent_token") || "");
const profile = ref<any>();
const loading = ref(false);
const loginError = ref("");
const page = ref(location.hash.slice(1) || "overview");
const products = ref<any[]>([]),
  quotas = ref<any[]>([]),
  ledgers = ref<any[]>([]),
  batches = ref<any[]>([]),
  licenses = ref<any[]>([]),
  cards = ref<any[]>([]),
  codes = ref<string[]>([]);
const login = reactive({
  login_code: localStorage.getItem("yn.agent_login_code") || "",
  username: "",
  password: "",
});
const batch = reactive({
  product_id: "",
  name: "代理发卡",
  quantity: 1,
  duration_days: 30,
  is_permanent: false,
});
const pages = [
  { id: "overview", label: "业务概览", icon: ChartNoAxesCombined },
  { id: "generate", label: "生成卡密", icon: Sparkles, manage: true },
  { id: "batches", label: "卡密批次", icon: TicketCheck },
  { id: "licenses", label: "授权记录", icon: BadgeCheck },
  { id: "ledgers", label: "额度流水", icon: ListRestart },
];
const canManage = computed(() =>
  ["agent_owner", "agent_manager"].includes(profile.value?.role),
);
const currentProduct = computed(() =>
  products.value.find((p) => p.id === batch.product_id),
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
  loading.value = true;
  loginError.value = "";
  try {
    const data = await api<any>("/agent/login", {
      method: "POST",
      body: JSON.stringify(login),
    });
    token.value = data.access_token;
    sessionStorage.setItem("yn.agent_token", token.value);
    login.login_code = login.login_code.toUpperCase();
    localStorage.setItem("yn.agent_login_code", login.login_code);
    profile.value = await api("/agent/profile");
    login.password = "";
    await refresh();
  } catch (e: any) {
    loginError.value = e.message;
  } finally {
    loading.value = false;
  }
}
function logout(message = "") {
  token.value = "";
  profile.value = null;
  sessionStorage.removeItem("yn.agent_token");
  loginError.value = message;
}
async function refresh() {
  loading.value = true;
  try {
    const [p, q, l, b, li] = await Promise.all([
      api<any[]>("/agent/products"),
      api<any[]>("/agent/quotas"),
      api<any[]>("/agent/quota-ledgers"),
      api<any[]>("/agent/card-batches"),
      api<any[]>("/agent/licenses"),
    ]);
    products.value = p || [];
    quotas.value = q || [];
    ledgers.value = l || [];
    batches.value = b || [];
    licenses.value = li || [];
    if (!batch.product_id) batch.product_id = products.value[0]?.id || "";
    syncPolicy();
  } catch (e) {
    toast(e);
  } finally {
    loading.value = false;
  }
}
function syncPolicy() {
  const policy = currentProduct.value?.policy;
  batch.duration_days = policy?.allowed_duration_days?.[0] || 30;
  batch.is_permanent = false;
  batch.quantity = Math.min(batch.quantity, policy?.max_batch_quantity || 1);
}
function navigate(id: string) {
  if (id === "generate" && !canManage.value) return;
  page.value = id;
  location.hash = id;
}
async function generate() {
  try {
    const data = await api<any>("/agent/card-batches", {
      method: "POST",
      body: JSON.stringify(batch),
    });
    codes.value = data.codes || [];
    toast("卡密已生成，请及时保存明文");
    await refresh();
    await showCards(data.batch.id);
  } catch (e) {
    toast(e);
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
    toast(e);
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
    toast(e);
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
  addEventListener(
    "hashchange",
    () => (page.value = location.hash.slice(1) || "overview"),
  );
  if (token.value) {
    try {
      profile.value = await api("/agent/profile");
      await refresh();
    } catch (e) {
      loginError.value = e instanceof Error ? e.message : String(e);
    }
  }
});
</script>
<template>
  <div v-if="!profile" class="login-page">
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
          <img src="/assets/yunnuo-mark.svg" alt="" />
          <div><strong>允诺云授权</strong><span>PARTNER CONSOLE</span></div>
        </div>
        <h2>代理工作台</h2>
        <p>登录代码会在此设备上自动记忆。</p>
        <form class="login-form" @submit.prevent="signIn">
          <label class="field"
            >代理代码<input
              v-model="login.login_code"
              placeholder="YN-ABC123"
              autocomplete="organization"
              required
              autofocus /></label
          ><label class="field"
            >登录名<input
              v-model="login.username"
              autocomplete="username"
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
            />进入工作台
          </button>
          <p class="error">{{ loginError }}</p>
        </form>
      </div>
    </section>
  </div>
  <div v-else class="console">
    <aside class="sidebar">
      <div class="brand">
        <img src="/assets/yunnuo-mark.svg" alt="" />
        <div>
          <strong>代理工作台</strong
          ><span>{{ profile.agent_name || profile.agent_no }}</span>
        </div>
      </div>
      <nav class="nav">
        <a
          v-for="item in pages"
          v-show="!item.manage || canManage"
          :key="item.id"
          href="#"
          :class="{ active: page === item.id }"
          @click.prevent="navigate(item.id)"
          ><component :is="item.icon" :size="18" /><span>{{
            item.label
          }}</span></a
        >
      </nav>
      <div class="sidebar-foot">PARTNER NODE / ONLINE</div>
    </aside>
    <main class="console-main">
      <header class="topbar">
        <div>
          <h1>{{ pages.find((p) => p.id === page)?.label }}</h1>
          <p>服务正常 · SECURE CONNECTION</p>
        </div>
        <div class="top-actions">
          <span class="identity"
            >{{ profile.display_name || profile.username }} ·
            {{ roleLabel(profile.role) }}</span
          ><button class="icon-btn" title="刷新" @click="refresh">
            <RefreshCw :size="17" /></button
          ><button class="icon-btn" title="退出" @click="logout()">
            <LogOut :size="17" />
          </button>
        </div>
      </header>
      <div class="workspace">
        <template v-if="page === 'overview'"
          ><section class="section-intro">
            <div>
              <h2>业务脉搏</h2>
              <p>额度、卡密与授权状态保持实时同步。</p>
            </div>
          </section>
          <div class="metrics">
            <div v-for="(m, i) in metrics" :key="i" class="metric">
              <span>{{
                ["可用额度", "卡密批次", "已生成卡密", "已激活授权"][i]
              }}</span
              ><strong>{{ m }}</strong>
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
                      ><code>{{ p.code }}</code>
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
              <p>按产品政策与当前额度创建新的授权凭证。</p>
            </div>
          </section>
          <section class="panel">
            <form class="form-grid" @submit.prevent="generate">
              <label class="field"
                >产品<select v-model="batch.product_id" @change="syncPolicy">
                  <option
                    v-for="p in products.filter(
                      (p) => p.status === 'active' && p.policy.can_generate,
                    )"
                    :value="p.id"
                  >
                    {{ p.name }} · {{ p.code }}
                  </option>
                </select></label
              ><label class="field"
                >授权时长<select
                  v-model.number="batch.duration_days"
                  :disabled="batch.is_permanent"
                >
                  <option
                    v-for="d in currentProduct?.policy.allowed_duration_days ||
                    []"
                    :value="d"
                  >
                    {{ d }} 天
                  </option>
                </select></label
              ><label class="field"
                >批次名称<input v-model="batch.name" required /></label
              ><label class="field"
                >数量<input
                  v-model.number="batch.quantity"
                  type="number"
                  min="1"
                  :max="currentProduct?.policy.max_batch_quantity || 1"
                  required /></label
              ><label class="check"
                ><input
                  v-model="batch.is_permanent"
                  type="checkbox"
                  :disabled="!currentProduct?.policy.allow_permanent"
                />永久授权</label
              >
              <div class="form-actions">
                <button class="btn"><Sparkles :size="16" />生成卡密</button>
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
                  <th>到期时间</th>
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
                  <td>{{ formatTime(l.last_verify_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
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
