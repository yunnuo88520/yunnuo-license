<script setup lang="ts">
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import { computed, onMounted, reactive, ref, watch } from "vue";
import {
  Activity,
  BadgeCheck,
  BellRing,
  Boxes,
  CheckCheck,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Download,
  FileKey2,
  GitCommit,
  History,
  ImageUp,
  KeyRound,
  LayoutDashboard,
  LoaderCircle,
  LogOut,
  Plus,
  PackageCheck,
  RefreshCw,
  RotateCcw,
  ScrollText,
  Settings2,
  ShieldAlert,
  Siren,
  Terminal,
  TicketCheck,
  UserRoundCog,
  X,
} from "lucide-vue-next";
import { useAdmin } from "./useAdmin";
import { formatTime, roleLabel, shortId } from "../shared/format";
import { toast } from "../shared/useToast";
import AppToast from "../shared/components/AppToast.vue";
import StatusBadge from "../shared/components/StatusBadge.vue";
import ProductReleases from "../shared/components/ProductReleases.vue";
import LicensePolicy from "../shared/components/LicensePolicy.vue";
import CustomersPanel from "./CustomersPanel.vue";
import BindingPolicy from "./BindingPolicy.vue";
import OperationLogs from "./OperationLogs.vue";
import CommercePanel from "./CommercePanel.vue";
import RecoveryPanel from "./RecoveryPanel.vue";
import SecurityPanel from "./SecurityPanel.vue";
import FormDialog from "../shared/components/FormDialog.vue";
import UserMenu from "../shared/components/UserMenu.vue";
import {
  applyBranding,
  branding,
  loadBranding,
  logoSource,
} from "../shared/branding";
const a = useAdmin();
const page = ref(location.hash.slice(1) || "overview"),
  action = ref("");
const settingsTab = ref("branding");
const settingTabs = [
  { id: "branding", label: "站点品牌" },
  { id: "account", label: "账号与安全" },
  { id: "version", label: "版本与升级" },
];
const agentActionTitles: Record<string, string> = {
  agent: "新增代理",
  user: "新增代理账号",
  policy: "代理授权规则",
  quota: "发放额度",
  agentBatch: "代理批量发卡",
};
const login = reactive({ username: "", password: "" });
const result = ref("{}");
const codes = ref<string[]>([]);
const siteForm = reactive({
  site_name: "允诺云授权",
  browser_title: "允诺云授权",
  logo_data_url: "",
  favicon_data_url: "",
});
const navHome = { id: "overview", label: "业务概览", icon: LayoutDashboard };
const navGroups = [
  {
    label: "产品",
    items: [
      { id: "products", label: "产品", icon: Boxes },
      { id: "releases", label: "产品升级", icon: PackageCheck },
    ],
  },
  {
    label: "代理",
    items: [
      { id: "agents", label: "代理", icon: UserRoundCog },
      { id: "commerce", label: "套餐与结算", icon: TicketCheck },
    ],
  },
  {
    label: "发卡",
    items: [
      { id: "cards", label: "卡密", icon: TicketCheck },
      { id: "licenses", label: "授权", icon: BadgeCheck },
      { id: "offline", label: "离线授权", icon: FileKey2 },
    ],
  },
  {
    label: "客户",
    items: [
      { id: "customers", label: "用户", icon: UserRoundCog },
      { id: "recovery", label: "账号恢复", icon: KeyRound },
    ],
  },
  {
    label: "安全",
    items: [
      { id: "risk", label: "风控", icon: ShieldAlert },
      { id: "security", label: "盗版处置", icon: ShieldAlert },
    ],
  },
  {
    label: "平台",
    items: [
      { id: "tools", label: "联调工具", icon: Terminal },
      { id: "audit", label: "审计", icon: ScrollText },
      { id: "admins", label: "系统", icon: Settings2 },
    ],
  },
];
const visibleGroups = computed(() =>
  navGroups
    .map((g) => ({
      ...g,
      items: g.items.filter((n) => n.id !== "recovery" || a.isManager.value),
    }))
    .filter((g) => g.items.length),
);
/* Collapse state: true/false are explicit user choices, undefined defers to
   "expand the group that holds the current page". */
const expandedGroups = reactive<Record<string, boolean | undefined>>(
  JSON.parse(localStorage.getItem("yn.admin_nav_expanded") || "{}"),
);
function isCollapsed(label: string) {
  const pref = expandedGroups[label];
  if (pref !== undefined) return !pref;
  const group = visibleGroups.value.find((g) => g.label === label);
  return !group?.items.some((n) => n.id === page.value);
}
function toggleGroup(label: string) {
  expandedGroups[label] = isCollapsed(label);
  localStorage.setItem(
    "yn.admin_nav_expanded",
    JSON.stringify(expandedGroups),
  );
}
const title = computed(
  () =>
    [navHome, ...navGroups.flatMap((g) => g.items)].find(
      (n) => n.id === page.value,
    )?.label || "业务概览",
);
const cardsTab = ref("list");
const licensesTab = ref("list");
const riskTab = ref("alerts");
const auditTab = ref("ops");
const releasesTab = ref("policy");
const pname = (id: string) =>
  a.data.products.find((p: any) => p.id === id)?.name || id;
const aname = (id: string) =>
  a.data.agents.find((p: any) => p.id === id)?.name || id;
