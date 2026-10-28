<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { downloadText } from "../api";
import {
  formatTime,
  siteDateTimeToUTC,
  licenseStateLabel,
  updatePeriodLabel,
} from "../format";
import FormDialog from "./FormDialog.vue";
import ApplicationIntegration from "./ApplicationIntegration.vue";
import ApplicationReleases from "./ApplicationReleases.vue";
const props = defineProps<{
  api: (path: string, options?: RequestInit) => Promise<any>;
  canWrite: boolean;
  developer?: boolean;
}>();
const applications = ref<any[]>([]),
  selected = ref(""),
  tab = ref("overview");
const app = computed(() =>
  applications.value.find((a) => a.application_id === selected.value),
);
const busy = ref(false),
  error = ref(""),
  notice = ref("");
const data = reactive<Record<string, any>>({
  plans: [],
  licenses: [],
  activations: [],
  cards: [],
  orders: [],
  "binding-rules": [],
  "pending-installations": [],
  "sales-channels": [],
  "agent-grants": [],
  audit: [],
  diagnostics: {},
});
const models = [
  { id: "perpetual", name: "永久" },
  { id: "subscription", name: "订阅" },
  { id: "floating", name: "浮动席位" },
  { id: "metered", name: "按量" },
];
const bindings = [
  { id: "machine_id", name: "机器指纹" },
  { id: "domain", name: "域名" },
  { id: "ip", name: "IP" },
  { id: "seat", name: "浮动席位" },
  { id: "installation", name: "持久安装标识" },
  { id: "account", name: "业务账号" },
  { id: "tenant", name: "租户" },
  { id: "cluster", name: "集群" },
];
const forms = [
  { id: "web", name: "Web / B/S" },
  { id: "container", name: "Docker" },
  { id: "cluster", name: "Kubernetes" },
  { id: "gui", name: "桌面 GUI" },
  { id: "cli", name: "命令行" },
  { id: "service", name: "后台服务" },
  { id: "plugin", name: "插件" },
  { id: "mobile", name: "移动端" },
  { id: "iot", name: "IoT / 设备" },
  { id: "saas", name: "SaaS" },
];
const tabs = [
  { id: "overview", name: "概览" },
  { id: "policy", name: "应用与策略" },
  { id: "plans", name: "套餐" },
  { id: "sales", name: "销售与卡密" },
  { id: "licenses", name: "许可证与安装" },
  { id: "rules", name: "域名 / IP 规则" },
  { id: "releases", name: "在线升级" },
  { id: "agents", name: "代理权限" },
  { id: "integration", name: "接入文档" },
  { id: "audit", name: "审计与诊断" },
];
const createOpen = ref(false),
  onceOpen = ref(false),
  onceTitle = ref(""),
  onceValue = ref("");
const creation = reactive({
  name: "",
  group: "",
  developer_id: "",
  allowed_models: ["perpetual", "subscription"],
  allowed_bindings: ["domain", "ip", "machine_id", "installation"],
  forms: ["web", "container", "gui", "cli", "service"],
});
const policy = reactive<any>({
  block_policy: {
    enabled: true,
    announcement: "站点未授权，请联系软件提供方核实。",
    support_url: "",
  },
});
const developers = ref<any[]>([]),
  groupFilter = ref("");
const appGroups = computed(() =>
  [...new Set(applications.value.map((a) => a.group).filter(Boolean))].sort(),
);
const filteredApps = computed(() =>
  applications.value.filter(
    (a) => !groupFilter.value || a.group === groupFilter.value,
  ),
);
watch(groupFilter, async () => {
  if (!filteredApps.value.some((a) => a.application_id === selected.value)) {
    selected.value = filteredApps.value[0]?.application_id || "";
    await run(selectApp);
  }
});
const assignment = reactive({ developer_id: "", reason: "" });
async function assignDeveloper() {
  await post(`${base()}/assign-developer`, assignment);
  assignment.reason = "";
  await reloadApps();
  fillPolicy();
  notice.value = "开发者归属已保存；原开发者的访问权限已收回。";
}
async function grantGroup() {
  const r = await post("/v2/platform/group-agent-grants", {
    group: app.value.group,
    agent_id: grant.agent_id,
    max_batch_quantity: grant.max_batch_quantity,
    enabled: grant.enabled,
  });
  await refresh();
  notice.value = `分组已处理 ${r.saved} 个套餐。${r.failures.length ? JSON.stringify(r.failures) : "全部成功。"}`;
}

const plan = reactive<any>({
  name: "",
  model: "subscription",
  bindings: ["domain"],
  forms: ["web", "container"],
  perpetual: false,
  duration_days: 365,
  update_duration_days: 365,
  starts_on: "activation",
  max_activations: 1,
  max_binding_groups: 1,
  max_seats: 1,
  max_domain_aliases: 2,
  usage_quota: 1000,
  usage_unit: "request",
  usage_period: "monthly",
  rebind_limit: 0,
  rebind_cooldown_hours: 24,
  features: "",
  price: 0,
  agent_cost: 0,
  renewal_targets: [] as string[],
});
const sale = reactive({
  plan_id: "",
  quantity: 1,
  customer_ref: "",
  owner_user_id: "",
  amount: 0,
  distribute_cards: true,
  confirmed: true,
  purpose: "activate",
  redeem_until: "",
  reason: "",
  custom: false,
  seats: 1,
  duration: 365,
  update_days: 365,
});
const customers = ref<any[]>([]),
  agents = ref<any[]>([]);
const grant = reactive({
  agent_id: "",
  plan_id: "",
  max_batch_quantity: 100,
  enabled: true,
  custom_cost: false,
  cost: 0,
});
async function saveGrant() {
  await post(`${base()}/agent-grants`, {
    agent_id: grant.agent_id,
    plan_id: grant.plan_id,
    max_batch_quantity: grant.max_batch_quantity,
    enabled: grant.enabled,
    override_cost_cents: grant.custom_cost
      ? Math.round(grant.cost * 100)
      : null,
  });
  await refresh();
  notice.value =
    "代理应用与套餐权限已保存。未单独指定成本时按代理等级比例结算。";
}
const rule = reactive({
  kind: "domain",
  pattern: "",
  enabled: true,
  include_apex: false,
  subdomain_depth: "one",
  license_id: "",
  label: "",
});
const channel = reactive({
  name: "",
  enabled: true,
  allowed_plan_ids: [] as string[],
});
const deliveries = ref<any[]>([]);
const op = reactive({
  type: "",
  id: "",
  status: "suspended",
  security_action: "deny_updates",
  reason: "",
  binding_type: "domain",
  binding_value: "",
  aliases: "",
  hours: 24,
  license_id: "",
  user_id: "",
});
const operationTitle = computed(
  () =>
    ({
      status: "修改授权状态",
      security: "人工核实安全处置",
      binding: "预录授权绑定",
      rebind: "更换授权绑定",
      invite: "生成一次性安装邀请",
      deactivate: "停用安装",
      revoke: "吊销卡密",
      confirm: "确认人工收款",
      refund: "订单退款与权益收回",
      remove: "删除白名单规则",
      approved: "批准安装申请",
      rejected: "拒绝安装申请",
      owner: "调整授权归属",
      "rotate-key": "轮换应用签名密钥",
      "reset-secret": "重置 AppSecret",
    })[op.type] || "操作",
);
const filter = reactive({ event_type: "", result: "", reason_code: "" });
const base = () => `/v2/console/applications/${selected.value}`;
const post = (path: string, value: any) =>
  props.api(path, { method: "POST", body: JSON.stringify(value) });
