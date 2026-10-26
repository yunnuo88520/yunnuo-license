<script setup lang="ts">
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import IntegrationGuide from "../shared/components/IntegrationGuide.vue";
import { computed, nextTick, onMounted, reactive, ref, watch } from "vue";
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
  Mail,
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
  Wallet,
  X,
} from "lucide-vue-next";
import { useAdmin } from "./useAdmin";
import { formatTime, roleLabel, shortId, expiryInfo, onlineStateLabel } from "../shared/format";
import { toast } from "../shared/useToast";
import AppToast from "../shared/components/AppToast.vue";
import StatusBadge from "../shared/components/StatusBadge.vue";
import ProductReleases from "../shared/components/ProductReleases.vue";
import LicensePolicy from "../shared/components/LicensePolicy.vue";
import CustomersPanel from "./CustomersPanel.vue";
import BindingPolicy from "./BindingPolicy.vue";
import OperationLogs from "./OperationLogs.vue";
import CommercePanel from "./CommercePanel.vue";
import AdminMFA from "./AdminMFA.vue";
import PasswordChangeForm from "../shared/components/PasswordChangeForm.vue";
import type { PasswordChangeInput } from "../shared/password";
import OperationsPanel from "./OperationsPanel.vue";
import AgentOnboarding from "./AgentOnboarding.vue";
import RecoveryPanel from "./RecoveryPanel.vue";
import SecurityPanel from "./SecurityPanel.vue";
import FormDialog from "../shared/components/FormDialog.vue";
import ConfirmDialog from "../shared/components/ConfirmDialog.vue";
import CaptchaField from "../shared/components/CaptchaField.vue";
import UserMenu from "../shared/components/UserMenu.vue";
import {
  applyBranding,
  branding,
  loadBranding,
  logoSource,
} from "../shared/branding";
const a = useAdmin();
const agentOnboardingOpen = ref(false);
const initialAgentUserID = ref("");
function openAgentOnboarding(userID = "") {
  initialAgentUserID.value = userID;
  agentOnboardingOpen.value = true;
}
async function agentOpened(result: any) {
  await a.refresh();
  await a.showAgent(result.agent.id);
  go("agents");
  toast("代理资格已开通，可直接使用用户账号登录。");
}
const integrationProductID = ref("");
const createdSecrets = reactive<Record<string, string>>({});
const integrationProduct = computed(() => a.data.products.find((p:any) => p.id === integrationProductID.value));
const integrationAnchor = ref<HTMLElement>();
async function openIntegration(id:string) {
  integrationProductID.value=id;
  await nextTick();
  integrationAnchor.value?.querySelector(".integration-guide")?.scrollIntoView({behavior:"smooth",block:"start"});
}
const page = ref(location.hash.slice(1) || "overview"),
  action = ref("");