const canOperate = computed(() =>
  ["super_admin", "admin", "operator"].includes(a.profile.value?.role),
);
function go(id: string) {
  page.value = id;
  location.hash = id;
  action.value = "";
}
function values(e: Event) {
  const f = e.currentTarget as HTMLFormElement;
  const o: any = Object.fromEntries(new FormData(f));
  f.querySelectorAll<HTMLInputElement>('input[type="checkbox"]').forEach(
    (i) => (o[i.name] = i.checked),
  );
  return o;
}
async function create(
  path: string,
  e: Event,
  message: string,
  transform?: (x: any) => void,
) {
  const o = values(e);
  transform?.(o);
  const r = await a.mutate(path, o, message);
  action.value = "";
  return r;
}
async function productCreate(e: Event) {
  await create("/admin/products", e, "产品已创建", (o) =>
    [
      "max_bind_count",
      "offline_grace_days",
      "expire_grace_days",
      "update_duration_days",
    ].forEach((k) => (o[k] = Number(o[k]))),
  );
}
async function agentTask(kind: string, e: Event) {
  const o = values(e);
  if (kind === "create") return create("/admin/agents", e, "代理已创建");
  const id = o.agent_id;
  delete o.agent_id;
  if (kind === "user")
    return a.mutate(`/admin/agents/${id}/users`, o, "代理账号已创建");
  if (kind === "policy") {
    o.allowed_duration_days = o.allowed_duration_days
      .split(",")
      .map(Number)
      .filter(Boolean);
    o.max_batch_quantity = Number(o.max_batch_quantity);
    return a.mutate(
      "/admin/agent-policies",
      { ...o, agent_id: id },
      "代理政策已保存",
    );
  }
  if (kind === "quota") {
    o.quantity = Number(o.quantity);
    o.duration_days = Number(o.duration_days);
    o.is_permanent = false;
    return a.mutate(
      "/admin/agent-quotas/grant",
      { ...o, agent_id: id },
      "额度已发放",
    );
  }
  o.quantity = Number(o.quantity);
  o.duration_days = Number(o.duration_days);
  const r = await a.mutate(
    `/admin/agents/${id}/card-batches`,
    o,
    "代理卡密已生成",
  );
  codes.value = r.codes || [];
}
async function batchCreate(e: Event) {
  const r = await create("/admin/card-batches", e, "卡密已生成", (o) => {
    o.quantity = Number(o.quantity);
    o.duration_days = Number(o.duration_days);
  });
  codes.value = r.codes || [];
}
async function exportBatch() {
  const r = await a.mutate(
    `/admin/card-batches/${a.selected.batch}/export`,
    undefined,
    "批次明文已导出",
  );
  codes.value = r.codes || [];
}
const productEdit = reactive({
  id: "",
  name: "",
  bind_mode: "device",
  max_bind_count: 1,
  bind_conflict_strategy: "reject",
  offline_grace_days: 15,
});
function openProductEdit(p: any) {
  Object.assign(productEdit, {
    id: p.id,
    name: p.name,
    bind_mode: p.bind_mode,
    max_bind_count: p.max_bind_count,
    bind_conflict_strategy: p.bind_conflict_strategy,
    offline_grace_days: p.offline_grace_days,
  });
  action.value = "productEdit";
}
async function productUpdate(e: Event) {
  const o = values(e);
  o.max_bind_count = Number(o.max_bind_count);
  o.offline_grace_days = Number(o.offline_grace_days);
  await a.mutate(`/admin/products/${productEdit.id}/update`, o, "产品已更新");
  action.value = "";
}
const agentEdit = reactive({ id: "", name: "", contact_name: "", phone: "" });
const selectedAgentName = computed(
  () =>
    a.data.agents.find((x: any) => x.id === a.selected.agent)?.name || "",
);
function openAgentEdit(x: any) {
  Object.assign(agentEdit, {
    id: x.id,
    name: x.name,
    contact_name: x.contact_name || "",
    phone: x.phone || "",
  });
  action.value = "agentEdit";
}
async function agentUpdate(e: Event) {
  const o = values(e);
  await a.mutate(`/admin/agents/${agentEdit.id}/update`, o, "代理信息已更新");
  action.value = "";
}
async function openBatch(id: string) {
  await a.showBatch(id);
  cardsTab.value = "detail";
}
async function openBindings(no: string) {
  await a.showBindings(no);
  licensesTab.value = "bindings";
}
async function tools(kind: string, e: Event) {
  const o = values(e);
  const p =
    a.data.products.find((x: any) => x.id === o.product_id) ||
    a.data.products.find((x: any) => x.id === a.selected.product);
  try {
    const payload =
      kind === "activate"
        ? {
            app_key: p.app_key,
            card_code: o.card_code,
            bind_mode: p.bind_mode,
            bind_value: o.bind_value,
            device_name: o.device_name,
          }
        : {
            app_key: p.app_key,
            license_no: o.license_no,
            bind_mode: p.bind_mode,
            bind_value: o.bind_value,
          };
    const r = await a.api(`/v1/licenses/${kind}`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
    result.value = JSON.stringify(r, null, 2);
    toast(kind === "activate" ? "激活成功" : "验证通过");
    await a.refresh();
  } catch (e) {
    toast(e);
  }
}
async function downloadOffline(id: string) {
  const r = await fetch(`/admin/offline-licenses/${id}/download`, {
    headers: { Authorization: `Bearer ${a.token.value}` },
  });
  if (!r.ok) return toast("下载失败");
  const url = URL.createObjectURL(await r.blob());
  const l = document.createElement("a");
  l.href = url;
  l.download = "license.key";
  l.click();
  URL.revokeObjectURL(url);
}
async function changePage(d: number) {
  a.data.licensePage.page += d;
  await a.refresh();
}
watch(
  () => a.data.siteSettings,
  (settings) => {
    if (!settings) return;
    siteForm.site_name = settings.site_name;
    siteForm.browser_title = settings.browser_title;
    siteForm.logo_data_url = settings.logo_data_url || "";
    siteForm.favicon_data_url = settings.favicon_data_url || "";
  },
  { immediate: true },
);
async function readBrandAsset(
  event: Event,
  field: "logo_data_url" | "favicon_data_url",
) {
  const input = event.currentTarget as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  if (file.size > 512 * 1024) return toast("图片不能超过 512 KB");
  if (
    ![
      "image/png",
      "image/jpeg",
      "image/webp",
      "image/x-icon",
      "image/vnd.microsoft.icon",
    ].includes(file.type)
  )
    return toast("仅支持 PNG、JPEG、WebP 或 ICO 图片");
  siteForm[field] = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}
async function saveSiteSettings() {
  const settings = await a.mutate(
    "/admin/site-settings",
    {
      site_name: siteForm.site_name,
      browser_title: siteForm.browser_title,
      logo_data_url: siteForm.logo_data_url,
      favicon_data_url: siteForm.favicon_data_url,
    },
    "站点设置已保存",
    false,
  );
  a.data.siteSettings = settings;
  applyBranding(settings, "管理控制台");
}
onMounted(async () => {
  loadBranding("管理控制台").catch(() => undefined);
  addEventListener(
    "hashchange",
    () => (page.value = location.hash.slice(1) || "overview"),
  );
  if (a.token.value) {
    try {
      a.profile.value = await a.api("/admin/profile");
      await a.refresh();
    } catch (e) {
      a.loginError.value = e instanceof Error ? e.message : String(e);
    }
  }
});
</script>
<template>
  <div v-if="!a.profile.value" class="login-page">
    <AppearanceMenu class="login-appearance" />
    <section class="login-art admin-art">
      <div class="login-copy">
        <small>CONTROL / AUTHORIZATION MATRIX</small>
        <h1>控制发生。<br />秩序可见。</h1>
        <p>产品、代理、卡密、授权和风险，在一个完整视野中协同。</p>
      </div>
    </section>
    <section class="login-panel">
      <div class="login-box">
        <div class="brand">
          <img :src="logoSource" alt="" />
          <div>
            <strong>{{ branding.site_name }}</strong
            ><span>ADMIN CONSOLE</span>
          </div>
        </div>
        <h2>管理控制台</h2>
        <p>使用管理员账号进入安全控制域。</p>
        <form class="login-form" @submit.prevent="a.signIn(login)">
          <label class="field"
            >账号<input
              v-model="login.username"
              autocomplete="username"
              autofocus
              required /></label
          ><label class="field"
            >密码<input
              v-model="login.password"
              type="password"
              autocomplete="current-password"
              required /></label
          ><button class="btn" :disabled="a.loading.value">
            <LoaderCircle
              v-if="a.loading.value"
              class="spin"
              :size="17"
            /><KeyRound v-else :size="17" />安全登录</button
          ><a href="/recover">忘记密码</a>
          <p class="error">{{ a.loginError.value }}</p>
        </form>
      </div>
    </section>
  </div>
  <div v-else class="console">
    <aside class="sidebar">
      <div class="brand">
        <img :src="logoSource" alt="" />
        <div><strong>管理控制台</strong><span>YUNNUO CLOUD</span></div>
      </div>
      <nav class="nav" aria-label="管理功能">
        <a
          href="#"
          :class="{ active: page === navHome.id }"
          :aria-label="navHome.label"
          :title="navHome.label"
          @click.prevent="go(navHome.id)"
          ><component :is="navHome.icon" :size="18" /><span>{{
            navHome.label
          }}</span></a
        >
        <div
          v-for="g in visibleGroups"
          :key="g.label"
          class="nav-group"
          :class="{ collapsed: isCollapsed(g.label) }"
        >
          <button
            type="button"
            class="nav-group-toggle"
            :aria-expanded="!isCollapsed(g.label)"
            @click="toggleGroup(g.label)"
          >
            <span>{{ g.label }}</span>
            <ChevronDown :size="13" />
          </button>
          <div class="nav-group-items">
            <a
              v-for="n in g.items"
              :key="n.id"
              href="#"
              :class="{ active: page === n.id }"
              :aria-label="n.label"
              :title="n.label"
              @click.prevent="go(n.id)"
              ><component :is="n.icon" :size="18" /><span>{{
                n.label
              }}</span></a
            >
          </div>
        </div>
      </nav>
      <div class="sidebar-foot">
        SECURE NODE / ONLINE<br />V{{
          a.data.system?.current_version || "…"
        }}
      </div>
    </aside>
    <main class="console-main">
      <header class="topbar">
        <div>
          <h1>{{ title }}</h1>
          <p>服务正常 · AUTHORIZATION CORE</p>
        </div>
        <div class="top-actions">
          <AppearanceMenu />
          <button class="icon-btn" title="刷新" @click="a.refresh">
            <RefreshCw :size="17" />
          </button>
          <UserMenu
            :name="
              a.profile.value.display_name || a.profile.value.username
            "
            :role="roleLabel(a.profile.value.role)"
            password-action="修改密码"
            @password="action = 'password'"
            @logout="a.logout()"
          />
        </div>
      </header>
      <div class="workspace">
        <template v-if="page === 'overview'"
          ><section class="hero-card">
            <div>
              <span class="hero-eyebrow">AUTHORIZATION CORE</span>
              <h2>授权运行态</h2>
              <p>产品供给、发卡与授权验证的核心状态聚合于此。</p>
            </div>
            <button
              v-if="canOperate"
              class="btn"
              @click="
                go('cards');
                action = 'batch';
              "
            >
              <Plus :size="16" />生成卡密
            </button>
          </section>
          <div class="metrics five">
            <div
              v-for="(v, i) in [
                a.data.products.length,
                a.data.batches.length,
                a.data.agents.length,
                a.data.licensePage.total,
                a.data.licenses.filter((x: any) => x.status === 'active')
                  .length,
              ]"
              class="metric"
            >
              <span class="metric-icon"
                ><component
                  :is="[Boxes, TicketCheck, UserRoundCog, BadgeCheck, Activity][i]"
                  :size="16"
              /></span>
              <strong>{{ v }}</strong>
              <small>{{ ["产品", "批次", "代理", "授权", "当前有效"][i] }}</small>
            </div>
          </div>
          <div class="overview-grid">
            <section class="panel">
              <div class="panel-head">
                <h3>最近授权</h3>
                <button class="btn secondary small" @click="go('licenses')">
                  全部授权
                </button>
              </div>
              <button
                v-for="l in a.data.licenses.slice(0, 5)"
                class="activity-row"
                @click="go('licenses')"
              >
                <span
                  ><b>{{ pname(l.product_id) }}</b
                  ><code>{{ l.license_no }}</code></span
                ><span
                  ><StatusBadge :status="l.status" /><small>{{
                    formatTime(l.activated_at)
                  }}</small></span
                >
              </button>
              <p v-if="!a.data.licenses.length" class="empty">
                暂无授权记录，生成卡密并激活后显示。
              </p>
            </section>
            <section class="panel">
              <div class="panel-head">
                <h3>最近批次</h3>
                <button class="btn secondary small" @click="go('cards')">
                  全部批次
                </button>
              </div>
              <button
                v-for="b in a.data.batches.slice(0, 5)"
                class="activity-row"
                @click="go('cards')"
              >
                <span
                  ><b>{{ b.name }}</b
                  ><small>{{ pname(b.product_id) }}</small></span
                ><span
                  ><b>{{ b.quantity }} 张</b
                  ><small>{{ formatTime(b.created_at) }}</small></span
                >
              </button>
              <p v-if="!a.data.batches.length" class="empty">暂无卡密批次。</p>
            </section>
          </div></template
        >
        <section v-if="page === 'products'" class="panel">
          <div class="panel-head">
            <h3>产品管理</h3>
            <button
              v-if="a.isManager.value"
              class="btn small"
              @click="action = action === 'product' ? '' : 'product'"
            >
              <Plus :size="14" />新增产品
            </button>
          </div>
          <FormDialog
            :open="action === 'product'"
            title="新增产品"
            description="保存后生成 AppKey 与验签公钥；期限规则在首次发卡前可调整。"
            @close="action = ''"
          >
            <form
              class="form-grid"
              @submit.prevent="productCreate"
            >
            <label class="field"
              >授权模式<select name="license_mode">
                <option value="subscription">订阅使用（到期停用）</option>
                <option value="perpetual">永久使用、限期更新</option>
              </select></label
            >
            <label class="field"
              >更新天数（0 跟随卡密期限）<input
                name="update_duration_days"
                type="number"
                min="0"
                max="36500"
                value="0"
                required
            /></label>
            <label class="field">产品名称<input name="name" required /></label
            ><label class="field"
              >产品编码<input name="code" maxlength="6" required /></label
            ><label class="field"
              >绑定模式<select name="bind_mode">
                <option value="device">设备</option>
                <option value="account">账号</option>
                <option value="domain">域名</option>
                <option value="ip">IP</option>
                <option value="none">无绑定</option>
              </select></label
            ><label class="field"
              >最大绑定数<input
                name="max_bind_count"
                type="number"
                value="1" /></label
            ><label class="field"
              >超限策略<select name="bind_conflict_strategy">
                <option value="reject">拒绝</option>
                <option value="kick_oldest">移除最久未活跃</option>
              </select></label
            ><label class="field"
              >离线天数<input
                name="offline_grace_days"
                type="number"
                value="15" /></label
            ><label class="field"
              >使用到期宽限天数<input
                name="expire_grace_days"
                type="number"
                min="0"
                max="365"
                value="0"
            /></label>
            <div class="form-actions">
              <button class="btn">确认新增</button>
            </div>
            </form>
          </FormDialog>
          <FormDialog
            :open="action === 'productEdit'"
            title="编辑产品"
            description="调整绑定与离线策略；期限规则请在“产品升级 → 策略与规则”中维护。"
            @close="action = ''"
          >
            <form class="form-grid" @submit.prevent="productUpdate">
              <label class="field"
                >产品名称<input name="name" :value="productEdit.name" required
              /></label>
              <label class="field"
                >绑定模式<select name="bind_mode" :value="productEdit.bind_mode">
                  <option value="device">设备</option>
                  <option value="account">账号</option>
                  <option value="domain">域名</option>
                  <option value="ip">IP</option>
                  <option value="none">无绑定</option>
                </select></label
              >
              <label class="field"
                >最大绑定数<input
                  name="max_bind_count"
                  type="number"
                  min="1"
                  :value="productEdit.max_bind_count"
                  required
              /></label>
              <label class="field"
                >超限策略<select
                  name="bind_conflict_strategy"
                  :value="productEdit.bind_conflict_strategy"
                >
                  <option value="reject">拒绝</option>
                  <option value="kick_oldest">移除最久未活跃</option>
                </select></label
              >
              <label class="field"
                >离线天数<input
                  name="offline_grace_days"
                  type="number"
                  min="0"
                  max="36500"
                  :value="productEdit.offline_grace_days"
                  required
              /></label>
              <div class="form-actions">
                <button class="btn">保存修改</button>
              </div>
            </form>
          </FormDialog>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>编码</th>
                  <th>AppKey</th>
                  <th>密钥版本</th>
                  <th>绑定</th>
                  <th>席位</th>
                  <th>授权模式</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="p in a.data.products">
                  <td>{{ p.name }}</td>
                  <td>{{ p.code }}</td>
                  <td>
                    <code>{{ p.app_key }}</code>
                  </td>
                  <td>v{{ p.key_version || 1 }}</td>
                  <td>{{ p.bind_mode }}</td>
                  <td>{{ p.max_bind_count }}</td>
                  <td>
                    {{
                      p.license_mode === "perpetual"
                        ? "永久使用、限期更新"
                        : "订阅使用"
                    }}
                  </td>
                  <td><StatusBadge :status="p.status" /></td>
                  <td>
                    <div class="row-actions">
                      <button
                        v-if="a.isManager.value"
                        class="btn secondary small"
                        @click="openProductEdit(p)"
                      >
                        编辑</button
                      ><button
                        class="btn secondary small"
                        @click="a.showKeys(p.id)"
                      >
                        公钥</button
                      ><button
                        v-if="a.isSuper.value"
                        class="btn secondary small"
                        @click="
                          a
                            .mutate(
                              `/admin/products/${p.id}/keys/rotate`,
                              undefined,
                              '密钥已轮换',
                            )
                            .then(() => a.showKeys(p.id))
                        "
                      >
                        轮换</button
                      ><button
                        v-if="a.isManager.value"
                        class="btn danger small"
                        @click="
                          a.mutate(
                            `/admin/products/${p.id}/${p.status === 'active' ? 'disable' : 'enable'}`,
                            undefined,
                            p.status === 'active' ? '产品已停用' : '产品已恢复',
                          )
                        "
                      >
                        {{ p.status === "active" ? "停用" : "恢复" }}
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
        <RecoveryPanel
          v-if="page === 'recovery' && a.isManager.value"
          :api="a.api"
          :agents="a.data.agents"
        />
        <SecurityPanel
          :token="a.token.value"
          v-if="page === 'security'"
          :api="a.api"
          :products="a.data.products"
          :can-write="a.isManager.value"
        />
        <CommercePanel
          :token="a.token.value"
          v-if="page === 'commerce'"
          :api="a.api"
          :products="a.data.products"
          :agents="a.data.agents"
          :can-write="a.isManager.value"
        />
        <CustomersPanel
          v-if="page === 'customers'"
          :api="a.api"
          :can-write="a.isManager.value"
        />
        <section v-if="page === 'releases'" class="panel">
          <div class="panel-head">
            <div>
              <h3>产品版本与升级包</h3>
              <p>选择产品后管理换绑规则、期限策略与升级包发布。</p>
            </div>
          </div>
          <div class="form-grid">
            <label class="field wide"
              >选择产品<select v-model="a.selected.product">
                <option value="" disabled>请选择产品</option>
                <option v-for="p in a.data.products" :key="p.id" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            >
          </div>
          <p v-if="!a.data.products.length" class="empty">请先创建产品。</p>
          <template v-if="a.selected.product">
            <div class="tabbar" role="tablist" aria-label="产品升级视图">
              <button
                type="button"
                role="tab"
                :aria-selected="releasesTab === 'policy'"
                :class="{ active: releasesTab === 'policy' }"
                @click="releasesTab = 'policy'"
              >
                策略与规则
              </button>
              <button
                type="button"
                role="tab"
                :aria-selected="releasesTab === 'release'"
                :class="{ active: releasesTab === 'release' }"
                @click="releasesTab = 'release'"
              >
                升级包
              </button>
            </div>
            <template v-if="releasesTab === 'policy'">
            <BindingPolicy
              :key="'binding-' + a.selected.product"
              :product-id="a.selected.product"
              :api="a.api"
              :can-write="a.isManager.value"
            />
            <LicensePolicy
              :key="'policy-' + a.selected.product"
              :product-id="a.selected.product"
              :api="a.api"
              prefix="/admin"
              :can-write="a.isManager.value"
              @saved="a.refresh"
            />
            <form
              v-if="a.isSuper.value"
              class="form-grid"
              @submit.prevent="
                create(
                  `/admin/products/${a.selected.product}/developer`,
                  $event,
                  '开发者归属已保存',
                )
              "
            >
              <label class="field"
                >产品所属开发者<select
                  name="developer_id"
                  :key="
                    a.selected.product +
                    (a.data.products.find(
                      (p: any) => p.id === a.selected.product,
                    )?.developer_id || '')
                  "
                  :value="
                    a.data.products.find(
                      (p: any) => p.id === a.selected.product,
                    )?.developer_id || ''
                  "
                >
                  <option value="">仅由站点管理</option>
                  <option
                    v-for="u in a.data.users.filter(
                      (u: any) =>
                        u.role === 'developer' && u.status === 'active',
                    )"
                    :key="u.id"
                    :value="u.id"
                  >
                    {{ u.display_name || u.username }}
                  </option>
                </select></label
              >
              <button class="btn secondary">保存产品归属</button>
            </form>
            </template>
            <ProductReleases
              v-else
              :key="a.selected.product"
              :product-id="a.selected.product"
              :api="a.api"
              prefix="/admin"
              :can-write="a.isManager.value"
            />
          </template>
        </section>
        <section v-if="page === 'agents'" class="panel">
          <div class="panel-head">
            <h3>代理管理</h3>
            <button
              v-if="a.isManager.value"
              class="btn small"
              @click="action = action === 'agent' ? '' : 'agent'"
            >
              <Plus :size="14" />新增代理
            </button>
          </div>
          <FormDialog
            :open="['agent', 'user', 'policy', 'quota', 'agentBatch'].includes(action)"
            :title="agentActionTitles[action] || '代理操作'"
            @close="action = ''"
          >
          <form
            class="form-grid"
            @submit.prevent="
              agentTask(action === 'agent' ? 'create' : action, $event)
            "
          >
            <label v-if="action === 'agent'" class="field"
              >代理名称<input name="name" required /></label
            ><template v-else
              ><label class="field"
                >代理<select name="agent_id">
                  <option v-for="x in a.data.agents" :value="x.id">
                    {{ x.name }} · {{ x.login_code || x.agent_no }}
                  </option>
                </select></label
              ></template
            ><template v-if="action === 'agent'"
              ><label class="field">联系人<input name="contact_name" /></label
              ><label class="field">手机<input name="phone" /></label></template
            ><template v-if="action === 'user'"
              ><label class="field"
                >登录名<input name="username" required /></label
              ><label class="field"
                >初始密码<input
                  name="password"
                  type="password"
                  minlength="8"
                  required /></label
              ><label class="field">显示名<input name="display_name" /></label
              ><label class="field"
                >角色<select name="role">
                  <option value="agent_owner">主账号</option>
                  <option value="agent_manager">经理</option>
                  <option value="agent_staff">员工</option>
                  <option value="agent_readonly">只读</option>
                </select></label
              ></template
            ><template v-if="action === 'policy'"
              ><label class="field"
                >产品<select name="product_id">
                  <option v-for="p in a.data.products" :value="p.id">
                    {{ p.name }}
                  </option>
                </select></label
              ><label class="field"
                >允许天数<input
                  name="allowed_duration_days"
                  value="30,90,365" /></label
              ><label class="field"
                >单批上限<input
                  name="max_batch_quantity"
                  type="number"
                  value="100" /></label
              ><label class="check"
                ><input
                  name="can_generate"
                  type="checkbox"
                  checked
                />允许生成</label
              ><label class="check"
                ><input
                  name="can_export_plain_code"
                  type="checkbox"
                />允许明文导出</label
              ><label class="check"
                ><input
                  name="allow_permanent"
                  type="checkbox"
                />允许永久卡</label
              ></template
            ><template v-if="action === 'quota' || action === 'agentBatch'"
              ><label class="field"
                >产品<select name="product_id">
                  <option v-for="p in a.data.products" :value="p.id">
                    {{ p.name }}
                  </option>
                </select></label
              ><label v-if="action === 'agentBatch'" class="field"
                >批次名称<input name="name" value="代理自助批次" /></label
              ><label class="field"
                >天数<input
                  name="duration_days"
                  type="number"
                  value="30" /></label
              ><label class="field"
                >数量<input name="quantity" type="number" value="10" /></label
            ></template>
            <div class="form-actions">
              <button class="btn">确认提交</button>
            </div>
          </form>
          </FormDialog>
          <FormDialog
            :open="action === 'agentEdit'"
            title="编辑代理"
            :description="`更新 ${agentEdit.name} 的联系信息；登录代码与编号不可变更。`"
            @close="action = ''"
          >
            <form class="form-grid" @submit.prevent="agentUpdate">
              <label class="field"
                >代理名称<input name="name" :value="agentEdit.name" required
              /></label>
              <label class="field"
                >联系人<input name="contact_name" :value="agentEdit.contact_name"
              /></label>
              <label class="field"
                >手机<input name="phone" :value="agentEdit.phone" /></label>
              <div class="form-actions">
                <button class="btn">保存修改</button>
              </div>
            </form>
          </FormDialog>
          <div class="agent-grid">
            <div class="table-wrap">
              <table class="table">
                <thead>
                  <tr>
                    <th>代理</th>
                    <th>内部登录代码</th>
                    <th>公开代理编号</th>
                    <th>联系人</th>
                    <th>状态</th>
                    <th>创建时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="x in a.data.agents"
                    :class="{ 'row-active': a.selected.agent === x.id }"
                  >
                    <td>{{ x.name }}</td>
                    <td>
                      <code>{{ x.login_code || x.agent_no }}</code>
                    </td>
                    <td>
                      <code>{{ x.agent_no }}</code>
                    </td>
                    <td>{{ x.contact_name || "-" }}</td>
                    <td><StatusBadge :status="x.status" /></td>
                    <td>{{ formatTime(x.created_at) }}</td>
                    <td>
                      <div class="row-actions">
                        <button
                          v-if="a.isManager.value"
                          class="btn secondary small"
                          @click="openAgentEdit(x)"
                        >
                          编辑</button
                        ><button
                          class="btn secondary small"
                          @click="a.showAgent(x.id)"
                        >
                          详情</button
                        ><button
                          v-if="a.isManager.value"
                          class="btn danger small"
                          @click="
                            a.mutate(
                              `/admin/agents/${x.id}/${x.status === 'active' ? 'suspend' : 'enable'}`,
                              undefined,
                              '代理状态已更新',
                            )
                          "
                        >
                          {{ x.status === "active" ? "暂停" : "恢复" }}
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <aside class="detail-pane">
              <div class="detail-head">
                <h4>{{ selectedAgentName || "代理详情" }}</h4>
                <div v-if="a.isManager.value && a.selected.agent" class="row-actions">
                  <button class="btn secondary small" @click="action = 'user'">
                    新增账号
                  </button>
                  <button class="btn secondary small" @click="action = 'policy'">
                    授权规则
                  </button>
                  <button class="btn secondary small" @click="action = 'quota'">
                    发放额度
                  </button>
                </div>
              </div>
              <template v-if="a.selected.agent">
                <h5>代理账号</h5>
                <div v-for="u in a.data.agentUsers" class="detail-row">
                  <span
                    ><b>{{ u.display_name || u.username }}</b
                    ><code>{{ u.username }} · {{ u.role }}</code></span
                  ><span class="row-actions"
                    ><StatusBadge :status="u.status" />
                    <button
                      v-if="a.isManager.value"
                      class="btn secondary small"
                      @click="
                        a.mutate(
                          `/admin/agents/${a.selected.agent}/users/${u.id}/${u.status === 'active' ? 'disable' : 'enable'}`,
                          undefined,
                          '代理账号状态已更新',
                        )
                      "
                    >
                      {{ u.status === "active" ? "停用" : "启用" }}
                    </button></span
                  >
                </div>
                <p v-if="!a.data.agentUsers.length" class="pane-empty">
                  该代理还没有登录账号，点击“新增账号”创建。
                </p>
                <h5>产品额度</h5>
                <div v-for="q in a.data.agentQuotas" class="detail-row">
                  <span
                    ><b>{{ pname(q.product_id) }}</b
                    ><code>{{
                      q.is_permanent ? "永久" : `${q.duration_days} 天`
                    }}</code></span
                  ><strong>{{ q.balance }}</strong>
                </div>
                <p v-if="!a.data.agentQuotas.length" class="pane-empty">
                  尚未发放额度，点击“发放额度”为该代理充值。
                </p>
              </template>
              <p v-else class="pane-empty">
                点击左侧代理的“详情”，在这里管理它的账号与额度。
              </p>
            </aside>
          </div>
        </section>
        <section v-if="page === 'cards'" class="panel">
          <div class="panel-head">
            <h3>卡密批次</h3>
            <button
              v-if="canOperate"
              class="btn small"
              @click="action = action === 'batch' ? '' : 'batch'"
            >
              <Plus :size="14" />生成卡密
            </button>
          </div>
          <FormDialog
            :open="action === 'batch'"
            title="生成卡密"
            @close="action = ''"
          >
          <form
            class="form-grid"
            @submit.prevent="batchCreate"
          >
            <label class="field"
              >产品<select name="product_id">
                <option v-for="p in a.data.products" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            ><label class="field"
              >批次名称<input name="name" value="新批次" required /></label
            ><label class="field"
              >数量<input name="quantity" type="number" value="5" /></label
            ><label class="field"
              >天数<input
                name="duration_days"
                type="number"
                value="30" /></label
            ><label class="check"
              ><input name="is_permanent" type="checkbox" />永久</label
            >
            <div class="form-actions">
              <button class="btn">生成卡密</button>
            </div>
          </form>
          </FormDialog>
          <div class="tabbar" role="tablist" aria-label="卡密视图">
            <button
              type="button"
              role="tab"
              :aria-selected="cardsTab === 'list'"
              :class="{ active: cardsTab === 'list' }"
              @click="cardsTab = 'list'"
            >
              批次列表
            </button>
            <button
              type="button"
              role="tab"
              :aria-selected="cardsTab === 'detail'"
              :class="{ active: cardsTab === 'detail' }"
              @click="cardsTab = 'detail'"
            >
              批次明细
            </button>
          </div>
          <template v-if="cardsTab === 'list'">
          <pre v-if="codes.length" class="code-box">{{ codes.join("\n") }}</pre>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>批次</th>
                  <th>产品 / 代理</th>
                  <th>数量</th>
                  <th>时长</th>
                  <th>导出</th>
                  <th>创建时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="b in a.data.batches">
                  <td>{{ b.name }}</td>
                  <td>
                    {{ pname(b.product_id)
                    }}<small v-if="b.agent_id">
                      · {{ aname(b.agent_id) }}</small
                    >
                  </td>
                  <td>{{ b.quantity }}</td>
                  <td>
                    {{ b.is_permanent ? "永久" : `${b.duration_days} 天` }}
                  </td>
                  <td>{{ b.export_count }}</td>
                  <td>{{ formatTime(b.created_at) }}</td>
                  <td>
                    <button
                      class="btn secondary small"
                      @click="openBatch(b.id)"
                    >
                      明细
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          </template>
          <template v-else>
          <div class="panel-head">
            <h3>批次卡密明细</h3>
            <button
              class="btn secondary small"
              :disabled="!a.selected.batch"
              @click="exportBatch"
            >
              <Download :size="14" />导出当前批次
            </button>
          </div>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>记录</th>
                  <th>状态</th>
                  <th>时长</th>
                  <th>作废原因</th>
                  <th>创建时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="c in a.data.batchCards">
                  <td>
                    <code>{{ shortId(c.id) }}</code>
                  </td>
                  <td><StatusBadge :status="c.status" /></td>
                  <td>
                    {{ c.is_permanent ? "永久" : `${c.duration_days} 天` }}
                  </td>
                  <td>{{ c.void_reason || "-" }}</td>
                  <td>{{ formatTime(c.created_at) }}</td>
                  <td>
                    <button
                      v-if="a.isManager.value && c.status === 'unused'"
                      class="btn danger small"
                      @click="
                        a
                          .mutate(
                            '/admin/cards/void',
                            { card_id: c.id, reason: 'admin_void' },
                            '卡密已作废',
                            false,
                          )
                          .then(() => a.showBatch(a.selected.batch))
                      "
                    >
                      作废
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          </template>
        </section>
        <section v-if="page === 'licenses'" class="panel">
          <div class="panel-head"><h3>授权管理</h3></div>
          <div class="tabbar" role="tablist" aria-label="授权视图">
            <button
              type="button"
              role="tab"
              :aria-selected="licensesTab === 'list'"
              :class="{ active: licensesTab === 'list' }"
              @click="licensesTab = 'list'"
            >
              授权列表
            </button>
            <button
              type="button"
              role="tab"
              :aria-selected="licensesTab === 'bindings'"
              :class="{ active: licensesTab === 'bindings' }"
              @click="licensesTab = 'bindings'"
            >
              绑定明细
            </button>
          </div>
          <template v-if="licensesTab === 'list'">
          <form
            class="form-grid filter-grid"
            @submit.prevent="
              a.data.licensePage.page = 1;
              a.refresh();
            "
          >
            <label class="field"
              >状态<select v-model="a.filters.license.status">
                <option value="">全部</option>
                <option value="active">有效</option>
                <option value="expired">过期</option>
                <option value="revoked">吊销</option>
              </select></label
            ><label class="field"
              >产品<select v-model="a.filters.license.product_id">
                <option value="">全部产品</option>
                <option v-for="p in a.data.products" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            ><label class="field"
              >代理<select v-model="a.filters.license.agent_id">
                <option value="">全部代理</option>
                <option v-for="x in a.data.agents" :value="x.id">
                  {{ x.name }}
                </option>
              </select></label
            ><label class="field"
              >授权号<input v-model="a.filters.license.q"
            /></label>
            <div class="form-actions"><button class="btn">筛选</button></div>
          </form>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>授权号</th>
                  <th>产品</th>
                  <th>状态</th>
                  <th>使用到期</th>
                  <th>更新到期</th>
                  <th>最近验证</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="l in a.data.licenses">
                  <td>
                    <code>{{ l.license_no }}</code>
                  </td>
                  <td>{{ pname(l.product_id) }}</td>
                  <td><StatusBadge :status="l.status" /></td>
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
                  <td>
                    <div class="row-actions">
                      <button
                        class="btn secondary small"
                        @click="openBindings(l.license_no)"
                      >
                        绑定</button
                      ><button
                        v-if="a.isManager.value"
                        class="btn danger small"
                        @click="
                          a.mutate(
                            '/admin/licenses/revoke',
                            { license_no: l.license_no, reason: 'admin' },
                            '授权已吊销',
                          )
                        "
                      >
                        吊销
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="pagination">
            <span
              >第 {{ a.data.licensePage.page }} 页 · 共
              {{ a.data.licensePage.total }} 条</span
            >
            <div>
              <button
                class="icon-btn"
                :disabled="a.data.licensePage.page <= 1"
                @click="changePage(-1)"
              >
                <ChevronLeft :size="17" /></button
              ><button
                class="icon-btn"
                :disabled="
                  a.data.licensePage.page * 20 >= a.data.licensePage.total
                "
                @click="changePage(1)"
              >
                <ChevronRight :size="17" />
              </button>
            </div>
          </div>
          </template>
          <template v-else>
          <div class="panel-head">
            <h3>
              绑定明细<small v-if="a.selected.license" class="binding-license">{{
                a.selected.license
              }}</small>
            </h3>
          </div>
          <div class="detail-list">
            <div v-for="b in a.data.bindings" class="detail-row">
              <span
                ><b>{{ b.bind_mode }} · {{ b.display_name || "-" }}</b
                ><code>{{ b.id }}</code></span
              ><button
                v-if="a.isManager.value"
                class="btn secondary small"
                @click="
                  a
                    .mutate(
                      '/admin/licenses/unbind',
                      {
                        license_no: a.selected.license,
                        binding_id: b.id,
                        reason: 'admin',
                      },
                      '绑定已解绑',
                      false,
                    )
                    .then(() => a.showBindings(a.selected.license))
                "
              >
                解绑
              </button>
            </div>
            </div>
            <p v-if="!a.data.bindings.length" class="empty">
              在授权列表中点击“绑定”查看该授权的绑定明细。
            </p>
          </template>
        </section>
        <section v-if="page === 'tools'" class="panel">
          <div class="panel-head"><h3>激活与验证</h3></div>
          <form class="form-grid" @submit.prevent="tools('activate', $event)">
            <label class="field"
              >产品<select name="product_id">
                <option v-for="p in a.data.products" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            ><label class="field">卡密<input name="card_code" required /></label
            ><label class="field"
              >绑定值<input
                name="bind_value"
                value="machine-A"
                required /></label
            ><label class="field"
              >设备名<input name="device_name" value="Office Mac"
            /></label>
            <div class="form-actions"><button class="btn">激活</button></div>
          </form>
          <form class="form-grid" @submit.prevent="tools('verify', $event)">
            <label class="field"
              >授权号<input name="license_no" required /></label
            ><label class="field"
              >绑定值<input name="bind_value" value="machine-A" required
            /></label>
            <div class="form-actions"><button class="btn">验证</button></div>
          </form>
          <pre class="code-box">{{ result }}</pre>
        </section>
        <section v-if="page === 'offline'" class="panel">
          <div class="panel-head">
            <h3>离线授权</h3>
            <button
              v-if="a.isManager.value"
              class="btn small"
              @click="action = action === 'offline' ? '' : 'offline'"
            >
              <Plus :size="14" />签发
            </button>
          </div>
          <FormDialog
            :open="action === 'offline'"
            title="签发离线授权"
            @close="action = ''"
          >
          <form
            class="form-grid"
            @submit.prevent="
              create(
                '/admin/offline-licenses',
                $event,
                '离线授权已生成',
                (o) => (o.duration_days = Number(o.duration_days)),
              )
            "
          >
            <label class="field"
              >产品<select name="product_id">
                <option v-for="p in a.data.products" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            ><label class="field">客户标识<input name="label" /></label
            ><label class="field wide"
              >机器码<input name="machine_code" required /></label
            ><label class="field"
              >有效天数<input
                name="duration_days"
                type="number"
                value="365" /></label
            ><label class="check"
              ><input name="is_permanent" type="checkbox" />永久授权</label
            >
            <div class="form-actions">
              <button class="btn">生成离线授权</button>
            </div>
          </form>
          </FormDialog>
          <div class="notice">
            纯离线文件无法远程实时收回；吊销后系统将停止再次下载。
          </div>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>授权号</th>
                  <th>产品</th>
                  <th>客户</th>
                  <th>机器码</th>
                  <th>状态</th>
                  <th>到期</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="l in a.data.offline">
                  <td>
                    <code>{{ l.license_no }}</code>
                  </td>
                  <td>{{ pname(l.product_id) }}</td>
                  <td>{{ l.label || "-" }}</td>
                  <td>
                    <code>{{ l.machine_code_masked }}</code>
                  </td>
                  <td><StatusBadge :status="l.status" /></td>
                  <td>
                    {{ l.expired_at ? formatTime(l.expired_at) : "永久" }}
                  </td>
                  <td>
                    <div class="row-actions">
                      <button
                        class="btn secondary small"
                        @click="downloadOffline(l.id)"
                      >
                        下载</button
                      ><button
                        v-if="a.isManager.value && l.status === 'active'"
                        class="btn danger small"
                        @click="
                          a.mutate(
                            `/admin/offline-licenses/${l.id}/revoke`,
                            { reason: 'admin_revoked' },
                            '离线授权已吊销',
                          )
                        "
                      >
                        吊销
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
        <section v-if="page === 'risk'" class="page-stack">
          <div class="metrics">
            <div
              v-for="(v, k) in {
                active_blocks: '生效封禁',
                open_alerts: '待处理告警',
                critical_alerts: '严重告警',
                alerts_24h: '近 24 小时',
              }"
              class="metric"
            >
              <span class="metric-icon"
                ><component
                  :is="{
                    active_blocks: ShieldAlert,
                    open_alerts: BellRing,
                    critical_alerts: Siren,
                    alerts_24h: Clock3,
                  }[k]"
                  :size="16"
              /></span>
              <strong>{{ a.data.riskSummary[k] || 0 }}</strong>
              <small>{{ v }}</small>
            </div>
          </div>
          <section class="panel">
            <div class="panel-head">
              <h3>授权风控中心</h3>
              <button
                v-if="a.isManager.value"
                class="btn small"
                @click="action = action === 'block' ? '' : 'block'"
              >
                <Plus :size="14" />新增封禁
              </button>
            </div>
            <FormDialog
              :open="action === 'block'"
              title="新增封禁规则"
              @close="action = ''"
            >
            <form
              class="form-grid"
              @submit.prevent="
                create('/admin/risk/blocks', $event, '封禁规则已生效')
              "
            >
              <label class="field"
                >类型<select name="kind">
                  <option value="ip">IP 地址</option>
                  <option value="device">设备指纹</option>
                </select></label
              ><label class="field"
                >范围<select name="product_id">
                  <option value="">全局</option>
                  <option v-for="p in a.data.products" :value="p.id">
                    {{ p.name }}
                  </option>
                </select></label
              ><label class="field">目标值<input name="value" required /></label
              ><label class="field">原因<input name="reason" required /></label>
              <div class="form-actions">
                <button class="btn">立即封禁</button>
              </div>
            </form>
            </FormDialog>
            <div class="tabbar" role="tablist" aria-label="风控视图">
              <button
                type="button"
                role="tab"
                :aria-selected="riskTab === 'alerts'"
                :class="{ active: riskTab === 'alerts' }"
                @click="riskTab = 'alerts'"
              >
                实时告警
              </button>
              <button
                type="button"
                role="tab"
                :aria-selected="riskTab === 'blocks'"
                :class="{ active: riskTab === 'blocks' }"
                @click="riskTab = 'blocks'"
              >
                封禁清单
              </button>
            </div>
            <template v-if="riskTab === 'alerts'">
            <div class="panel-head"><h3>异常告警</h3></div>
            <div class="table-wrap">
              <table class="table">
                <thead>
                  <tr>
                    <th>级别</th>
                    <th>告警</th>
                    <th>产品</th>
                    <th>目标</th>
                    <th>详情</th>
                    <th>次数</th>
                    <th>最近发生</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="x in a.data.riskAlerts">
                    <td><StatusBadge :status="x.severity" /></td>
                    <td>{{ x.alert_type }}</td>
                    <td>{{ pname(x.product_id) }}</td>
                    <td>
                      <code>{{ x.subject_masked }}</code>
                    </td>
                    <td>{{ x.detail }}</td>
                    <td>{{ x.occurrence_count }}</td>
                    <td>{{ formatTime(x.last_seen_at) }}</td>
                    <td>
                      <button
                        v-if="a.isManager.value && x.status === 'open'"
                        class="btn secondary small"
                        @click="
                          a.mutate(
                            `/admin/risk/alerts/${x.id}/resolve`,
                            undefined,
                            '告警已解决',
                          )
                        "
                      >
                        <CheckCheck :size="14" />解决
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            </template>
            <template v-else>
            <div class="panel-head"><h3>封禁清单</h3></div>
            <div class="table-wrap">
              <table class="table">
                <thead>
                  <tr>
                    <th>范围</th>
                    <th>类型</th>
                    <th>目标</th>
                    <th>原因</th>
                    <th>状态</th>
                    <th>创建时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="x in a.data.riskBlocks">
                    <td>{{ x.product_id ? pname(x.product_id) : "全局" }}</td>
                    <td>{{ x.kind }}</td>
                    <td>
                      <code>{{ x.value_masked }}</code>
                    </td>
                    <td>{{ x.reason }}</td>
                    <td><StatusBadge :status="x.status" /></td>
                    <td>{{ formatTime(x.created_at) }}</td>
                    <td>
                      <button
                        v-if="a.isManager.value && x.status === 'active'"
                        class="btn danger small"
                        @click="
                          a.mutate(
                            `/admin/risk/blocks/${x.id}/disable`,
                            undefined,
                            '封禁已解除',
                          )
                        "
                      >
                        解除
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            </template>
          </section>
        </section>
        <section v-if="page === 'audit'" class="panel">
          <div class="tabbar" role="tablist" aria-label="审计视图">
            <button
              type="button"
              role="tab"
              :aria-selected="auditTab === 'ops'"
              :class="{ active: auditTab === 'ops' }"
              @click="auditTab = 'ops'"
            >
              操作记录
            </button>
            <button
              type="button"
              role="tab"
              :aria-selected="auditTab === 'logs'"
              :class="{ active: auditTab === 'logs' }"
              @click="auditTab = 'logs'"
            >
              审计日志
            </button>
          </div>
          <OperationLogs
            v-if="auditTab === 'ops'"
            :api="a.api"
            :token="a.token.value"
          />
          <template v-else>
          <div class="panel-head"><h3>审计日志</h3></div>
          <form class="form-grid" @submit.prevent="a.refresh">
            <label class="field"
              >操作者<select v-model="a.filters.audit.actor_type">
                <option value="">全部</option>
                <option value="admin">管理员</option>
                <option value="agent">代理</option>
                <option value="client">客户端</option>
              </select></label
            ><label class="field"
              >结果<select v-model="a.filters.audit.result">
                <option value="">全部</option>
                <option value="success">成功</option>
                <option value="failed">失败</option>
              </select></label
            ><label class="field"
              >操作<input v-model="a.filters.audit.action"
            /></label>
            <div class="form-actions"><button class="btn">筛选</button></div>
          </form>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>时间</th>
                  <th>类型</th>
                  <th>操作</th>
                  <th>结果</th>
                  <th>代理</th>
                  <th>产品</th>
                  <th>授权</th>
                  <th>错误码</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="x in a.data.audit">
                  <td>{{ formatTime(x.created_at) }}</td>
                  <td>{{ x.actor_type }}</td>
                  <td>
                    <code>{{ x.action }}</code>
                  </td>
                  <td><StatusBadge :status="x.result" /></td>
                  <td>{{ shortId(x.agent_id) }}</td>
                  <td>{{ shortId(x.product_id) }}</td>
                  <td>{{ shortId(x.license_id) }}</td>
                  <td>{{ x.error_code || "-" }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          </template>
        </section>
        <section v-if="page === 'admins'" class="panel">
          <div class="panel-head">
            <h3>系统设置</h3>
            <button
              v-if="a.isSuper.value"
              class="btn small"
              @click="action = action === 'admin' ? '' : 'admin'"
            >
              <Plus :size="14" />创建账号
            </button>
          </div>
          <div class="tabbar" role="tablist" aria-label="设置分类">
            <button
              v-for="t in settingTabs"
              :key="t.id"
              type="button"
              role="tab"
              :aria-selected="settingsTab === t.id"
              :class="{ active: settingsTab === t.id }"
              @click="settingsTab = t.id"
            >
              {{ t.label }}
            </button>
          </div>
          <form
            v-if="settingsTab === 'branding'"
            class="branding-settings"
            @submit.prevent="saveSiteSettings"
          >
            <div class="branding-copy">
              <span class="eyebrow"><ImageUp :size="15" /> SITE IDENTITY</span>
              <h4>站点品牌</h4>
              <p>统一应用于授权查询、管理控制台和代理工作台。</p>
            </div>
            <div class="branding-fields">
              <label class="field"
                >网站名称<input
                  v-model="siteForm.site_name"
                  maxlength="80"
                  :disabled="!a.isManager.value"
                  required
              /></label>
              <label class="field"
                >浏览器标题<input
                  v-model="siteForm.browser_title"
                  maxlength="80"
                  :disabled="!a.isManager.value"
                  required
              /></label>
              <div class="asset-field">
                <span>网站 Logo</span>
                <div class="asset-preview">
                  <img
                    :src="siteForm.logo_data_url || '/assets/yunnuo-mark.svg'"
                    alt="Logo 预览"
                  />
                </div>
                <label v-if="a.isManager.value" class="btn secondary small"
                  ><ImageUp :size="14" />上传图片<input
                    type="file"
                    accept="image/png,image/jpeg,image/webp,image/x-icon"
                    @change="readBrandAsset($event, 'logo_data_url')"
                /></label>
                <button
                  v-if="a.isManager.value && siteForm.logo_data_url"
                  class="icon-btn"
                  type="button"
                  title="恢复默认 Logo"
                  @click="siteForm.logo_data_url = ''"
                >
                  <RotateCcw :size="16" />
                </button>
              </div>
              <div class="asset-field">
                <span>网站图标</span>
                <div class="asset-preview favicon">
                  <img
                    :src="
                      siteForm.favicon_data_url || '/assets/yunnuo-mark.svg'
                    "
                    alt="网站图标预览"
                  />
                </div>
                <label v-if="a.isManager.value" class="btn secondary small"
                  ><ImageUp :size="14" />上传图片<input
                    type="file"
                    accept="image/png,image/jpeg,image/webp,image/x-icon"
                    @change="readBrandAsset($event, 'favicon_data_url')"
                /></label>
                <button
                  v-if="a.isManager.value && siteForm.favicon_data_url"
                  class="icon-btn"
                  type="button"
                  title="恢复默认图标"
                  @click="siteForm.favicon_data_url = ''"
                >
                  <RotateCcw :size="16" />
                </button>
              </div>
              <div v-if="a.isManager.value" class="form-actions">
                <button class="btn">保存站点设置</button>
              </div>
            </div>
          </form>
          <section v-if="settingsTab === 'version'" class="version-console">
            <div class="version-hero">
              <span><PackageCheck :size="18" /> 当前运行版本</span>
              <strong>v{{ a.data.system?.current_version || "…" }}</strong>
              <div>
                <code>{{ a.data.system?.channel || "stable" }}</code>
                <span
                  ><GitCommit :size="13" />{{
                    shortId(a.data.system?.commit)
                  }}</span
                >
              </div>
            </div>
            <div class="upgrade-state">
              <History :size="19" />
              <div>
                <strong>授权站自身升级</strong>
                <p>
                  授权站自身升级尚未启用。被授权产品的升级包请在“产品升级”中管理。
                </p>
              </div>
              <StatusBadge
                :status="
                  a.data.system?.capabilities?.online_upgrade
                    ? 'active'
                    : 'disabled'
                "
              />
            </div>
            <div class="release-timeline">
              <article
                v-for="release in a.data.system?.releases || []"
                :key="release.version"
              >
                <div>
                  <strong>v{{ release.version }}</strong
                  ><time>{{ release.date }}</time>
                </div>
                <section>
                  <h4>{{ release.title }}</h4>
                  <ul>
                    <li v-for="item in release.highlights" :key="item">
                      {{ item }}
                    </li>
                  </ul>
                </section>
              </article>
            </div>
          </section>
          <FormDialog
            :open="action === 'admin'"
            title="创建账号"
            description="创建管理员、运营、审计或开发者账号。"
            @close="action = ''"
          >
          <form
            class="form-grid"
            @submit.prevent="create('/admin/users', $event, '账号已创建')"
          >
            <label class="field">登录名<input name="username" required /></label
            ><label class="field"
              >初始密码<input
                name="password"
                type="password"
                minlength="8"
                required /></label
            ><label class="field">显示名<input name="display_name" /></label
            ><label class="field"
              >角色<select name="role">
                <option value="admin">管理员</option>
                <option value="operator">运营人员</option>
                <option value="auditor">审计人员</option>
                <option value="developer">开发者</option>
                <option value="super_admin">超级管理员</option>
              </select></label
            >
            <div class="form-actions">
              <button class="btn">创建账号</button>
            </div>
          </form>
          </FormDialog>
          <template v-if="settingsTab === 'account'">
          <div v-if="a.isSuper.value" class="table-wrap">            <table class="table">
              <thead>
                <tr>
                  <th>账号</th>
                  <th>显示名</th>
                  <th>角色</th>
                  <th>状态</th>
                  <th>最近登录</th>
                  <th>开发者管理</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="u in a.data.users">
                  <td>
                    <code>{{ u.username }}</code>
                  </td>
                  <td>{{ u.display_name || "-" }}</td>
                  <td>{{ roleLabel(u.role) }}</td>
                  <td><StatusBadge :status="u.status" /></td>
                  <td>{{ formatTime(u.last_login_at) }}</td>
                  <td>
                    <button
                      v-if="u.role === 'developer'"
                      class="btn secondary small"
                      @click="
                        a.mutate(
                          `/admin/developers/${u.id}/${u.status === 'active' ? 'disable' : 'enable'}`,
                          {},
                          '开发者状态已更新',
                        )
                      "
                    >
                      {{ u.status === "active" ? "停用" : "启用" }}
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-if="!a.isSuper.value" class="empty">
            账号管理仅超级管理员可见；可在右上角头像菜单修改密码。
          </p>
          </template>
        </section>
      </div>
    </main>
    <FormDialog
      :open="action === 'password'"
      title="修改密码"
      description="修改成功后将退出登录，请使用新密码重新登录。"
      @close="action = ''"
    >
      <form
        class="form-grid"
        @submit.prevent="
          create('/admin/password', $event, '密码已修改').then(() =>
            a.logout('请使用新密码登录'),
          )
        "
      >
        <label class="field"
          >当前密码<input
            name="current_password"
            type="password"
            required /></label
        ><label class="field"
          >新密码<input
            name="new_password"
            type="password"
            minlength="8"
            required
        /></label>
        <div class="form-actions">
          <button class="btn">确认修改</button>
        </div>
      </form>
    </FormDialog>
    <AppToast />
    <dialog :open="!!a.data.keys" class="key-dialog">
      <div class="panel-head">
        <h3>{{ a.data.keys?.product_code }} 公钥</h3>
        <button class="icon-btn" @click="a.data.keys = null">
          <X :size="17" />
        </button>
      </div>
      <div class="key-content">
        <section v-for="k in a.data.keys?.keys">
          <b>v{{ k.key_version }}</b
          ><small>{{ formatTime(k.created_at) }}</small>
          <pre>{{ k.public_key_pem }}</pre>
        </section>
      </div>
    </dialog>
  </div>
</template>