async function run(action: () => Promise<unknown>) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await action();
  } catch (e: any) {
    error.value = e.message || String(e);
  } finally {
    busy.value = false;
  }
}
async function reloadApps() {
  applications.value = await props.api("/v2/console/applications");
  if (!applications.value.some((a) => a.application_id === selected.value))
    selected.value = applications.value[0]?.application_id || "";
}
function fillPolicy() {
  assignment.developer_id = app.value?.developer_id || "";
  if (!app.value) return;
  Object.assign(policy, JSON.parse(JSON.stringify(app.value)));
  policy.block_policy ||= {
    enabled: true,
    announcement: "站点未授权，请联系软件提供方核实。",
    support_url: "",
  };
  policy.jwt_hours = policy.verify_policy.jwt_ttl_seconds / 3600;
  policy.refresh_minutes = policy.verify_policy.verify_interval_seconds / 60;
  policy.grace_days = policy.verify_policy.network_grace_seconds / 86400;
  policy.subscription_grace_days =
    policy.verify_policy.subscription_grace_seconds / 86400;
  policy.borrow_hours = policy.verify_policy.max_borrow_seconds / 3600;
  policy.overlap_days = policy.verify_policy.key_overlap_seconds / 86400;
}
async function refresh() {
  auditOffset.value = 0;
  if (!selected.value) return;
  const collections = Object.keys(data);
  const values = await Promise.all(
    collections.map((c) => props.api(`${base()}/${c}`)),
  );
  collections.forEach((c, i) => (data[c] = values[i]));
  if (!data.plans.some((p: any) => p.id === sale.plan_id))
    sale.plan_id = data.plans.find((p: any) => p.enabled)?.id || "";
  fillPolicy();
}
async function selectApp() {
  deliveries.value = [];
  onceValue.value = "";
  tab.value = "overview";
  await refresh();
}
function showOnce(title: string, value: string) {
  onceTitle.value = title;
  onceValue.value = value;
  onceOpen.value = true;
}
async function create() {
  const result = await post("/v2/console/applications", creation);
  await reloadApps();
  groupFilter.value = "";
  selected.value = result.app_id;
  createOpen.value = false;
  creation.name = "";
  await refresh();
  showOnce(
    "保存应用服务端凭证",
    `AppID: ${result.app_id}\nAppSecret: ${result.app_secret}\n\nAppSecret 仅供你自己控制的服务端使用，不要放入客户发行包。关闭后无法再次查看，请立即保存。`,
  );
}
async function savePolicy() {
  const p = {
    ...policy.verify_policy,
    jwt_ttl_seconds: Math.round(policy.jwt_hours * 3600),
    verify_interval_seconds: Math.round(policy.refresh_minutes * 60),
    network_grace_seconds: Math.round(policy.grace_days * 86400),
    subscription_grace_seconds: Math.round(
      policy.subscription_grace_days * 86400,
    ),
    max_borrow_seconds: Math.round(policy.borrow_hours * 3600),
    key_overlap_seconds: Math.round(policy.overlap_days * 86400),
  };
  await post(base(), {
    name: policy.name,
    group: policy.group,
    allowed_models: policy.allowed_models,
    allowed_bindings: policy.allowed_bindings,
    forms: policy.forms,
    verify_policy: p,
    status: policy.status,
    allow_owner_transfer: policy.allow_owner_transfer,
    updates_enabled: policy.updates_enabled,
    public_query_enabled: policy.public_query_enabled,
    block_policy: policy.block_policy,
  });
  await reloadApps();
  fillPolicy();
  notice.value = "应用策略已保存；服务端下一次校验将执行新策略。";
}
const planBindings = computed(() =>
  bindings.filter((b) => app.value?.allowed_bindings.includes(b.id)),
);
const planForms = computed(() =>
  forms.filter((f) => app.value?.forms.includes(f.id)),
);
watch(
  () => plan.model,
  (model) => {
    if (model === "floating") plan.bindings = ["seat"];
    else if (plan.bindings.includes("seat"))
      plan.bindings = [
        planBindings.value.find((b) => b.id !== "seat")?.id || "domain",
      ];
  },
);
async function createPlan() {
  await post(`${base()}/plans`, {
    name: plan.name,
    terms: {
      model: plan.model,
      renewal_target_plan_ids: plan.renewal_targets,
      binding_types: plan.bindings,
      forms: plan.forms,
      perpetual: plan.model === "perpetual" || plan.perpetual,
      duration_days: plan.duration_days,
      update_duration_days: plan.update_duration_days,
      starts_on: plan.starts_on,
      max_activations: plan.max_activations,
      max_binding_groups: plan.max_binding_groups,
      max_seats: plan.model === "floating" ? plan.max_seats : 0,
      max_domain_aliases: plan.max_domain_aliases,
      usage_quota: plan.usage_quota,
      usage_unit: plan.usage_unit,
      usage_period: plan.usage_period,
      rebind_limit: plan.rebind_limit,
      rebind_cooldown_seconds: plan.rebind_cooldown_hours * 3600,
      features: plan.features
        .split(",")
        .map((v: string) => v.trim())
        .filter(Boolean),
    },
    price_cents: Math.round(plan.price * 100),
    agent_cost_cents: Math.round(plan.agent_cost * 100),
    currency: "CNY",
  });
  plan.name = "";
  await refresh();
  notice.value = "套餐已创建。发售后规格会冻结，后续变更请建立新套餐。";
}
async function createSale() {
  const p = data.plans.find((p: any) => p.id === sale.plan_id);
  const payload: any = {
    plan_id: sale.plan_id,
    quantity: sale.quantity,
    customer_ref: sale.customer_ref,
    owner_user_id: sale.owner_user_id,
    amount_cents: Math.round(sale.amount * 100),
    distribute_cards: sale.distribute_cards,
    confirmed: sale.confirmed,
    purpose: sale.purpose,
    reason: sale.reason,
    redeem_until: sale.redeem_until
      ? siteDateTimeToUTC(sale.redeem_until)
      : null,
  };
  if (sale.custom)
    payload.terms = {
      ...p.terms,
      max_seats: sale.seats,
      duration_days: sale.duration,
      update_duration_days: sale.update_days,
    };
  const result = await post(`${base()}/orders`, payload);
  deliveries.value = result.cards || [];
  await refresh();
  notice.value = sale.confirmed
    ? "销售已确认并签发。卡密只在本次显示，请立即导出。"
    : "订单已建立，确认收款后才会签发许可证。";
}
function exportCards() {
  downloadText(
    [
      "card_id,license_id,code",
      ...deliveries.value.map((c) => `${c.card_id},${c.license_id},${c.code}`),
    ].join("\n"),
    `${selected.value}-cards.csv`,
  );
}
function openOperation(type: string, id = "") {
  Object.assign(op, {
    type,
    id,
    reason: "",
    binding_type: "domain",
    binding_value: "",
    aliases: "",
    license_id: "",
    user_id: "",
    status: "suspended",
    security_action: "deny_updates",
  });
}
async function operate() {
  let path = "",
    payload: any = { reason: op.reason };
  switch (op.type) {
    case "status":
      path = `licenses/${op.id}/status`;
      payload.status = op.status;
      break;
    case "binding":
    case "rebind":
      path = `licenses/${op.id}/${op.type}`;
      payload.binding = {
        type: op.binding_type,
        value: op.binding_value,
        aliases: op.aliases
          .split(/[\n,]/)
          .map((v) => v.trim())
          .filter(Boolean),
      };
      break;
    case "invite":
      path = `licenses/${op.id}/invite`;
      payload = { hours: op.hours };
      break;
    case "owner":
      path = `licenses/${op.id}/owner`;
      payload.user_id = op.user_id;
      break;
    case "security":
      path = `licenses/${op.id}/security`;
      payload.action = op.security_action;
      break;
    case "deactivate":
      path = `activations/${op.id}/deactivate`;
      break;
    case "revoke":
      path = `cards/${op.id}/revoke`;
      break;
    case "refund":
      path = `orders/${op.id}/refund`;
      break;
    case "confirm":
      path = `orders/${op.id}/confirm`;
      break;
    case "remove":
      path = `binding-rules/${op.id}/remove`;
      break;
    case "approved":
    case "rejected":
      path = `pending-installations/${op.id}/${op.type}`;
      payload.license_id = op.license_id;
      break;
    case "rotate-key":
      path = "rotate-key";
      payload.retire_immediately = false;
      break;
    case "reset-secret":
      path = "reset-secret";
      break;
    default:
      return;
  }
  const result = await post(`${base()}/${path}`, payload);
  const type = op.type;
  op.type = "";
  if (type === "invite")
    showOnce(
      "安装邀请（只显示本次）",
      `AppID: ${selected.value}\nInvitation ID: ${result.invitation.id}\nInvitation token: ${result.token}\n到期：${formatTime(result.invitation.expires_at)}\n\n将邀请信息填入产品的激活页面。邀请码仅能关联一份安装公钥。`,
    );
  if (type === "reset-secret")
    showOnce("新 AppSecret（只显示本次）", String(result.app_secret));
  if (result.cards?.length) deliveries.value = result.cards;
  await refresh();
  notice.value = "操作已完成并记入应用审计。";
}
async function saveRule() {
  await post(`${base()}/binding-rules`, rule);
  rule.pattern = "";
  await refresh();
  notice.value = "规则已保存，服务端会在每次校验时重新评估。";
}
async function saveChannel() {
  const result = await post(`${base()}/sales-channels`, channel);
  await refresh();
  showOnce(
    "销售通道签名密钥（只显示本次）",
    `Webhook URL: ${location.origin}/v2/sales/${selected.value}/${result.channel.id}\nSecret: ${result.secret}\n\n签名：HMAC-SHA256(secret, 时间戳 + 换行 + 原始 JSON 请求体)，小写十六进制。\n请求头：X-YN-Timestamp、X-YN-Signature。\n事件须有唯一 event_id；重复事件不重复发货。AppSecret 不用于此签名。`,
  );
}
async function inspectIntegrity() {
  const result = await props.api(`${base()}/audit-integrity`);
  notice.value = `审计链验证通过，共 ${result.sequence} 条事件。`;
}
const auditOffset = ref(0);
async function filterAudit(offset = 0) {
  auditOffset.value = offset;
  data.audit = await props.api(
    `${base()}/audit?${new URLSearchParams({ ...filter, offset: String(auditOffset.value) })}`,
  );
}
const stateName = (v: string) =>
  ({
    active: "有效",
    suspended: "暂停",
    revoked: "吊销",
    expired: "到期",
    pending: "待处理",
    confirmed: "已确认",
    refunded: "已退款",
    unused: "未使用",
    used: "已使用",
    approved: "已批准",
    rejected: "已拒绝",
  })[v] || v;