const settingsTab = ref("branding");
const settingTabs = [
  { id: "branding", label: "站点品牌" },
  { id: "email", label: "邮件通知" },
  { id: "account", label: "账号与安全" },
  { id: "version", label: "版本与升级" },
];
const visibleSettingTabs = computed(() =>
  settingTabs.filter((t) => t.id !== "email" || a.isManager.value),
);
const agentActionTitles: Record<string, string> = {
  user: "添加团队成员（可选）",
  quota: "发放额度",
  agentBatch: "代理批量发卡",
};
const login = reactive({ username: "", password: "", captcha_id: "", captcha_code: "" });
const captchaField = ref<any>(null);
const mfaMode = ref<"totp" | "recovery">("totp");
const mfaCode = ref("");
const mfaInput = ref<HTMLInputElement>();
async function changeMFAMode(mode: "totp" | "recovery") {
  mfaMode.value = mode;
  mfaCode.value = "";
  a.loginError.value = "";
  await nextTick();
  mfaInput.value?.focus();
}
async function completeAdminSignIn() {
  await a.completeSignIn(mfaCode.value.trim());
  mfaCode.value = "";
  await nextTick();
  mfaInput.value?.focus();
}
const result = ref("{}");
const codes = ref<string[]>([]);
const blockKind = ref("ip");
const toolsProductId = ref("");
watch(
  () => a.data.products,
  (list: any[]) => {
    if (!toolsProductId.value && list?.length) toolsProductId.value = list[0].id;
  },
  { immediate: true, deep: true },
);
const activateBindMode = computed(
  () =>
    a.data.products.find((x: any) => x.id === toolsProductId.value)
      ?.bind_mode || "device",
);
const verifyBindMode = computed(
  () =>
    a.data.products.find((x: any) => x.id === a.selected.product)?.bind_mode ||
    "device",
);
const siteForm = reactive({
  site_name: "允诺云授权",
  browser_title: "允诺云授权",
  logo_data_url: "",
  favicon_data_url: "",
});
const navHome = { id: "overview", label: "仪表盘", icon: LayoutDashboard };
const dashCards = computed(() => {
  const today = new Date().toISOString().slice(0, 10);
  const todayBatches = a.data.batches.filter(
    (b: any) => String(b.created_at || "").slice(0, 10) === today,
  );
  const sum = (arr: any[]) =>
    arr.reduce((s: number, b: any) => s + (b.quantity || 0), 0);
  return [
    {
      label: "产品",
      value: a.data.products.length,
      sub: `${a.data.products.filter((p: any) => p.status === "active").length} 个启用中`,
      icon: Boxes,
      tone: "blue",
    },
    {
      label: "代理",
      value: a.data.agents.length,
      sub: `${a.data.agents.filter((x: any) => x.status === "active").length} 家启用中`,
      icon: UserRoundCog,
      tone: "green",
    },
    {
      label: "卡密批次",
      value: a.data.batches.length,
      sub: `累计发卡 ${sum(a.data.batches)} 张`,
      icon: TicketCheck,
      tone: "purple",
    },
    {
      label: "授权总数",
      value: a.data.licensePage.total,
      sub: `永久卡 ${sum(a.data.batches.filter((b: any) => b.is_permanent))} 张`,
      icon: BadgeCheck,
      tone: "cyan",
    },
    {
      label: "今日发卡",
      value: sum(todayBatches),
      sub: `新增 ${todayBatches.length} 个批次`,
      icon: Clock3,
      tone: "orange",
    },
    {
      label: "未处理风险",
      value: a.data.riskAlerts.filter((x: any) => x.status === "open").length,
      sub: `封禁规则 ${a.data.riskBlocks.length} 条`,
      icon: ShieldAlert,
      tone: "red",
    },
    {
      label: "最近操作",
      value: a.data.audit.length,
      sub: "最近 100 条审计日志",
      icon: History,
      tone: "slate",
    },
    {
      label: "邮件通知",
      value: emailSettings.value
        ? emailSettings.value.enabled
          ? "已启用"
          : "已停用"
        : "未配置",
      sub: emailSettings.value?.enabled
        ? String(emailSettings.value.smtp_host || "SMTP 已就绪")
        : "在系统设置中配置 SMTP",
      icon: Mail,
      tone: "indigo",
    },
    {
      label: "临期未触达",
      value: a.data.stats?.reminders?.missing_email ?? 0,
      sub: `30 天内到期 ${a.data.stats?.reminders?.expiring ?? 0} 项`,
      icon: BellRing,
      tone: "amber",
    },
  ];
});
const cardDonut = computed(() => {
  const cards = a.data.stats?.cards || { total: 0, unused: 0, used: 0, voided: 0 };
  const segments = [
    { label: "未使用", value: cards.unused, color: "#10b981" },
    { label: "已激活", value: cards.used, color: "#6366f1" },
    { label: "已作废", value: cards.voided, color: "#ef4444" },
  ];
  const total = cards.total || 0;
  const circumference = 2 * Math.PI * 52;
  let offset = 0;
  const arcs = segments
    .filter((s: { value: number }) => s.value > 0)
    .map((s) => {
      const frac = total ? s.value / total : 0;
      const arc = { ...s, dash: `${frac * circumference} ${circumference}`, offset: -offset };
      offset += frac * circumference;
      return arc;
    });
  return { total, arcs, segments };
});
const trendChart = computed(() => {
  const points: { date: string; count: number }[] = a.data.stats?.activations || [];
  const width = 560,
    height = 180,
    padX = 26,
    padTop = 16,
    padBottom = 28;
  const max = Math.max(1, ...points.map((p: any) => p.count));
  const stepX = (width - padX * 2) / Math.max(1, points.length - 1);
  const pts = points.map((p: any, i: number) => ({
    date: p.date,
    count: p.count,
    x: padX + i * stepX,
    y: padTop + (1 - p.count / max) * (height - padTop - padBottom),
  }));
  const line = pts.map((p: { x: number; y: number }) => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(" ");
  const base = height - padBottom;
  const area = pts.length
    ? `M${pts[0].x.toFixed(1)},${base} L` +
      pts.map((p) => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(" L") +
      ` L${pts[pts.length - 1].x.toFixed(1)},${base} Z`
    : "";
  return { pts, line, area, max, width, height, padX, padTop, padBottom };
});
const quickActions = [
  {
    label: "生成卡密",
    desc: "创建卡密批次并导出明文",
    icon: TicketCheck,
    tone: "purple",
    run: () => {
      go("cards");
      action.value = "batch";
    },
  },
  {
    label: "新增产品",
    desc: "定义授权模式与绑定策略",
    icon: Boxes,
    tone: "blue",
    run: () => {
      go("products");
      action.value = "product";
    },
  },
  {
    label: "开通代理",
    desc: "创建或选择用户，一次开通",
    icon: UserRoundCog,
    tone: "green",
    run: () => {
      go("agents");
      openAgentOnboarding();
    },
  },
  {
    label: "余额调整",
    desc: "代理预付余额充值或扣减",
    icon: Wallet,
    tone: "orange",
    run: () => go("commerce"),
  },
  {
    label: "邮件通知",
    desc: "配置 SMTP 与代理通知",
    icon: Mail,
    tone: "indigo",
    run: () => {
      go("admins");
      settingsTab.value = "email";
    },
  },
  {
    label: "版本发布",
    desc: "上传升级包并发布给客户",
    icon: PackageCheck,
    tone: "cyan",
    run: () => go("releases"),
  },
];
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
      { id: "operations", label: "运行与容量", icon: Activity },
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
  const result = await create("/admin/products", e, "产品已创建", (o) =>
    [
      "max_bind_count",
      "offline_grace_days",
      "expire_grace_days",
      "update_duration_days",
    ].forEach((k) => (o[k] = Number(o[k]))),
  );
  if (result?.id) {
    createdSecrets[result.id] = result.app_secret;
    await openIntegration(result.id);
  }
}
async function agentTask(kind: string, e: Event) {
  const o = values(e);
  const id = o.agent_id;
  delete o.agent_id;
  if (kind === "user")
    return a.mutate(`/admin/agents/${id}/users`, o, "代理账号已创建");
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
const batchFilter = reactive({ product: "", keyword: "" });
const filteredBatches = computed(() =>
  (a.data.batches as any[]).filter((b) => {
    if (batchFilter.product && b.product_id !== batchFilter.product) return false;
    const kw = batchFilter.keyword.trim().toLowerCase();
    if (kw && !`${b.name} ${b.id}`.toLowerCase().includes(kw)) return false;
    return true;
  }),
);
async function voidBatchConfirm(b: any) {
  const reason = await askConfirm({
    title: "批量作废未使用卡密",
    description: `将作废批次「${b.name}」内全部未使用卡密（已激活卡不受影响），操作不可恢复。`,
    confirmText: "批量作废",
    requireReason: true,
  });
  if (reason == null) return;
  try {
    const r = await a.mutate(`/admin/card-batches/${b.id}/void`, { reason }, "批次未使用卡密已作废", false);
    toast(`已作废 ${r?.voided ?? 0} 张未使用卡密`);
    await a.refresh();
  } catch {}
}
function exportBatchesCSV() {
  const rows = [
    ["批次ID", "批次名", "产品", "数量", "时长", "导出次数", "创建时间"],
    ...filteredBatches.value.map((b: any) => [
      b.id,
      b.name,
      pname(b.product_id),
      String(b.quantity),
      b.is_permanent ? "永久" : `${b.duration_days} 天`,
      String(b.export_count),
      formatTime(b.created_at),
    ]),
  ];
  const csv =
    "\uFEFF" +
    rows
      .map((r) => r.map((c) => `"${String(c).replaceAll('"', '""')}"`).join(","))
      .join("\n");
  const url = URL.createObjectURL(new Blob([csv], { type: "text/csv" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = "card-batches.csv";
  link.click();
  URL.revokeObjectURL(url);
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
const agentEdit = reactive({ id: "", name: "", contact_name: "", phone: "", email: "" });
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
    email: x.email || "",
  });
  action.value = "agentEdit";
}
async function agentUpdate(e: Event) {
  const o = values(e);
  await a.mutate(`/admin/agents/${agentEdit.id}/update`, o, "代理信息已更新");
  action.value = "";
}
const emailSettings = ref<any>(null);
const emailLogs = ref<any[]>([]);
const emailForm = reactive({
  enabled: false,
  smtp_host: "",
  smtp_port: 465,
  encryption: "ssl",
  smtp_username: "",
  password: "",
  from_address: "",
  from_name: "",
  site_url: "",
});
const emailTestTo = ref("");
const emailTesting = ref(false);
const emailTestResult = ref("");
const emailKindLabels: Record<string, string> = {
  test: "测试",
  manual: "手动",
  balance_adjust: "余额",
  quota_grant: "额度",
  account_created: "开通",
  status_change: "状态",
  recovery_code: "恢复码",
  verify_code: "验证码",
};
const myEmail = ref("");
const myEmailSaving = ref(false);
async function saveMyEmail() {
  myEmailSaving.value = true;
  try {
    const u = await a.mutate(
      "/admin/profile/email",
      { email: myEmail.value },
      "我的邮箱已更新",
      false,
    );
    a.profile.value = { ...a.profile.value, email: u.email };
  } catch (e) {
    toast(e);
  } finally {
    myEmailSaving.value = false;
  }
}
const captchaSettings = ref<any>(null);
const captchaSaving = ref(false);
async function loadCaptchaSettings() {
  if (captchaSettings.value) return;
  try {
    captchaSettings.value = await a.api("/admin/captcha-settings");
  } catch (e) {
    toast(e);
  }
}
async function saveCaptchaSettings() {
  captchaSaving.value = true;
  try {
    captchaSettings.value = await a.mutate(
      "/admin/captcha-settings",
      { login_enabled: !captchaSettings.value?.login_enabled },
      captchaSettings.value?.login_enabled ? "图形验证码已关闭" : "图形验证码已开启",
      false,
    );
  } catch (e) {
    toast(e);
  } finally {
    captchaSaving.value = false;
  }
}
function switchSettingsTab(id: string) {
  settingsTab.value = id;
  if (id === "email" && a.isManager.value && !emailSettings.value) {
    loadEmailSettings().catch((e) => toast(e));
  }
  if (id === "account" && a.isManager.value) {
    loadCaptchaSettings();
  }
  if (id === "account") {
    myEmail.value = a.profile.value?.email || "";
  }
}
async function loadEmailSettings() {
  const [settings, logs] = await Promise.all([
    a.api("/admin/email-settings"),
    a.api("/admin/email-logs?limit=20"),
  ]);
  emailSettings.value = settings;
  emailLogs.value = logs || [];
  Object.assign(emailForm, {
    enabled: !!settings?.enabled,
    smtp_host: settings?.smtp_host || "",
    smtp_port: settings?.smtp_port || 465,
    encryption: settings?.encryption || "ssl",
    smtp_username: settings?.smtp_username || "",
    from_address: settings?.from_address || "",
    from_name: settings?.from_name || "",
    site_url: settings?.site_url || "",
  });
}
async function saveEmailSettings() {
  await a.mutate(
    "/admin/email-settings",
    { ...emailForm, smtp_port: Number(emailForm.smtp_port) || 465 },
    "邮件设置已保存",
    false,
  );
  emailForm.password = "";
  await loadEmailSettings();
}
async function sendTestEmail() {
  if (!emailTestTo.value.trim()) return;
  emailTesting.value = true;
  emailTestResult.value = "";
  try {
    await a.api("/admin/email-settings/test", {
      method: "POST",
      body: JSON.stringify({ to_address: emailTestTo.value }),
    });
    emailTestResult.value = "测试邮件已发送，请查收收件箱。";
    emailLogs.value = await a.api("/admin/email-logs?limit=20");
  } catch (e: any) {
    emailTestResult.value = e?.message || String(e);
  } finally {
    emailTesting.value = false;
  }
}
const agentEmailForm = reactive({ id: "", name: "", subject: "", body: "" });
function openAgentEmail() {
  agentEmailForm.id = a.selected.agent;
  agentEmailForm.name = selectedAgentName.value;
  agentEmailForm.subject = "";
  agentEmailForm.body = "";
  action.value = "agentEmail";
}
async function agentEmailSubmit() {
  try {
    const r = await a.api(`/admin/agents/${agentEmailForm.id}/email`, {
      method: "POST",
      body: JSON.stringify({
        subject: agentEmailForm.subject,
        body: agentEmailForm.body,
      }),
    });
    toast(`邮件已发送给 ${r.recipients} 个收件人`);
    action.value = "";
  } catch (e) {
    toast(e);
  }
}
const agentIssue = reactive({ package_id: "", quantity: 5, name: "" });
const agentAccount = ref<any>(null);
const agentIssueQuote = computed(() =>
  agentAccount.value?.packages?.find(
    (q: any) => q.package.id === agentIssue.package_id,
  ),
);
const yuanText = (cents: number) =>
  `¥${((cents || 0) / 100).toFixed(2)}`;
async function openAgentBatch() {
  action.value = "agentBatch";
  agentIssue.quantity = 5;
  agentIssue.name = "";
  await loadAgentAccount();
}
async function loadAgentAccount() {
  agentIssue.package_id = "";
  try {
    agentAccount.value = await a.api(
      `/admin/agents/${a.selected.agent}/account`,
    );
    agentIssue.package_id =
      agentAccount.value?.packages?.[0]?.package?.id || "";
  } catch (e) {
    agentAccount.value = null;
    toast(e);
  }
}
async function agentBatchSubmit() {
  const quote = agentIssueQuote.value;
  if (!quote) return;
  try {
    const r = await a.api(
      `/admin/agents/${a.selected.agent}/card-batches`,
      {
        method: "POST",
        body: JSON.stringify({
          package_id: quote.package.id,
          quantity: Number(agentIssue.quantity),
          name: agentIssue.name.trim(),
          request_id: crypto.randomUUID(),
          expected_unit_cost: quote.unit_cost,
        }),
      },
    );
    toast(`已按代理价格生成 ${r.codes?.length || 0} 张卡密`);
    action.value = "";
    await a.showAgent(a.selected.agent);
  } catch (e) {
    toast(e);
  }
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
            app_key: p.id,
            card_code: o.card_code,
            bind_mode: p.bind_mode,
            bind_value: o.bind_value,
            device_name: o.device_name,
          }
        : {
            app_key: p.id,
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

const confirmState = reactive({
  open: false,
  title: "",
  description: "",
  confirmText: "确认执行",
  requireReason: false,
  reasonLabel: "操作原因",
  reasonPlaceholder: "请填写原因，将记入审计日志",
  resolve: null as ((reason: string | null) => void) | null,
});
function askConfirm(options: {
  title: string;
  description?: string;
  confirmText?: string;
  requireReason?: boolean;
  reasonLabel?: string;
  reasonPlaceholder?: string;
}): Promise<string | null> {
  return new Promise((resolve) => {
    Object.assign(confirmState, {
      open: true,
      title: options.title,
      description: options.description || "",
      confirmText: options.confirmText || "确认执行",
      requireReason: options.requireReason ?? false,
      reasonLabel: options.reasonLabel || "操作原因",
      reasonPlaceholder: options.reasonPlaceholder || "请填写原因，将记入审计日志",
      resolve,
    });
  });
}
function closeConfirm() {
  confirmState.open = false;
  confirmState.resolve?.(null);
  confirmState.resolve = null;
}
function acceptConfirm(reason: string) {
  const resolve = confirmState.resolve;
  confirmState.open = false;
  confirmState.resolve = null;
  resolve?.(reason);
}

async function voidCardConfirm(c: any) {
  const reason = await askConfirm({
    title: "作废卡密",
    description: `卡密 ${shortId(c.id)} 作废后将无法激活，且不可恢复。`,
    confirmText: "作废",
    requireReason: true,
  });
  if (reason == null) return;
  try {
    await a.mutate("/admin/cards/void", { card_id: c.id, reason }, "卡密已作废", false);
    await a.showBatch(a.selected.batch);
  } catch {}
}
async function revokeLicenseConfirm(l: any) {
  const reason = await askConfirm({
    title: "吊销授权",
    description: `授权 ${l.license_no} 吊销后立即失效，全部绑定一并吊销，且无法恢复。`,
    confirmText: "吊销",
    requireReason: true,
  });
  if (reason == null) return;
  try {
    await a.mutate("/admin/licenses/revoke", { license_no: l.license_no, reason }, "授权已吊销");
  } catch {}
}
async function restoreLicenseConfirm(l: any) {
  const reason = await askConfirm({
    title: "恢复授权",
    description: `恢复 ${l.license_no} 为有效授权，吊销时一并吊销的绑定将随之恢复，恢复原因将记入审计日志。`,
    confirmText: "恢复授权",
    requireReason: true,
  });
  if (reason == null) return;
  try {
    await a.mutate("/admin/licenses/restore", { license_no: l.license_no, reason }, "授权已恢复");
  } catch {}
}
async function unbindBindingConfirm(b: any) {
  const reason = await askConfirm({
    title: "解绑绑定",
    description: `解绑该 ${b.bind_mode} 绑定后，对应客户端将无法再通过验证。`,
    confirmText: "解绑",
    requireReason: true,
  });
  if (reason == null) return;
  try {
    await a.mutate(
      "/admin/licenses/unbind",
      { license_no: a.selected.license, binding_id: b.id, reason },
      "绑定已解绑",
      false,
    );
    await a.showBindings(a.selected.license);
  } catch {}
}
async function revokeOfflineConfirm(l: any) {
  const reason = await askConfirm({
    title: "吊销离线授权",
    description: "吊销后系统将停止该离线授权文件的再次下载；已下载到客户本地的文件无法远程收回。",
    confirmText: "吊销",
    requireReason: true,
  });
  if (reason == null) return;
  try {
    await a.mutate(`/admin/offline-licenses/${l.id}/revoke`, { reason }, "离线授权已吊销");
  } catch {}
}
async function disableBlockConfirm(x: any) {
  const ok = await askConfirm({
    title: "解除封禁",
    description: `解除 ${x.kind === "ip" ? "IP" : "设备"} ${x.value_masked} 的封禁后，该对象可重新访问授权接口。`,
    confirmText: "解除封禁",
  });
  if (ok == null) return;
  try {
    await a.mutate(`/admin/risk/blocks/${x.id}/disable`, undefined, "封禁已解除");
  } catch {}
}
async function rotateKeyConfirm(p: any) {
  const ok = await askConfirm({
    title: "轮换产品密钥",
    description: `轮换「${p.name}」的签名密钥后密钥版本将升级：支持密钥环的 SDK 会按 token 的 key_version 自动选择公钥，旧版单公钥客户端需要更新内置公钥后才能验签新 token。`,
    confirmText: "轮换密钥",
  });
  if (ok == null) return;
  try {
    await a.mutate(`/admin/products/${p.id}/keys/rotate`, undefined, "密钥已轮换");
    await a.showKeys(p.id);
  } catch {}
}
async function toggleProductConfirm(p: any) {
  if (p.status !== "active") {
    try {
      await a.mutate(`/admin/products/${p.id}/enable`, undefined, "产品已恢复");
    } catch {}
    return;
  }
  const ok = await askConfirm({
    title: "停用产品",
    description: `停用「${p.name}」后，该产品的激活、验证、心跳、续期接口将全部拒绝请求。`,
    confirmText: "停用",
  });
  if (ok == null) return;
  try {
    await a.mutate(`/admin/products/${p.id}/disable`, undefined, "产品已停用");
  } catch {}
}
async function suspendAgentConfirm(x: any) {
  if (x.status !== "active") {
    try {
      await a.mutate(`/admin/agents/${x.id}/enable`, undefined, "代理状态已更新");
    } catch {}
    return;
  }
  const ok = await askConfirm({
    title: "暂停代理",
    description: `暂停「${x.name || x.id}」后，该代理及其账号将无法登录和发卡。`,
    confirmText: "暂停",
  });
  if (ok == null) return;
  try {
    await a.mutate(`/admin/agents/${x.id}/suspend`, undefined, "代理状态已更新");
  } catch {}
}
async function toggleAccountConfirm(kind: "agent-user" | "developer", u: any) {
  const path =
    kind === "agent-user"
      ? `/admin/agents/${a.selected.agent}/users/${u.id}/${u.status === "active" ? "disable" : "enable"}`
      : `/admin/developers/${u.id}/${u.status === "active" ? "disable" : "enable"}`;
  if (u.status !== "active") {
    try {
      await a.mutate(path, kind === "developer" ? {} : undefined, "状态已更新");
    } catch {}
    return;
  }
  const ok = await askConfirm({
    title: "停用账号",
    description: `停用「${u.username || u.id}」后其登录会话立即失效，无法再访问控制台。`,
    confirmText: "停用",
  });
  if (ok == null) return;
  try {
    await a.mutate(path, kind === "developer" ? {} : undefined, "状态已更新");
  } catch {}
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
async function adminSignIn() {
  login.captcha_id = captchaField.value?.captchaId || "";
  try {
    const outcome = await a.signIn(login);
    if (outcome) login.password = "";
    if (outcome === "mfa") await changeMFAMode("totp");
  } finally {
    login.captcha_code = "";
    if (!a.loginChallenge.value && !a.profile.value) captchaField.value?.refresh();
  }
}
function changeAdminPassword(input: PasswordChangeInput) {
  return a.api<{ changed: boolean }>("/admin/password", { method: "POST", body: JSON.stringify(input) });
}
function adminPasswordChanged() {
  action.value = "";
  // Do not refresh with the token which this successful change just revoked.
  a.logout("密码已修改，请使用新密码重新登录。", "notice");
}
onMounted(async () => {
  loadBranding("管理控制台").catch(() => undefined);
  // 登录后 profile 异步就绪，isManager 变化时再拉取邮件配置供仪表盘展示。
  watch(
    () => a.isManager.value,
    (manager) => {
      if (manager && !emailSettings.value) {
        loadEmailSettings().catch(() => undefined);
      }
    },
    { immediate: true },
  );
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
        <h2>{{ a.loginChallenge.value ? "两步验证" : "管理控制台" }}</h2>
        <p v-if="a.loginNotice.value" class="notice" role="status">{{ a.loginNotice.value }}</p>
        <p v-if="!a.loginChallenge.value">使用管理员账号登录。</p>
        <p v-else-if="mfaMode === 'totp'">打开你绑定的验证器 App，输入当前显示的 6 位验证码。</p>
        <p v-else>输入开启两步验证时保存的备用恢复码，每个只能使用一次。</p>
        <form v-if="!a.loginChallenge.value" class="login-form" @submit.prevent="adminSignIn">
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
          ><CaptchaField ref="captchaField" v-model="login.captcha_code" />
          <button class="btn" :disabled="a.loading.value">
            <LoaderCircle
              v-if="a.loading.value"
              class="spin"
              :size="17"
            /><KeyRound v-else :size="17" />安全登录</button
          ><a href="/recover?from=admin">忘记密码</a>
          <p v-if="a.loginError.value" role="alert" class="error">{{ a.loginError.value }}</p>
        </form>
        <form v-else class="login-form" @submit.prevent="completeAdminSignIn">
          <label v-if="mfaMode === 'totp'" class="field">验证器验证码<input
            ref="mfaInput" v-model.trim="mfaCode" inputmode="numeric"
            autocomplete="one-time-code" minlength="6" maxlength="6" pattern="[0-9]{6}"
            placeholder="6 位数字" required /></label>
          <label v-else class="field">备用恢复码<input
            ref="mfaInput" v-model.trim="mfaCode" autocomplete="off"
            minlength="32" maxlength="32" pattern="[0-9a-fA-F]{32}" required /></label>
          <p v-if="a.loginError.value" role="alert" class="error">{{ a.loginError.value }}</p>
          <button class="btn" :disabled="a.loading.value">
            <LoaderCircle v-if="a.loading.value" class="spin" :size="17" />
            <KeyRound v-else :size="17" />{{ a.loading.value ? "正在验证…" : "验证并登录" }}
          </button>
          <button v-if="mfaMode === 'totp'" type="button" class="mfa-login-link"
            :disabled="a.loading.value" @click="changeMFAMode('recovery')">无法使用验证器？使用备用恢复码</button>
          <button v-else type="button" class="mfa-login-link"
            :disabled="a.loading.value" @click="changeMFAMode('totp')">返回验证器验证</button>
          <button type="button" class="mfa-login-link" :disabled="a.loading.value"
            @click="a.resetLoginStep(); mfaCode = ''">返回账号密码登录</button>
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
          ><div class="page-head">
            <h2>仪表盘</h2>
            <p>系统概览与统计数据</p>
          </div>
          <div class="stat-grid">
            <div v-for="s in dashCards" :key="s.label" class="stat-card">
              <span class="stat-icon" :class="'tone-' + s.tone"
                ><component :is="s.icon" :size="18"
              /></span>
              <span class="stat-body">
                <small>{{ s.label }}</small>
                <strong>{{ s.value }}</strong>
                <small class="stat-sub">{{ s.sub }}</small>
              </span>
            </div>
          </div>
          <section class="panel quick-panel">
            <div class="panel-head"><h3>快捷操作</h3></div>
            <div class="quick-grid">
              <button
                v-for="q in quickActions"
                :key="q.label"
                class="quick-card"
                @click="q.run()"
              >
                <span class="stat-icon" :class="'tone-' + q.tone"
                  ><component :is="q.icon" :size="18"
                /></span>
                <span class="quick-body"
                  ><b>{{ q.label }}</b
                  ><small>{{ q.desc }}</small></span
                ><ChevronRight :size="16" class="quick-arrow" />
              </button>
            </div>
          </section>
          <div class="chart-grid">
            <section class="panel">
              <div class="panel-head"><h3>卡密状态分布</h3></div>
              <div class="donut-wrap">
                <svg viewBox="0 0 140 140" class="donut" role="img" aria-label="卡密状态分布">
                  <circle cx="70" cy="70" r="52" fill="none" stroke="var(--line)" stroke-width="16" />
                  <circle
                    v-for="a in cardDonut.arcs"
                    :key="a.label"
                    cx="70"
                    cy="70"
                    r="52"
                    fill="none"
                    :stroke="a.color"
                    stroke-width="16"
                    :stroke-dasharray="a.dash"
                    :stroke-dashoffset="a.offset"
                    transform="rotate(-90 70 70)"
                  />
                  <text x="70" y="68" text-anchor="middle" class="donut-total">
                    {{ cardDonut.total }}
                  </text>
                  <text x="70" y="86" text-anchor="middle" class="donut-label">
                    张卡密
                  </text>
                </svg>
                <ul class="donut-legend">
                  <li v-for="s in cardDonut.segments" :key="s.label">
                    <span class="legend-dot" :style="{ background: s.color }"></span>
                    <span>{{ s.label }}</span>
                    <strong>{{ s.value }}</strong>
                    <small>{{
                      cardDonut.total
                        ? Math.round((s.value / cardDonut.total) * 100)
                        : 0
                    }}%</small>
                  </li>
                </ul>
              </div>
              <p v-if="!cardDonut.total" class="pane-empty chart-empty">
                暂无卡密数据，生成卡密后展示分布。
              </p>
            </section>
            <section class="panel">
              <div class="panel-head"><h3>近 14 天激活趋势</h3></div>
              <div class="trend-wrap">
                <svg
                  :viewBox="`0 0 ${trendChart.width} ${trendChart.height}`"
                  class="trend"
                  role="img"
                  aria-label="近 14 天激活趋势"
                >
                  <line
                    :x1="trendChart.padX"
                    :y1="trendChart.height - trendChart.padBottom"
                    :x2="trendChart.width - trendChart.padX"
                    :y2="trendChart.height - trendChart.padBottom"
                    class="trend-axis"
                  />
                  <text
                    :x="trendChart.padX - 6"
                    :y="trendChart.padTop + 4"
                    text-anchor="end"
                    class="trend-max"
                  >
                    {{ trendChart.max }}
                  </text>
                  <text
                    :x="trendChart.padX - 6"
                    :y="trendChart.height - trendChart.padBottom + 4"
                    text-anchor="end"
                    class="trend-max"
                  >
                    0
                  </text>
                  <polygon v-if="trendChart.area" :points="trendChart.area" class="trend-area" />
                  <polyline v-if="trendChart.pts.length > 1" :points="trendChart.line" class="trend-line" />
                  <circle
                    v-for="p in trendChart.pts"
                    :key="p.date"
                    :cx="p.x"
                    :cy="p.y"
                    r="3"
                    class="trend-dot"
                  >
                    <title>{{ p.date }}：激活 {{ p.count }} 次</title>
                  </circle>
                  <text
                    v-for="(p, i) in trendChart.pts"
                    :key="'l' + p.date"
                    v-show="i % 2 === 0"
                    :x="p.x"
                    :y="trendChart.height - 8"
                    text-anchor="middle"
                    class="trend-date"
                  >
                    {{ p.date.slice(5) }}
                  </text>
                </svg>
                <p
                  v-if="!trendChart.pts.some((p) => p.count > 0)"
                  class="pane-empty chart-empty"
                >
                  近 14 天暂无激活记录。
                </p>
              </div>
            </section>
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
            description="自动生成 18 位 AppID、独立 AppSecret 与验签公钥，无需填写代码；AppSecret 仅在本次创建后展示，请及时保存。"
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
                min="1"
                value="1"
                required /></label
            ><label class="field"
              >超限策略<select name="bind_conflict_strategy">
                <option value="reject">拒绝</option>
                <option value="kick_oldest">移除最久未活跃</option>
              </select></label
            ><label class="field"
              >离线天数<input
                name="offline_grace_days"
                type="number"
                min="0"
                max="36500"
                value="15"
                required /></label
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
                >离线天数（最长 365）<input
                  name="offline_grace_days"
                  type="number"
                  min="0"
                  max="365"
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
                  <th>应用 ID</th>
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
                  <td>
                    <code>{{ p.id }}</code>
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
                      <button class="btn secondary small" @click="openIntegration(p.id)">接入授权</button>
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
                        @click="rotateKeyConfirm(p)"
                      >
                        轮换</button
                      ><button
                        v-if="a.isManager.value"
                        class="btn danger small"
                        @click="toggleProductConfirm(p)"
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
        <div v-if="page === 'products' && integrationProduct" ref="integrationAnchor">
          <IntegrationGuide :key="integrationProduct.id" :product="integrationProduct" prefix="/admin" :token="a.token.value" :can-manage-credentials="a.isManager.value" :initial-secret="createdSecrets[integrationProduct.id]" @secret-consumed="delete createdSecrets[integrationProduct.id]" />
        </div>
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
        <OperationsPanel v-if="page === 'operations'" :api="a.api" :can-retry="a.isSuper.value" />
        <CustomersPanel
          v-if="page === 'customers'"
          :api="a.api"
          :can-write="a.isManager.value"
          @open-agent="openAgentOnboarding"
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
              @click="openAgentOnboarding()"
            >
              <Plus :size="14" />开通代理账号
            </button>
          </div>
          <FormDialog
            :open="['user', 'quota', 'agentBatch'].includes(action)"
            :title="agentActionTitles[action] || '代理操作'"
            @close="action = ''"
          >
          <form
            class="form-grid"
            @submit.prevent="
              action === 'agentBatch'
                ? agentBatchSubmit()
                : agentTask(action, $event)
            "
          >
            <label class="field">代理<select v-model="a.selected.agent" name="agent_id" required @change="action === 'agentBatch' && loadAgentAccount()"><option v-for="x in a.data.agents" :value="x.id">{{ x.name }} · {{ x.username || x.agent_no }}</option></select></label
            ><template v-if="action === 'user'"
              ><label class="field"
                >登录账号<input name="username" required /></label
              ><label class="field"
                >初始密码<input
                  name="password"
                  type="password"
                  minlength="8"
                  required /></label
              ><label class="field">个人邮箱<input name="email" type="email" placeholder="用于账号找回，不能填写公司共用邮箱" /></label><label class="field">显示名<input name="display_name" /></label
              ><label class="field"
                >角色<select name="role">
                  <option value="agent_owner">负责人</option>
                  <option value="agent_manager">经理</option>
                  <option value="agent_staff" selected>员工</option>
                  <option value="agent_readonly">只读</option>
                </select></label
              ></template
            ><template v-if="action === 'quota'"
              ><label class="field"
                >产品<select name="product_id" required>
                  <option v-for="p in a.data.products" :value="p.id">
                    {{ p.name }}
                  </option>
                </select></label
              ><label class="field"
                >天数<input
                  name="duration_days"
                  type="number"
                  min="1"
                  value="30"
                  required /></label
              ><label class="field"
                >数量<input
                  name="quantity"
                  type="number"
                  min="1"
                  value="10"
                  required /></label
            ></template>
            <template v-if="action === 'agentBatch'"
              ><label class="field"
                >套餐（按代理价格结算）<select v-model="agentIssue.package_id" required>
                  <option
                    v-for="q in agentAccount?.packages || []"
                    :key="q.package.id"
                    :value="q.package.id"
                  >
                    {{ pname(q.package.product_id) }} · {{ q.package.name }}
                    （{{ yuanText(q.unit_cost) }}/张）
                  </option>
                </select></label
              ><label class="field"
                >批次名称<input
                  v-model="agentIssue.name"
                  maxlength="160"
                  placeholder="默认由系统命名" /></label
              ><label class="field"
                >数量<input
                  v-model.number="agentIssue.quantity"
                  type="number"
                  min="1"
                  max="1000"
                  required /></label
              ><p
                v-if="action === 'agentBatch' && agentAccount && !agentAccount.packages?.length"
                class="issue-quote"
              >
                该代理还没有可发的套餐，请在下方“代理授权与资金”中授予套餐。
              </p>
              <p v-if="agentIssueQuote" class="issue-quote">
                单张成本
                <strong>{{ yuanText(agentIssueQuote.unit_cost) }}</strong>
                · 预计合计
                <strong>{{
                  yuanText(agentIssueQuote.unit_cost * (agentIssue.quantity || 0))
                }}</strong>
                · 当前余额
                <strong>{{ yuanText(agentAccount?.account?.balance) }}</strong>
              </p>
            </template>
            <div class="form-actions">
              <button class="btn">确认提交</button>
            </div>
          </form>
          </FormDialog>
          <FormDialog
            :open="action === 'agentEdit'"
            title="编辑代理"
            :description="`更新 ${agentEdit.name} 的业务联系信息。账号登录与密码找回使用用户自己的账号邮箱。`"
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
              <label class="field"
                >邮箱<input
                  name="email"
                  type="email"
                  :value="agentEdit.email"
                  placeholder="业务通知邮箱，不用于重置个人账号密码"
              /></label>
              <div class="form-actions">
                <button class="btn">保存修改</button>
              </div>
            </form>
          </FormDialog>
          <FormDialog
            :open="action === 'agentEmail'"
            title="发送邮件"
            :description="`发送给 ${agentEmailForm.name} 的联系邮箱及其全部启用账号邮箱（自动去重）。`"
            @close="action = ''"
          >
            <form class="form-grid" @submit.prevent="agentEmailSubmit">
              <label class="field"
                >主题<input
                  v-model="agentEmailForm.subject"
                  maxlength="200"
                  required
              /></label>
              <label class="field wide"
                >正文<textarea
                  v-model="agentEmailForm.body"
                  rows="8"
                  maxlength="5000"
                  required
                  placeholder="支持纯文本，换行会保留"
              /></label>
              <div class="form-actions">
                <button class="btn">发送邮件</button>
              </div>
            </form>
          </FormDialog>
          <div class="agent-grid">
            <div class="table-wrap">
              <table class="table">
                <thead>
                  <tr>
                    <th>代理</th>
                    <th>登录账号</th>
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
                      <code>{{ x.username || '待关联用户' }}</code>
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
                          @click="suspendAgentConfirm(x)"
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
                  <button class="btn secondary small" @click="openAgentBatch">
                    批量发卡
                  </button>
                  <button class="btn secondary small" @click="openAgentEmail">
                    发送邮件
                  </button>
                </div>
              </div>
              <template v-if="a.selected.agent">
                <p>用户中心和代理工作台共用账号与密码。代理暂停只限制代理业务，个人授权仍归原用户。</p>
                <details>
                <summary>团队协作（可选，{{ a.data.agentUsers.length }} 位成员）</summary>
                <p>个人代理无需添加成员；企业团队可添加不同权限的成员，代理等级决定价格，成员角色决定操作权限。</p>
                <button v-if="a.isManager.value" class="btn secondary small" @click="action = 'user'">添加团队成员</button>
                <div v-for="u in a.data.agentUsers" class="detail-row">
                  <span
                    ><b>{{ u.display_name || u.username }}</b
                    ><code>{{ u.username }} · {{ u.role }}</code></span
                  ><span class="row-actions"
                    ><StatusBadge :status="u.status" />
                    <button
                      v-if="a.isManager.value"
                      class="btn secondary small"
                      @click="toggleAccountConfirm('agent-user', u)"
                    >
                      {{ u.status === "active" ? "停用" : "启用" }}
                    </button></span
                  >
                </div>
                <p v-if="!a.data.agentUsers.length" class="pane-empty">
                  尚未关联用户，可添加团队负责人。
                </p>
                </details>
                <details>
                <summary>附加张数额度（可选）</summary>
                <p>仅对启用“同时扣除张数额度”的套餐生效。普通余额结算无需另发额度。</p>
                <button v-if="a.isManager.value" class="btn secondary small" @click="action = 'quota'">发放张数额度</button>
                <div v-for="q in a.data.agentQuotas" class="detail-row">
                  <span
                    ><b>{{ pname(q.product_id) }}</b
                    ><code>{{
                      q.is_permanent ? "永久" : `${q.duration_days} 天`
                    }}</code></span
                  ><strong>{{ q.balance }}</strong>
                </div>
                <p v-if="!a.data.agentQuotas.length" class="pane-empty">
                  未配置附加张数额度。
                </p>
                </details>
              </template>
              <p v-else class="pane-empty">
                选择代理，可管理等级、可售套餐、余额与团队。
              </p>
            </aside>
          </div>
          <CommercePanel
            v-if="a.selected.agent"
            :key="a.selected.agent"
            :agent-id="a.selected.agent"
            :token="a.token.value"
            :api="a.api"
            :products="a.data.products"
            :agents="a.data.agents"
            :can-write="a.isManager.value"
          />
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
              >产品<select name="product_id" required>
                <option v-for="p in a.data.products" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            ><label class="field"
              >批次名称<input name="name" value="新批次" maxlength="160" /></label
            ><label class="field"
              >数量<input
                name="quantity"
                type="number"
                min="1"
                max="1000"
                value="5"
                required /></label
            ><label class="field"
              >天数<input
                name="duration_days"
                type="number"
                min="1"
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
          <div class="batch-filter">
            <label class="field"
              >产品筛选<select v-model="batchFilter.product">
                <option value="">全部产品</option>
                <option v-for="p in a.data.products" :key="p.id" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            >
            <label class="field"
              >搜索批次<input
                v-model="batchFilter.keyword"
                placeholder="批次名称或 ID"
                autocomplete="off"
            /></label>
            <button class="btn secondary small" @click="exportBatchesCSV">
              <Download :size="14" />导出批次 CSV
            </button>
          </div>
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
                <tr v-for="b in filteredBatches">
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
                    <div class="row-actions">
                      <button
                        class="btn secondary small"
                        @click="openBatch(b.id)"
                      >
                        明细</button
                      ><button
                        v-if="a.isManager.value"
                        class="btn danger small"
                        @click="voidBatchConfirm(b)"
                      >
                        批量作废
                      </button>
                    </div>
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
                      @click="voidCardConfirm(c)"
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
                    <span
                      v-if="l.expired_at"
                      :class="['expiry-tag', `expiry-tag-${expiryInfo(l.expired_at).level}`]"
                      >{{ expiryInfo(l.expired_at).label }}</span
                    >
                  </td>
                  <td>
                    {{
                      l.updates_expired_at
                        ? formatTime(l.updates_expired_at)
                        : "永久更新"
                    }}
                    <span
                      v-if="l.updates_expired_at"
                      :class="['expiry-tag', `expiry-tag-${expiryInfo(l.updates_expired_at).level}`]"
                      >{{ expiryInfo(l.updates_expired_at).label }}</span
                    >
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
                        v-if="a.isManager.value && l.status !== 'revoked'"
                        class="btn danger small"
                        @click="revokeLicenseConfirm(l)"
                      >
                        吊销</button
                      ><button
                        v-if="a.isManager.value && l.status === 'revoked'"
                        class="btn secondary small"
                        @click="restoreLicenseConfirm(l)"
                      >
                        恢复
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
                ><code>{{ b.id }}</code
                ><span
                  v-if="b.online_state"
                  :class="['heartbeat-tag', `heartbeat-${b.online_state}`]"
                  >{{ onlineStateLabel(b.online_state, b.offline_days) }}</span
                ></span
              ><button
                v-if="a.isManager.value"
                class="btn secondary small"
                @click="unbindBindingConfirm(b)"
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
              >产品<select v-model="toolsProductId" name="product_id" required>
                <option v-for="p in a.data.products" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            ><label class="field">卡密<input name="card_code" required /></label
            ><label class="field"
              >绑定值<input
                name="bind_value"
                value="machine-A"
                :required="activateBindMode !== 'none'" /></label
            ><label class="field"
              >设备名<input name="device_name" value="Office Mac"
            /></label>
            <div class="form-actions"><button class="btn">激活</button></div>
          </form>
          <form class="form-grid" @submit.prevent="tools('verify', $event)">
            <label class="field"
              >授权号<input name="license_no" required /></label
            ><label class="field"
              >绑定值<input
                name="bind_value"
                value="machine-A"
                :required="verifyBindMode !== 'none'"
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
              >产品<select name="product_id" required>
                <option v-for="p in a.data.products" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            ><label class="field"
              >客户标识<input name="label" maxlength="100" /></label
            ><label class="field wide"
              >机器码<input name="machine_code" required maxlength="256" /></label
            ><label class="field"
              >有效天数（最长 365，更长周期请分次签发）<input
                name="duration_days"
                type="number"
                min="1"
                max="365"
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
            纯离线文件无法远程实时收回；离线授权最长 365 天，到期后重新签发可让吊销与到期策略生效。
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
                        @click="revokeOfflineConfirm(l)"
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
                >类型<select v-model="blockKind" name="kind">
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
              ><label class="field"
                >目标值<input
                  name="value"
                  required
                  maxlength="256"
                  :pattern="blockKind === 'ip' ? '[0-9a-fA-F:.]{3,45}' : '.{1,256}'"
                  :title="
                    blockKind === 'ip'
                      ? '填写有效的 IPv4/IPv6 地址'
                      : '填写设备指纹'
                  " /></label
              ><label class="field"
                >原因<input name="reason" required maxlength="500" /></label>
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
                        @click="disableBlockConfirm(x)"
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
              v-for="t in visibleSettingTabs"
              :key="t.id"
              type="button"
              role="tab"
              :aria-selected="settingsTab === t.id"
              :class="{ active: settingsTab === t.id }"
              @click="switchSettingsTab(t.id)"
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
          <div
            v-if="settingsTab === 'email' && a.isManager.value"
            class="email-settings"
          >
            <div class="email-main">
              <div class="branding-copy">
                <span class="eyebrow"><Mail :size="15" /> EMAIL / SMTP</span>
                <h4>邮件通知</h4>
                <p>
                  配置 SMTP 后可向代理发送通知与验证码邮件；未启用时自动通知静默跳过，不影响业务。
                </p>
              </div>
              <form class="branding-fields" @submit.prevent="saveEmailSettings">
                <label class="check"
                  ><input v-model="emailForm.enabled" type="checkbox" />启用邮件通知（余额、额度、账号开通、状态变更自动通知）</label
                >
                <label class="field"
                  >SMTP 服务器<input
                    v-model="emailForm.smtp_host"
                    placeholder="smtp.example.com"
                    :required="emailForm.enabled"
                /></label>
                <label class="field"
                  >端口<input
                    v-model.number="emailForm.smtp_port"
                    type="number"
                    min="1"
                    max="65535"
                    required
                /></label>
                <label class="field"
                  >加密方式<select v-model="emailForm.encryption">
                    <option value="ssl">SSL/TLS（465 端口常用）</option>
                    <option value="starttls">STARTTLS（587 端口常用）</option>
                    <option value="none">无加密</option>
                  </select></label
                >
                <label class="field"
                  >SMTP 用户名<input
                    v-model="emailForm.smtp_username"
                    autocomplete="off"
                    placeholder="留空默认使用发件人地址"
                /></label>
                <label class="field"
                  >SMTP 密码<input
                    v-model="emailForm.password"
                    type="password"
                    autocomplete="new-password"
                    :placeholder="
                      emailSettings?.has_password
                        ? '已保存，留空保持不变'
                        : '未设置'
                    "
                /></label>
                <label class="field"
                  >发件人地址<input
                    v-model="emailForm.from_address"
                    type="email"
                    placeholder="noreply@example.com"
                    :required="emailForm.enabled"
                /></label>
                <label class="field"
                  >发件人名称<input
                    v-model="emailForm.from_name"
                    placeholder="显示在邮件发件人位置"
                /></label>
                <label class="field wide"
                  >站点地址<input
                    v-model="emailForm.site_url"
                    placeholder="https://your-site.com（邮件内登录与恢复链接使用，选填）"
                /></label>
                <div class="form-actions">
                  <button class="btn">保存邮件设置</button>
                </div>
              </form>
            </div>
            <div class="email-test">
              <h5>测试发送</h5>
              <form class="email-test-row" @submit.prevent="sendTestEmail">
                <input
                  v-model="emailTestTo"
                  type="email"
                  placeholder="填写一个能收信的邮箱地址"
                  required
                />
                <button class="btn secondary" :disabled="emailTesting">
                  {{ emailTesting ? "发送中…" : "发送测试邮件" }}
                </button>
              </form>
              <p v-if="emailTestResult" role="status" class="email-test-result">
                {{ emailTestResult }}
              </p>
            </div>
            <div class="email-logs">
              <h5>最近发送记录</h5>
              <div v-if="emailLogs.length" class="table-wrap">
                <table class="table">
                  <thead>
                    <tr>
                      <th>类型</th>
                      <th>收件人</th>
                      <th>主题</th>
                      <th>状态</th>
                      <th>时间</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="log in emailLogs" :key="log.id">
                      <td>{{ emailKindLabels[log.kind] || log.kind }}</td>
                      <td><code>{{ log.to_address }}</code></td>
                      <td>{{ log.subject }}</td>
                      <td>
                        <StatusBadge :status="log.status" />
                        <small v-if="log.error" class="email-error">
                          {{ log.error }}</small
                        >
                      </td>
                      <td>{{ formatTime(log.created_at) }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <p v-else class="pane-empty">暂无发送记录。</p>
            </div>
          </div>
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
            <label class="field">登录账号<input name="username" required /></label
            ><label class="field"
              >初始密码<input
                name="password"
                type="password"
                minlength="8"
                required /></label
            ><label class="field">个人邮箱<input name="email" type="email" placeholder="用于账号找回，不能填写公司共用邮箱" /></label><label class="field">显示名<input name="display_name" /></label
            ><label class="field"
              >邮箱<input
                name="email"
                type="email"
                maxlength="254"
                placeholder="用于找回密码与通知，建议填写" /></label
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
 <AdminMFA :api="a.api" @signed-out="a.logout('安全设置已变更，请重新登录')" />
          <div class="captcha-setting">
            <div class="captcha-setting-copy">
              <h4>我的邮箱</h4>
              <p>
                用于自助找回密码与接收通知（{{ a.profile.value?.username }}）。邮箱在同类账号中唯一，修改后立即生效。
              </p>
            </div>
            <form class="my-email-row" @submit.prevent="saveMyEmail">
              <input
                v-model="myEmail"
                type="email"
                maxlength="254"
                placeholder="you@example.com"
              />
              <button class="btn" :disabled="myEmailSaving">
                {{ myEmailSaving ? "保存中…" : "保存邮箱" }}
              </button>
            </form>
          </div>
          <div v-if="a.isManager.value" class="captcha-setting">
            <div class="captcha-setting-copy">
              <h4>登录图形验证码</h4>
              <p>
                开启后，管理、代理、用户与开发者控制台的登录页都会要求输入图形验证码。验证码 5
                分钟有效且只能使用一次，用于防御暴力尝试登录。
              </p>
            </div>
            <button
              class="btn"
              :class="{ danger: captchaSettings?.login_enabled }"
              :disabled="captchaSaving"
              @click="saveCaptchaSettings"
            >
              {{ captchaSaving ? "处理中…" : captchaSettings?.login_enabled ? "关闭验证码" : "开启验证码" }}
            </button>
          </div>
          <div v-if="a.isSuper.value" class="table-wrap">            <table class="table">
              <thead>
                <tr>
                  <th>账号</th>
                  <th>邮箱</th>
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
                  <td><code>{{ u.email || "-" }}</code></td>
                  <td>{{ u.display_name || "-" }}</td>
                  <td>{{ roleLabel(u.role) }}</td>
                  <td><StatusBadge :status="u.status" /></td>
                  <td>{{ formatTime(u.last_login_at) }}</td>
                  <td>
                    <button
                      v-if="u.role === 'developer'"
                      class="btn secondary small"
                      @click="toggleAccountConfirm('developer', u)"
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
      @close="action = ''"
    >
      <PasswordChangeForm :change="changeAdminPassword" submit-label="确认修改" @changed="adminPasswordChanged" />
    </FormDialog>
    <ConfirmDialog
      :open="confirmState.open"
      :title="confirmState.title"
      :description="confirmState.description"
      :confirm-text="confirmState.confirmText"
      :require-reason="confirmState.requireReason"
      :reason-label="confirmState.reasonLabel"
      :reason-placeholder="confirmState.reasonPlaceholder"
      @close="closeConfirm"
      @confirm="acceptConfirm"
    />
    <AgentOnboarding :open="agentOnboardingOpen" :initial-user-id="initialAgentUserID" :products="a.data.products" :api="a.api" @close="agentOnboardingOpen = false" @opened="agentOpened" />
    <AppToast />
    <dialog :open="!!a.data.keys" class="key-dialog">
      <div class="panel-head">
        <h3>{{ pname(a.data.keys?.product_id) }} 公钥</h3>
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