const modelName = (v: string) => models.find((m) => m.id === v)?.name || v;
onMounted(() =>
  run(async () => {
    await reloadApps();
    await refresh();
    if (!props.developer && props.canWrite) {
      developers.value = await props.api("/v2/platform/developers");
      customers.value = await props.api("/admin/customers");
      agents.value = await props.api("/admin/agents");
    }
  }),
);
</script>
<template>
  <div class="application-workspace">
    <div class="application-heading">
      <div>
        <h2>应用授权中心</h2>
        <p>一个应用、一套策略；相同发行包支持不同客户与部署方式。</p>
      </div>
      <button v-if="canWrite" class="btn" @click="createOpen = true">
        创建应用
      </button>
    </div>
    <p v-if="error" role="alert" class="error">{{ error }}</p>
    <p v-if="notice" role="status" class="notice wrap">{{ notice }}</p>
    <div v-if="applications.length" class="application-picker">
      <label class="field"
        >应用分组<select v-model="groupFilter" aria-label="应用分组筛选">
          <option value="">全部应用</option>
          <option v-for="g in appGroups" :value="g">{{ g }}</option>
        </select></label
      >
      <label class="field"
        >当前应用<select
          v-model="selected"
          @change="run(selectApp)"
          aria-label="当前应用"
        >
          <option
            v-for="a in filteredApps"
            :key="a.application_id"
            :value="a.application_id"
          >
            {{ a.name }} · {{ a.application_id }}
          </option>
        </select></label
      ><button class="btn secondary" :disabled="busy" @click="run(refresh)">
        刷新
      </button>
    </div>
    <section v-else class="panel empty">
      <h3>从创建应用开始</h3>
      <p>
        填写名称并选择支持的授权模型、绑定和产品形态。系统自动生成 AppID
        与独立密钥。
      </p>
    </section>
    <template v-if="app"
      ><nav class="application-tabs" aria-label="应用业务">
        <button
          v-for="t in tabs"
          :key="t.id"
          :class="{ selected: tab === t.id }"
          @click="tab = t.id"
        >
          {{ t.name }}
        </button>
      </nav>
      <section v-if="tab === 'overview'" class="panel">
        <h3>{{ app.name }}</h3>
        <p class="wrap">
          AppID：<code>{{ selected }}</code> · {{ stateName(app.status) }}
        </p>
        <div class="application-stats">
          <article>
            <small>许可证</small
            ><strong>{{ data.diagnostics.license_total || 0 }}</strong>
          </article>
          <article>
            <small>活动安装 / 租约</small
            ><strong>{{ data.diagnostics.active_activations || 0 }}</strong>
          </article>
          <article>
            <small>7 天内到期</small
            ><strong>{{ data.diagnostics.expiring_licenses || 0 }}</strong>
          </article>
        </div>
        <ol class="workflow">
          <li>
            <button @click="tab = 'plans'">建立套餐</button
            ><span>定义期限、绑定、席位与功能。</span>
          </li>
          <li>
            <button @click="tab = 'integration'">接入产品</button
            ><span>复制 AppID，选择语言，使用标准 HTTP 与 JWT。</span>
          </li>
          <li>
            <button @click="tab = 'sales'">销售并签发</button
            ><span>人工确认或验签回调，生成卡密 / 预录授权。</span>
          </li>
          <li>
            <button @click="tab = 'licenses'">激活与运维</button
            ><span>客户输入卡密，或通过一次性邀请 / 审批关联安装。</span>
          </li>
        </ol>
        <p class="muted">
          客户安装不持有
          AppSecret。安装私钥和激活缓存必须持久保存；容器推荐浮动席位。
        </p>
      </section>
      <section v-if="tab === 'policy'" class="panel">
        <h3>应用与验证策略</h3>
        <form
          v-if="!developer && canWrite"
          class="application-form panel"
          @submit.prevent="run(assignDeveloper)"
        >
          <h3>开发者归属</h3>
          <p>
            当前归属：{{
              developers.find((d) => d.id === app.developer_id)?.display_name ||
              (app.developer_id ? app.developer_id : "平台管理")
            }}。分配后开发者仅能访问所属应用。
          </p>
          <label class="field"
            >分配开发者<select
              v-model="assignment.developer_id"
              aria-label="分配开发者"
            >
              <option value="">收回至平台管理</option>
              <option v-for="d in developers" :key="d.id" :value="d.id">
                {{ d.display_name || d.username }} · {{ d.username }}
              </option>
            </select></label
          ><label class="field"
            >分配原因<input
              v-model="assignment.reason"
              required
              maxlength="500" /></label
          ><button class="btn secondary" :disabled="busy">
            保存开发者归属
          </button>
        </form>
        <form class="application-form" @submit.prevent="run(savePolicy)">
          <label class="field"
            >应用名称<input
              v-model="policy.name"
              required
              :disabled="!canWrite" /></label
          ><label class="field"
            >应用分组<input
              v-model="policy.group"
              maxlength="160"
              :disabled="!canWrite"
              list="application-groups" /></label
          ><datalist id="application-groups">
            <option v-for="g in appGroups" :value="g" /></datalist
          ><label class="field"
            >应用状态<select
              v-model="policy.status"
              :disabled="!canWrite"
              aria-label="应用状态"
            >
              <option value="active">启用</option>
              <option value="suspended">暂停</option>
              <option value="archived">归档</option>
            </select></label
          >
          <label class="check"
            ><input
              v-model="policy.updates_enabled"
              type="checkbox"
              :disabled="!canWrite"
            />启用此应用的在线升级服务</label
          ><label class="check"
            ><input
              v-model="policy.public_query_enabled"
              type="checkbox"
              :disabled="!canWrite"
            />允许前台免登录查询授权状态（脱敏）</label
          ><label class="check"
            ><input
              v-model="policy.allow_owner_transfer"
              type="checkbox"
              :disabled="!canWrite"
            />允许管理员变更已有授权的客户归属（转移时停用旧安装与邀请）</label
          >
          <fieldset>
            <legend>允许的授权模型</legend>
            <label v-for="m in models" :key="m.id"
              ><input
                v-model="policy.allowed_models"
                type="checkbox"
                :value="m.id"
                :disabled="!canWrite"
              />{{ m.name }}</label
            >
          </fieldset>
          <fieldset>
            <legend>允许的绑定</legend>
            <label v-for="b in bindings" :key="b.id"
              ><input
                v-model="policy.allowed_bindings"
                type="checkbox"
                :value="b.id"
                :disabled="!canWrite"
              />{{ b.name }}</label
            >
          </fieldset>
          <fieldset>
            <legend>同一应用支持的产品形态</legend>
            <label v-for="f in forms" :key="f.id"
              ><input
                v-model="policy.forms"
                type="checkbox"
                :value="f.id"
                :disabled="!canWrite"
              />{{ f.name }}</label
            >
          </fieldset>
          <div class="application-grid">
            <label class="field"
              >JWT 有效小时数<input
                v-model.number="policy.jwt_hours"
                type="number"
                min="0.0167"
                max="72"
                step="any"
                required
                :disabled="!canWrite" /></label
            ><label class="field"
              >在线刷新间隔（分钟）<input
                v-model.number="policy.refresh_minutes"
                type="number"
                min="0.0167"
                step="any"
                required
                :disabled="!canWrite" /></label
            ><label class="field"
              >断网可用宽限（天，0 禁用）<input
                v-model.number="policy.grace_days"
                type="number"
                min="0"
                max="30"
                step="any"
                required
                :disabled="!canWrite" /></label
            ><label class="field"
              >订阅使用宽限（天，0 严格到期）<input
                v-model.number="policy.subscription_grace_days"
                type="number"
                min="0"
                max="365"
                required
                :disabled="!canWrite" /></label
            ><label class="field"
              >席位租约（秒）<input
                v-model.number="policy.verify_policy.seat_lease_seconds"
                type="number"
                min="30"
                max="3600"
                required
                :disabled="!canWrite" /></label
            ><label class="field"
              >最大离线借用（小时，0 禁用）<input
                v-model.number="policy.borrow_hours"
                type="number"
                min="0"
                max="720"
                step="any"
                required
                :disabled="!canWrite" /></label
            ><label class="field"
              >密钥轮换重叠期（天）<input
                v-model.number="policy.overlap_days"
                type="number"
                min="0"
                max="366"
                step="any"
                required
                :disabled="!canWrite" /></label
            ><label class="field"
              >IP 来源<select
                v-model="policy.verify_policy.ip_source"
                :disabled="!canWrite"
                aria-label="IP 来源"
              >
                <option value="egress">可信请求出口 IP（推荐）</option>
                <option value="declared">
                  已批准安装声明的 IP（内网 / NAT）
                </option>
              </select></label
            ><label class="field"
              >域名规则<select
                v-model="policy.verify_policy.domain_rule_mode"
                :disabled="!canWrite"
                aria-label="域名规则"
              >
                <option value="license_binding">许可证绑定</option>
                <option value="application_allowlist">
                  绑定 + 应用白名单（空表拒绝）
                </option>
              </select></label
            ><label class="field"
              >IP 规则<select
                v-model="policy.verify_policy.ip_rule_mode"
                :disabled="!canWrite"
                aria-label="IP 规则"
              >
                <option value="license_binding">许可证绑定</option>
                <option value="application_allowlist">
                  绑定 + 应用白名单（空表拒绝）
                </option>
              </select></label
            >
          </div>
          <p class="muted">
            使用宽限不延长更新资格。离线令牌有独立有效期，在线明确拒绝后必须清除缓存。
          </p>
          <button v-if="canWrite" class="btn" :disabled="busy">
            保存应用策略
          </button>
        </form>
        <form
          v-if="canWrite"
          class="application-form panel"
          @submit.prevent="run(savePolicy)"
        >
          <h3>盗版拦截公告</h3>
          <p>
            实际产品在授权核验明确拒绝时展示此页；关闭公告样式不会放行无效授权。
          </p>
          <label class="check"
            ><input
              v-model="policy.block_policy.enabled"
              type="checkbox"
            />启用盗版拦截公告样式</label
          ><label class="field"
            >公告内容<textarea
              v-model="policy.block_policy.announcement"
              required
              maxlength="1000"
              rows="4"
              placeholder="站点未授权，联系 QQ531023977"
            ></textarea></label
          ><label class="field"
            >联系链接（可选）<input
              v-model="policy.block_policy.support_url"
              type="url"
              maxlength="1000"
              placeholder="https://example.com/contact"
          /></label>
          <div class="application-actions">
            <button class="btn" :disabled="busy">保存拦截公告</button
            ><a
              class="btn secondary"
              :href="`/v2/applications/${selected}/block-page`"
              target="_blank"
              rel="noopener"
              >预览拦截页</a
            >
          </div>
        </form>
        <div v-if="canWrite" class="application-actions">
          <button class="btn secondary" @click="openOperation('rotate-key')">
            轮换应用签名密钥</button
          ><button class="btn secondary" @click="openOperation('reset-secret')">
            重置 AppSecret
          </button>
        </div>
      </section>
      <section v-if="tab === 'plans'" class="panel">
        <h3>套餐与销售规格</h3>
        <p>
          套餐规格在发售后冻结。永久使用与更新期限分别计算；更新天数为 0
          表示不包含在线更新。
        </p>
        <form
          v-if="canWrite"
          class="application-form"
          @submit.prevent="run(createPlan)"
        >
          <div class="application-grid">
            <label class="field"
              >套餐名称<input v-model="plan.name" required /></label
            ><label class="field"
              >授权模型<select v-model="plan.model" aria-label="授权模型">
                <option
                  v-for="m in models.filter((m) =>
                    app.allowed_models.includes(m.id),
                  )"
                  :key="m.id"
                  :value="m.id"
                >
                  {{ m.name }}
                </option>
              </select></label
            ><label
              v-if="['floating', 'metered'].includes(plan.model)"
              class="check"
              ><input v-model="plan.perpetual" type="checkbox" />永久使用</label
            ><label
              v-if="plan.model !== 'perpetual' && !plan.perpetual"
              class="field"
              >使用天数<input
                v-model.number="plan.duration_days"
                type="number"
                min="1"
                max="36500"
                required /></label
            ><label class="field"
              >更新天数<input
                v-model.number="plan.update_duration_days"
                type="number"
                min="0"
                max="36500"
                required /></label
            ><label class="field"
              >期限从何时起算<select
                v-model="plan.starts_on"
                aria-label="期限从何时起算"
              >
                <option value="activation">首次成功激活</option>
                <option value="issue">签发许可证</option>
              </select></label
            ><label v-if="plan.model === 'floating'" class="field"
              >最大同时在线席位<input
                v-model.number="plan.max_seats"
                type="number"
                min="1"
                max="10000"
                required /></label
            ><template v-else
              ><label class="field"
                >最大活动安装数<input
                  v-model.number="plan.max_activations"
                  type="number"
                  min="1"
                  max="1000"
                  required /></label
              ><label class="field"
                >最大绑定组数<input
                  v-model.number="plan.max_binding_groups"
                  type="number"
                  min="1"
                  max="1000"
                  required /></label></template
            ><label class="field"
              >每组域名别名数<input
                v-model.number="plan.max_domain_aliases"
                type="number"
                min="1"
                max="128"
                required /></label
            ><label class="field"
              >自助换绑次数（0 禁用，-1 不限）<input
                v-model.number="plan.rebind_limit"
                type="number"
                min="-1"
                max="10000"
                required /></label
            ><label class="field"
              >换绑冷却（小时）<input
                v-model.number="plan.rebind_cooldown_hours"
                type="number"
                min="0"
                required /></label
            ><label class="field"
              >售价（元）<input
                v-model.number="plan.price"
                type="number"
                min="0"
                step="0.01"
                required /></label
            ><label class="field"
              >代理基础成本（元）<input
                v-model.number="plan.agent_cost"
                type="number"
                min="0"
                step="0.01"
                required /></label
            ><template v-if="plan.model === 'metered'"
              ><label class="field"
                >计量额度<input
                  v-model.number="plan.usage_quota"
                  type="number"
                  min="1"
                  required /></label
              ><label class="field"
                >计量单位<input v-model="plan.usage_unit" required /></label
              ><label class="field"
                >计量周期<select
                  v-model="plan.usage_period"
                  aria-label="计量周期"
                >
                  <option value="monthly">UTC 自然月</option>
                  <option value="lifetime">整个授权期</option>
                </select></label
              ></template
            ><label class="field"
              >功能标识（英文逗号分隔）<input
                v-model="plan.features"
                placeholder="export, api, reports"
            /></label>
          </div>
          <fieldset>
            <legend>允许的绑定</legend>
            <label v-for="b in planBindings" :key="b.id"
              ><input
                v-model="plan.bindings"
                type="checkbox"
                :value="b.id"
                :disabled="plan.model === 'floating'"
              />{{ b.name }}</label
            >
          </fieldset>
          <fieldset>
            <legend>允许的产品形态</legend>
            <label v-for="f in planForms" :key="f.id"
              ><input v-model="plan.forms" type="checkbox" :value="f.id" />{{
                f.name
              }}</label
            >
          </fieldset>
          <fieldset>
            <legend>可续费的其他套餐（默认只能续同套餐）</legend>
            <label v-for="p in data.plans" :key="p.id"
              ><input
                v-model="plan.renewal_targets"
                type="checkbox"
                :value="p.id"
              />{{ p.name }}</label
            >
            <p class="muted">
              只有明确选中的其他套餐能使用本规格的续费卡，避免低价套餐给高价授权续费。规则随本次签发快照冻结。
            </p>
          </fieldset>
          <button class="btn" :disabled="busy">创建套餐</button>
        </form>
        <div class="application-table">
          <table>
            <thead>
              <tr>
                <th>套餐</th>
                <th>模型</th>
                <th>使用 / 更新</th>
                <th>额度</th>
                <th>绑定</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in data.plans" :key="p.id">
                <td>
                  {{ p.name
                  }}<small>{{ p.enabled ? "可销售" : "已停售" }}</small>
                </td>
                <td>{{ modelName(p.terms.model) }}</td>
                <td>
                  {{
                    p.terms.perpetual ? "永久" : p.terms.duration_days + " 天"
                  }}
                  / {{ p.terms.update_duration_days }} 天
                </td>
                <td>
                  {{
                    p.terms.model === "floating"
                      ? p.terms.max_seats + " 席位"
                      : p.terms.max_activations + " 安装"
                  }}<small v-if="p.terms.model === 'metered'"
                    >{{ p.terms.usage_quota }} {{ p.terms.usage_unit }}</small
                  >
                </td>
                <td>{{ p.terms.binding_types.join(", ") }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section v-if="tab === 'sales'" class="panel">
        <h3>销售、批量发卡与回调</h3>
        <p>
          先建立订单，确认收款时签发。也可以预先签发卡密销售；请在原因中注明。
        </p>
        <form
          v-if="canWrite"
          class="application-form"
          @submit.prevent="run(createSale)"
        >
          <div class="application-grid">
            <label class="field"
              >选择套餐<select
                v-model="sale.plan_id"
                required
                aria-label="选择套餐"
              >
                <option value="" disabled>请选择套餐</option>
                <option
                  v-for="p in data.plans.filter((p: any) => p.enabled)"
                  :key="p.id"
                  :value="p.id"
                >
                  {{ p.name }}
                </option>
              </select></label
            ><label class="field"
              >数量<input
                v-model.number="sale.quantity"
                type="number"
                min="1"
                max="1000"
                required /></label
            ><label class="field"
              >客户参考号<input
                v-model="sale.customer_ref"
                maxlength="190"
                placeholder="订单号、客户合同编号" /></label
            ><label v-if="customers.length" class="field"
              >授权归属客户（可选）<select
                v-model="sale.owner_user_id"
                aria-label="授权归属客户（可选）"
              >
                <option value="">客户之后凭卡密关联</option>
                <option v-for="c in customers" :key="c.id" :value="c.id">
                  {{ c.display_name || c.username }}
                </option>
              </select></label
            ><label class="field"
              >实收金额（元）<input
                v-model.number="sale.amount"
                type="number"
                min="0"
                step="0.01"
                required /></label
            ><label class="field"
              >卡密用途<select v-model="sale.purpose" aria-label="卡密用途">
                <option value="activate">新授权激活</option>
                <option value="renew_usage">仅续使用时间</option>
                <option value="renew_updates">仅续更新时间</option>
              </select></label
            ><label class="field"
              >卡密兑换截止（北京时间，可选）<input
                v-model="sale.redeem_until"
                type="datetime-local" /></label
            ><label class="field"
              >确认 / 备注<input
                v-model="sale.reason"
                required
                placeholder="已人工收款 / 合同交付 / 预制卡密"
            /></label>
          </div>
          <label class="check"
            ><input
              v-model="sale.distribute_cards"
              type="checkbox"
            />生成卡密（关闭后可预录域名/IP，再发送安装邀请）</label
          ><label class="check"
            ><input v-model="sale.confirmed" type="checkbox" />本次已确认收款 /
            允许签发</label
          ><label class="check"
            ><input
              v-model="sale.custom"
              type="checkbox"
            />本次自定义席位或期限（写入本次许可证快照）</label
          >
          <div v-if="sale.custom" class="application-grid">
            <label class="field"
              >席位数<input
                v-model.number="sale.seats"
                type="number"
                min="1" /></label
            ><label class="field"
              >使用天数<input
                v-model.number="sale.duration"
                type="number"
                min="1" /></label
            ><label class="field"
              >更新天数<input
                v-model.number="sale.update_days"
                type="number"
                min="0"
            /></label>
          </div>
          <button class="btn" :disabled="busy || !sale.plan_id">
            建立销售订单
          </button>
        </form>
        <div v-if="deliveries.length" class="once-box">
          <h4>本次生成的卡密</h4>
          <p>
            原文只显示本次。刷新、切换应用或离开页面后无法恢复，请立即保存。
          </p>
          <textarea
            :value="deliveries.map((c) => c.code).join('\n')"
            readonly
            aria-label="本次卡密"
          ></textarea>

          <div class="application-actions">
            <button class="btn" @click="exportCards">导出本次卡密 CSV</button
            ><button class="btn secondary" @click="deliveries = []">
              已保存，隐藏原文
            </button>
          </div>
        </div>
        <h4>销售订单</h4>
        <div class="application-table">
          <table>
            <thead>
              <tr>
                <th>订单 / 客户</th>
                <th>数量 / 金额</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="o in data.orders" :key="o.id">
                <td class="wrap">
                  {{ o.id
                  }}<small>{{ o.customer_ref || "未填写客户参考号" }}</small>
                </td>
                <td>
                  {{ o.quantity }} / ¥{{ (o.amount_cents / 100).toFixed(2) }}
                </td>
                <td>{{ stateName(o.status) }}</td>
                <td>
                  <button
                    v-if="canWrite && o.status === 'pending'"
                    class="btn secondary"
                    @click="openOperation('confirm', o.id)"
                  >
                    确认收款并签发</button
                  ><button
                    v-if="
                      canWrite &&
                      o.status === 'confirmed' &&
                      (!developer || !o.agent_id)
                    "
                    class="btn danger small"
                    @click="openOperation('refund', o.id)"
                  >
                    确认退款
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <h4>卡密记录（仅保留尾号）</h4>
        <div class="application-table">
          <table>
            <thead>
              <tr>
                <th>尾号</th>
                <th>用途</th>
                <th>状态</th>
                <th>许可证</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="c in data.cards" :key="c.id">
                <td>
                  {{
                    c.source === "sales_system"
                      ? "商城分发"
                      : "•••• " + c.last_four
                  }}
                </td>
                <td>{{ c.purpose }}</td>
                <td>{{ stateName(c.status) }}</td>
                <td class="wrap">
                  {{ c.redeemed_license_id || c.bound_license_id }}
                </td>
                <td>
                  <button
                    v-if="canWrite && c.status !== 'revoked'"
                    class="btn secondary"
                    @click="openOperation('revoke', c.id)"
                  >
                    吊销卡密
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <details v-if="canWrite">
          <summary>外部销售系统自动发货</summary>
          <p>
            仅配置你可信的销售服务。回调签名与 AppSecret
            相互独立；支付成功/退款事件在接入文档中提供完整样例。
          </p>
          <form class="application-form" @submit.prevent="run(saveChannel)">
            <label class="field"
              >通道名称<input
                v-model="channel.name"
                required
                placeholder="自建商城"
            /></label>
            <fieldset>
              <legend>允许销售的套餐</legend>
              <label v-for="p in data.plans" :key="p.id"
                ><input
                  v-model="channel.allowed_plan_ids"
                  type="checkbox"
                  :value="p.id"
                />{{ p.name }}</label
              >
            </fieldset>
            <button class="btn" :disabled="busy">创建销售通道</button>
          </form>
          <p v-for="c in data['sales-channels']" :key="c.id" class="wrap">
            {{ c.name }} · {{ c.enabled ? "启用" : "停用" }} · /v2/sales/{{
              selected
            }}/{{ c.id }}
          </p>
        </details>
      </section>
      <section v-if="tab === 'licenses'" class="panel">
        <h3>许可证与安装</h3>
        <p>
          许可证保存销售规格；安装保存公钥与绑定。停用安装会释放在线席位，绑定迁移请使用换绑流程。
        </p>
        <div class="application-table">
          <table>
            <thead>
              <tr>
                <th>许可证</th>
                <th>状态 / 模型</th>
                <th>使用期限</th>
                <th>更新期限</th>
                <th>额度</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="l in data.licenses" :key="l.id">
                <td class="wrap">
                  {{ l.id
                  }}<small>{{ l.customer_ref || "未填写客户参考号" }}</small>
                </td>
                <td>
                  {{ licenseStateLabel(l, app.status) }} /
                  {{ modelName(l.terms.model) }}
                </td>
                <td>
                  {{
                    l.terms.perpetual
                      ? "永久使用"
                      : l.expires_at
                        ? formatTime(l.expires_at)
                        : "首次激活起算"
                  }}
                </td>
                <td>
                  {{ updatePeriodLabel(l, app.updates_enabled) }}
                </td>
                <td>
                  {{
                    l.terms.model === "floating"
                      ? l.terms.max_seats + " 席位"
                      : l.terms.max_activations + " 安装"
                  }}<small v-if="l.terms.model === 'metered'"
                    >已用 {{ l.used_units }} / {{ l.terms.usage_quota }}</small
                  >
                </td>
                <td>
                  <div v-if="canWrite && !l.renewal_only" class="row-actions">
                    <button @click="openOperation('invite', l.id)">
                      安装邀请</button
                    ><button @click="openOperation('binding', l.id)">
                      预录绑定</button
                    ><button @click="openOperation('rebind', l.id)">换绑</button
                    ><button @click="openOperation('security', l.id)">
                      安全处置</button
                    ><button @click="openOperation('status', l.id)">状态</button
                    ><button
                      v-if="!developer"
                      @click="openOperation('owner', l.id)"
                    >
                      归属
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <h4>激活记录与席位租约</h4>
        <div class="application-table">
          <table>
            <thead>
              <tr>
                <th>安装</th>
                <th>绑定</th>
                <th>租约</th>
                <th>最近验证</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="a in data.activations" :key="a.id">
                <td class="wrap">
                  {{ a.label || a.id }}<small>{{ a.license_id }}</small>
                </td>
                <td>
                  {{ a.binding_type
                  }}<small>{{ a.client_binding_hash.slice(0, 16) }}…</small>
                </td>
                <td>
                  {{
                    a.deactivated_at
                      ? "已停用"
                      : a.lease_expires_at
                        ? formatTime(a.lease_expires_at)
                        : "节点锁定"
                  }}<small v-if="a.borrowed">离线借用占位中</small>
                </td>
                <td>{{ formatTime(a.last_verified_at) }}</td>
                <td>
                  <button
                    v-if="canWrite && !a.deactivated_at"
                    class="btn secondary"
                    @click="openOperation('deactivate', a.id)"
                  >
                    停用安装
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <h4>待批准安装</h4>
        <p>
          此入口用于客户环境无法自动证明域名控制权的场景。请核对客户合同、申请编号和公钥摘要后，再关联已购买的许可证。
        </p>
        <article
          v-for="p in data['pending-installations']"
          :key="p.id"
          class="record"
        >
          <p class="wrap">
            {{ p.id }} · {{ p.display }} · {{ stateName(p.status) }}
          </p>
          <small class="wrap">公钥指纹 {{ p.public_key_hash }}</small>
          <div
            v-if="canWrite && p.status === 'pending'"
            class="application-actions"
          >
            <button class="btn" @click="openOperation('approved', p.id)">
              选择许可证并批准</button
            ><button
              class="btn secondary"
              @click="openOperation('rejected', p.id)"
            >
              拒绝
            </button>
          </div>
        </article>
      </section>
      <section v-if="tab === 'rules'" class="panel">
        <h3>域名 / IP 实时规则</h3>
        <p>
          应用白名单是否强制启用，在「应用与策略」中配置。指定许可证的规则也可用于泛域名
          / CIDR
          授权；删除或停用后，下一次在线校验拒绝。通配符不默认包含主域名。
        </p>
        <form
          v-if="canWrite"
          class="application-form"
          @submit.prevent="run(saveRule)"
        >
          <div class="application-grid">
            <label class="field"
              >规则类型<select v-model="rule.kind" aria-label="规则类型">
                <option value="domain">域名 / 泛域名</option>
                <option value="ip">IP / CIDR</option>
              </select></label
            ><label class="field"
              >模式<input
                v-model="rule.pattern"
                :placeholder="
                  rule.kind === 'domain' ? '*.example.com' : '203.0.113.0/24'
                "
                required /></label
            ><label class="field"
              >适用范围<select v-model="rule.license_id" aria-label="适用范围">
                <option value="">整个应用白名单</option>
                <option v-for="l in data.licenses" :key="l.id" :value="l.id">
                  {{ l.customer_ref || l.id }}
                </option>
              </select></label
            ><label class="field">备注<input v-model="rule.label" /></label
            ><label v-if="rule.kind === 'domain'" class="field"
              >子域层级<select
                v-model="rule.subdomain_depth"
                aria-label="子域层级"
              >
                <option value="one">仅一层子域</option>
                <option value="all">任意层子域</option>
              </select></label
            >
          </div>
          <label v-if="rule.kind === 'domain'" class="check"
            ><input
              v-model="rule.include_apex"
              type="checkbox"
            />通配符同时包含主域名</label
          ><button class="btn" :disabled="busy">添加规则</button>
        </form>
        <div class="application-table">
          <table>
            <thead>
              <tr>
                <th>规则</th>
                <th>范围</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in data['binding-rules']" :key="r.id">
                <td>
                  {{ r.pattern
                  }}<small
                    >{{ r.label }} {{ r.include_apex ? "含主域名" : "" }}</small
                  >
                </td>
                <td class="wrap">{{ r.license_id || "整个应用" }}</td>
                <td>{{ r.enabled ? "启用" : "停用" }}</td>
                <td>
                  <button
                    v-if="canWrite"
                    class="btn secondary"
                    @click="openOperation('remove', r.id)"
                  >
                    删除规则
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section v-if="tab === 'agents'" class="panel">
        <h3>代理应用与套餐权限</h3>
        <p>
          代理使用现有用户账号与代理资格。只允许销售被授予的套餐，成本采用代理等级比例；平台管理员可为本应用单独覆盖。
        </p>
        <form
          v-if="canWrite && !developer"
          class="application-form"
          @submit.prevent="run(saveGrant)"
        >
          <div class="application-grid">
            <label class="field"
              >代理<select
                v-model="grant.agent_id"
                required
                aria-label="授权代理"
              >
                <option value="" disabled>选择代理</option>
                <option v-for="a in agents" :key="a.id" :value="a.id">
                  {{ a.name }}
                </option>
              </select></label
            ><label class="field"
              >可销售套餐<select
                v-model="grant.plan_id"
                required
                aria-label="可销售套餐"
              >
                <option value="" disabled>选择本应用套餐</option>
                <option v-for="p in data.plans" :key="p.id" :value="p.id">
                  {{ p.name }}
                </option>
              </select></label
            ><label class="field"
              >单批数量上限<input
                v-model.number="grant.max_batch_quantity"
                type="number"
                min="1"
                max="1000"
                required /></label
            ><label class="check"
              ><input
                v-model="grant.enabled"
                type="checkbox"
              />启用代理权限</label
            ><label class="check"
              ><input
                v-model="grant.custom_cost"
                type="checkbox"
              />单独指定此代理的成本</label
            ><label v-if="grant.custom_cost" class="field"
              >每份成本（元）<input
                v-model.number="grant.cost"
                type="number"
                min="0"
                step="0.01"
                required
            /></label>
          </div>
          <button class="btn" :disabled="busy">保存代理权限</button>
        </form>
        <div v-if="!developer && canWrite && app.group" class="notice">
          <p>
            当前应用属于“{{
              app.group
            }}”。可将该分组当前全部上架套餐批量授予或撤销给已选代理；新增套餐需再次授予，未成功的应用会逐项显示。
          </p>
          <button
            class="btn secondary"
            :disabled="busy || !grant.agent_id"
            @click="run(grantGroup)"
          >
            按当前分组保存代理权限
          </button>
        </div>
        <p v-if="developer" class="muted">
          代理资质、等级与结算成本由平台管理员配置。
        </p>
        <div class="application-table">
          <table>
            <thead>
              <tr>
                <th>代理</th>
                <th>套餐</th>
                <th>状态</th>
                <th>成本</th>
                <th>批量上限</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="g in data['agent-grants']" :key="g.id">
                <td>
                  {{
                    agents.find((a: any) => a.id === g.agent_id)?.name ||
                    g.agent_id
                  }}
                </td>
                <td>
                  {{
                    data.plans.find((p: any) => p.id === g.plan_id)?.name ||
                    g.plan_id
                  }}
                </td>
                <td>{{ g.enabled ? "启用" : "停用" }}</td>
                <td>
                  {{
                    g.override_cost_cents == null
                      ? "代理等级比例"
                      : "¥" + (g.override_cost_cents / 100).toFixed(2)
                  }}
                </td>
                <td>{{ g.max_batch_quantity }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <ApplicationReleases
        v-if="tab === 'releases'"
        :application="app"
        :api="api"
        :can-write="canWrite"
      />
      <ApplicationIntegration
        v-if="tab === 'integration'"
        :application="app"
        :api="api"
      />
      <section v-if="tab === 'audit'" class="panel">
        <h3>应用审计与诊断</h3>
        <div class="application-actions">
          <button
            class="btn secondary"
            :disabled="busy"
            @click="run(inspectIntegrity)"
          >
            验证审计完整性</button
          ><button
            class="btn secondary"
            @click="
              downloadText(
                JSON.stringify(data.audit, null, 2),
                selected + '-audit.json',
              )
            "
          >
            导出当前审计
          </button>
        </div>
        <div class="application-actions">
          <button
            class="btn secondary"
            :disabled="busy || auditOffset === 0"
            @click="run(() => filterAudit(Math.max(0, auditOffset - 500)))"
          >
            上一页审计
          </button>
          <span
            >第 {{ Math.floor(auditOffset / 500) + 1 }} 页，每页最多 500
            条</span
          >
          <button
            class="btn secondary"
            :disabled="busy || data.audit.length < 500"
            @click="run(() => filterAudit(auditOffset + 500))"
          >
            下一页审计
          </button>
        </div>
        <h4>验证失败原因</h4>
        <ul>
          <li
            v-for="(count, reason) in data.diagnostics.verify_failures"
            :key="reason"
          >
            {{ reason }}：{{ count }}
          </li>
        </ul>
        <form
          class="application-grid"
          @submit.prevent="run(() => filterAudit())"
        >
          <label class="field"
            >事件分类<select v-model="filter.event_type" aria-label="事件分类">
              <option value="">全部事件</option>
              <option
                v-for="kind in [
                  ...new Set(data.audit.map((e: any) => e.event_type)),
                ]"
                :key="String(kind)"
                :value="kind"
              >
                {{ kind }}
              </option>
            </select></label
          ><label class="field"
            >结果<select v-model="filter.result" aria-label="结果">
              <option value="">全部</option>
              <option value="success">成功</option>
              <option value="failed">失败</option>
              <option value="pending">待核实 / 申诉</option>
            </select></label
          ><label class="field"
            >原因码<input
              v-model="filter.reason_code"
              placeholder="LICENSE_REVOKED" /></label
          ><button class="btn secondary" :disabled="busy">筛选</button>
        </form>
        <div class="application-table">
          <table>
            <thead>
              <tr>
                <th>序号 / 时间</th>
                <th>事件</th>
                <th>操作者</th>
                <th>结果 / 原因</th>
                <th>详情</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="e in data.audit" :key="e.id">
                <td>
                  #{{ e.sequence }}<small>{{ formatTime(e.created_at) }}</small>
                </td>
                <td>{{ e.event_type }}</td>
                <td>
                  {{ e.actor.kind }}<small>{{ e.actor.id }}</small>
                </td>
                <td>
                  {{ e.result }}<small>{{ e.reason_code }}</small>
                </td>
                <td>
                  <details>
                    <summary>查看</summary>
                    <pre>{{ JSON.stringify(e.payload, null, 2) }}</pre>
                  </details>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
    <FormDialog
      :open="createOpen"
      title="创建应用"
      description="系统生成公开 AppID、一次性 AppSecret 和独立签名密钥。"
      @close="createOpen = false"
      ><form class="application-form" @submit.prevent="run(create)">
        <label class="field"
          >应用名称<input
            v-model="creation.name"
            required
            maxlength="160"
            placeholder="Ebox" /></label
        ><label class="field"
          >应用分组<input v-model="creation.group" maxlength="160" /></label
        ><label v-if="!developer" class="field"
          >所属开发者<select
            v-model="creation.developer_id"
            aria-label="所属开发者"
          >
            <option value="">平台管理</option>
            <option v-for="d in developers" :value="d.id">
              {{ d.display_name || d.username }}
            </option>
          </select></label
        >
        <fieldset>
          <legend>允许的授权模型</legend>
          <label v-for="m in models" :key="m.id"
            ><input
              v-model="creation.allowed_models"
              type="checkbox"
              :value="m.id"
            />{{ m.name }}</label
          >
        </fieldset>
        <fieldset>
          <legend>允许的绑定</legend>
          <label v-for="b in bindings" :key="b.id"
            ><input
              v-model="creation.allowed_bindings"
              type="checkbox"
              :value="b.id"
            />{{ b.name }}</label
          >
        </fieldset>
        <fieldset>
          <legend>产品形态（可多选）</legend>
          <label v-for="f in forms" :key="f.id"
            ><input v-model="creation.forms" type="checkbox" :value="f.id" />{{
              f.name
            }}</label
          >
        </fieldset>
        <p v-if="error" class="error">{{ error }}</p>
        <button class="btn" :disabled="busy">创建并生成凭证</button>
      </form></FormDialog
    >
    <FormDialog
      :open="onceOpen"
      :title="onceTitle"
      @close="
        onceOpen = false;
        onceValue = '';
      "
      ><p>此内容仅在本次操作显示，请及时保存。</p>
      <textarea
        class="credential-output"
        :value="onceValue"
        readonly
        aria-label="一次性凭证"
      ></textarea
      ><button
        class="btn"
        @click="downloadText(onceValue, selected + '-credentials.txt')"
      >
        下载保存
      </button></FormDialog
    >
    <FormDialog :open="!!op.type" :title="operationTitle" @close="op.type = ''"
      ><form class="application-form" @submit.prevent="run(operate)">
        <p v-if="op.type === 'rotate-key'">
          旧公钥会按策略保留重叠期，新令牌使用新 kid。客户端通过 JWKS
          获取新公钥。
        </p>
        <p v-if="op.type === 'reset-secret'">
          旧 AppSecret 将立即失效，请同步更新你控制的销售 /
          服务端配置。客户安装不受影响。
        </p>
        <label v-if="op.type === 'security'" class="field"
          >处置措施<select v-model="op.security_action" aria-label="处置措施">
            <option value="deny_updates">只限制更新，保留使用</option>
            <option value="allow_updates">解除更新限制</option>
            <option value="observe">记录人工核实，不改变授权</option>
          </select></label
        >
        <p v-if="op.type === 'security'">
          需要限制产品运行时，请使用许可证“状态”暂停或吊销。客户上报只作为证据，不能自动判定盗版；所有处置和申诉均写入应用审计。
        </p>
        <p v-if="op.type === 'refund'">
          系统不会自动向外部付款账户转账，请先自行处理人工订单款项。确认后会收回该订单授权，并按剩余有效续费记录重新计算期限；代理订单的原成本只退回平台余额一次。此操作记入审计。
        </p>
        <p v-if="op.type === 'revoke'">
          仅吊销卡密，不会隐式吊销已签发的许可证。如需停用已激活客户，请修改授权状态。
        </p>
        <label v-if="op.type === 'status'" class="field"
          >授权状态<select v-model="op.status" aria-label="授权状态">
            <option value="active">恢复有效（吊销不可恢复）</option>
            <option value="suspended">暂停</option>
            <option value="revoked">永久吊销</option>
            <option value="expired">标记到期</option>
          </select></label
        ><template v-if="['binding', 'rebind'].includes(op.type)"
          ><label class="field"
            >绑定类型<select v-model="op.binding_type" aria-label="绑定类型">
              <option
                v-for="b in planBindings.filter((b) => b.id !== 'seat')"
                :key="b.id"
                :value="b.id"
              >
                {{ b.name }}
              </option>
            </select></label
          ><label class="field"
            >绑定值<input v-model="op.binding_value" required /></label
          ><label v-if="op.binding_type === 'domain'" class="field"
            >同组域名别名（每行一个，须含绑定值）<textarea
              v-model="op.aliases"
              placeholder="example.com&#10;www.example.com"
            ></textarea>
          </label>
          <p v-if="op.type === 'rebind'">
            换绑会停用此许可证的全部旧安装，客户需使用卡密或新邀请重新激活。旧离线令牌仍受其自身有效期限制。
          </p></template
        ><label v-if="op.type === 'invite'" class="field"
          >邀请有效小时数<input
            v-model.number="op.hours"
            type="number"
            min="1"
            max="168"
            required /></label
        ><label v-if="op.type === 'approved'" class="field"
          >关联许可证<select
            v-model="op.license_id"
            required
            aria-label="关联许可证"
          >
            <option value="" disabled>核对客户后选择</option>
            <option
              v-for="l in data.licenses.filter(
                (l: any) => l.status === 'active' && !l.renewal_only,
              )"
              :key="l.id"
              :value="l.id"
            >
              {{ l.customer_ref || l.id }}
            </option>
          </select></label
        ><label v-if="op.type === 'owner'" class="field"
          >客户<select v-model="op.user_id" required aria-label="客户">
            <option value="" disabled>选择客户</option>
            <option v-for="c in customers" :key="c.id" :value="c.id">
              {{ c.display_name || c.username }}
            </option>
          </select></label
        ><label v-if="op.type !== 'invite'" class="field"
          >操作原因<input v-model="op.reason" required maxlength="500"
        /></label>
        <p v-if="error" class="error">{{ error }}</p>
        <button class="btn" :disabled="busy">确认{{ operationTitle }}</button>
      </form></FormDialog
    >
  </div>
</template>
<style scoped>
.application-workspace {
  min-width: 0;
}
.application-heading,
.application-picker {
  display: flex;
  gap: 16px;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  margin-bottom: 20px;
}
.application-heading h2 {
  margin: 0;
}
.application-heading p {
  color: var(--muted);
}
.application-picker .field {
  min-width: 240px;
  flex: 1;
  max-width: 650px;
}
.application-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 18px 0;
}
.application-tabs button {
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 10px 14px;
  background: var(--surface, #fff);
  color: inherit;
  cursor: pointer;
}
.application-tabs .selected {
  background: #087c64;
  color: white;
  border-color: #087c64;
}
.panel {
  padding: 24px;
  margin-bottom: 20px;
}
.application-form {
  display: grid;
  gap: 16px;
  margin: 20px 0;
}
.application-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
  align-items: end;
}
.application-form fieldset {
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 14px;
}
.application-form fieldset label,
.check {
  display: flex;
  align-items: center;
  gap: 7px;
}
.application-form fieldset input,
.check input {
  width: auto;
  min-height: auto;
}
.application-actions,
.row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 12px 0;
}
.row-actions button,
.workflow button {
  background: none;
  border: 0;
  color: var(--green);
  cursor: pointer;
  padding: 4px;
}
.application-table {
  overflow: auto;
  max-width: 100%;
  margin: 20px 0;
}
.application-table table {
  width: 100%;
  border-collapse: collapse;
  min-width: 640px;
}
.application-table td,
.application-table th {
  padding: 13px 10px;
  text-align: left;
  border-bottom: 1px solid var(--line);
  vertical-align: top;
}
.application-table small,
td small {
  display: block;
  color: var(--muted);
  margin-top: 4px;
}
.application-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin: 24px 0;
}
.application-stats article {
  background: var(--surface);
  padding: 18px;
  border-radius: 12px;
}
.application-stats strong {
  display: block;
  font-size: 30px;
  margin-top: 8px;
}
.workflow {
  padding-left: 24px;
  display: grid;
  gap: 15px;
}
.workflow span {
  display: block;
  margin: 5px;
}
.once-box {
  border: 1px solid #ceaa40;
  padding: 20px;
  border-radius: 12px;
  margin: 20px 0;
}
.once-box textarea,
.credential-output {
  width: 100%;
  min-height: 180px;
  font: 13px/1.7 monospace;
  white-space: pre;
  overflow: auto;
}
.credential-output {
  min-height: 280px;
}
.wrap {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
.record {
  padding: 16px 0;
  border-bottom: 1px solid var(--line);
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-width: 600px;
}
summary {
  cursor: pointer;
}
.notice {
  overflow-wrap: anywhere;
}
.empty {
  padding: 50px 25px;
}
@media (max-width: 640px) {
  .panel {
    padding: 16px;
  }
  .application-grid {
    grid-template-columns: 1fr;
  }
  .application-stats {
    gap: 7px;
  }
  .application-stats article {
    padding: 11px;
  }
  .application-stats small {
    font-size: 11px;
  }
  .application-tabs {
    gap: 5px;
  }
  .application-tabs button {
    padding: 9px;
    font-size: 12px;
  }
  .application-picker .field {
    min-width: 0;
    width: 100%;
  }
}
</style>
