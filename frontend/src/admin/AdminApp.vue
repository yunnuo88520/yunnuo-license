<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import {
  Activity,
  BadgeCheck,
  Boxes,
  CheckCheck,
  ChevronLeft,
  ChevronRight,
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
import {
  applyBranding,
  branding,
  loadBranding,
  logoSource,
} from "../shared/branding";
const a = useAdmin();
const page = ref(location.hash.slice(1) || "overview"),
  action = ref("");
const login = reactive({ username: "", password: "" });
const result = ref("{}");
const codes = ref<string[]>([]);
const siteForm = reactive({
  site_name: "允诺云授权",
  browser_title: "允诺云授权",
  logo_data_url: "",
  favicon_data_url: "",
});
const nav = [
  { id: "overview", label: "业务概览", icon: LayoutDashboard },
  { id: "products", label: "产品", icon: Boxes },
  { id: "agents", label: "代理", icon: UserRoundCog },
  { id: "cards", label: "卡密", icon: TicketCheck },
  { id: "licenses", label: "授权", icon: BadgeCheck },
  { id: "risk", label: "风控", icon: ShieldAlert },
  { id: "tools", label: "联调工具", icon: Terminal },
  { id: "offline", label: "离线授权", icon: FileKey2 },
  { id: "audit", label: "审计", icon: ScrollText },
  { id: "admins", label: "系统", icon: Settings2 },
];
const title = computed(
  () => nav.find((n) => n.id === page.value)?.label || "业务概览",
);
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
    ["max_bind_count", "offline_grace_days", "expire_grace_days"].forEach(
      (k) => (o[k] = Number(o[k])),
    ),
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
  if (!["image/png", "image/jpeg", "image/webp", "image/x-icon", "image/vnd.microsoft.icon"].includes(file.type))
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
          <div><strong>{{ branding.site_name }}</strong><span>ADMIN CONSOLE</span></div>
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
            /><KeyRound v-else :size="17" />安全登录
          </button>
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
      <nav class="nav">
        <a
          v-for="n in nav"
          :key="n.id"
          href="#"
          :class="{ active: page === n.id }"
          :aria-label="n.label"
          :title="n.label"
          @click.prevent="go(n.id)"
          ><component :is="n.icon" :size="18" /><span>{{ n.label }}</span></a
        >
      </nav>
      <div class="sidebar-foot">
        SECURE NODE / ONLINE<br />V{{
          a.data.system?.current_version || "0.2.0"
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
          <span class="identity"
            >{{ a.profile.value.display_name || a.profile.value.username }} ·
            {{ roleLabel(a.profile.value.role) }}</span
          ><button class="icon-btn" title="刷新" @click="a.refresh">
            <RefreshCw :size="17" /></button
          ><button class="icon-btn" title="退出" @click="a.logout()">
            <LogOut :size="17" />
          </button>
        </div>
      </header>
      <div class="workspace">
        <template v-if="page === 'overview'"
          ><section class="section-intro">
            <div>
              <h2>授权运行态</h2>
              <p>从产品供给到授权验证，核心状态聚合于此。</p>
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
              <span>{{ ["产品", "批次", "代理", "授权", "当前有效"][i] }}</span
              ><strong>{{ v }}</strong>
            </div>
          </div>
          <div class="overview-grid">
            <section class="panel">
              <div class="panel-head"><h3>最近授权</h3></div>
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
            </section>
            <section class="panel">
              <div class="panel-head"><h3>最近批次</h3></div>
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
          <form
            v-if="action === 'product'"
            class="form-grid"
            @submit.prevent="productCreate"
          >
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
              >过期宽限<input name="expire_grace_days" type="number" value="3"
            /></label>
            <div class="form-actions">
              <button class="btn">确认新增</button>
            </div>
          </form>
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
                  <td><StatusBadge :status="p.status" /></td>
                  <td>
                    <div class="row-actions">
                      <button
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
        <section v-if="page === 'agents'" class="panel">
          <div class="panel-head">
            <h3>代理管理</h3>
            <div class="row-actions" v-if="a.isManager.value">
              <button
                v-for="x in [
                  { k: 'agent', t: '新增代理' },
                  { k: 'user', t: '账号' },
                  { k: 'policy', t: '授权规则' },
                  { k: 'quota', t: '发放额度' },
                  { k: 'agentBatch', t: '代生成' },
                ]"
                class="btn secondary small"
                @click="action = action === x.k ? '' : x.k"
              >
                {{ x.t }}
              </button>
            </div>
          </div>
          <form
            v-if="action"
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
          <div class="agent-grid">
            <div class="table-wrap">
              <table class="table">
                <thead>
                  <tr>
                    <th>代理</th>
                    <th>登录代码</th>
                    <th>联系人</th>
                    <th>状态</th>
                    <th>创建时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="x in a.data.agents">
                    <td>{{ x.name }}</td>
                    <td>
                      <code>{{ x.login_code || x.agent_no }}</code>
                    </td>
                    <td>{{ x.contact_name || "-" }}</td>
                    <td><StatusBadge :status="x.status" /></td>
                    <td>{{ formatTime(x.created_at) }}</td>
                    <td>
                      <div class="row-actions">
                        <button
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
              <h4>代理账号与额度</h4>
              <div v-for="u in a.data.agentUsers" class="detail-row">
                <span
                  ><b>{{ u.display_name || u.username }}</b
                  ><code>{{ u.username }} · {{ u.role }}</code></span
                ><StatusBadge :status="u.status" />
              </div>
              <div v-for="q in a.data.agentQuotas" class="detail-row">
                <span
                  ><b>{{ pname(q.product_id) }}</b
                  ><code>{{
                    q.is_permanent ? "永久" : `${q.duration_days} 天`
                  }}</code></span
                ><strong>{{ q.balance }}</strong>
              </div>
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
          <form
            v-if="action === 'batch'"
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
                      @click="a.showBatch(b.id)"
                    >
                      明细
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
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
        </section>
        <section v-if="page === 'licenses'" class="panel">
          <div class="panel-head"><h3>授权管理</h3></div>
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
                  <th>到期</th>
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
                  <td>{{ formatTime(l.last_verify_at) }}</td>
                  <td>
                    <div class="row-actions">
                      <button
                        class="btn secondary small"
                        @click="a.showBindings(l.license_no)"
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
          <form
            v-if="action === 'offline'"
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
              <span>{{ v }}</span
              ><strong>{{ a.data.riskSummary[k] || 0 }}</strong>
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
            <form
              v-if="action === 'block'"
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
          </section>
        </section>
        <section v-if="page === 'audit'" class="panel">
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
        </section>
        <section v-if="page === 'admins'" class="panel">
          <div class="panel-head">
            <h3>系统设置</h3>
            <button
              v-if="a.isSuper.value"
              class="btn small"
              @click="action = action === 'admin' ? '' : 'admin'"
            >
              <Plus :size="14" />创建管理员
            </button>
          </div>
          <form class="branding-settings" @submit.prevent="saveSiteSettings">
            <div class="branding-copy">
              <span class="eyebrow"><ImageUp :size="15" /> SITE IDENTITY</span>
              <h4>站点品牌</h4>
              <p>统一应用于授权查询、管理控制台和代理工作台。</p>
            </div>
            <div class="branding-fields">
              <label class="field">网站名称<input v-model="siteForm.site_name" maxlength="80" :disabled="!a.isManager.value" required /></label>
              <label class="field">浏览器标题<input v-model="siteForm.browser_title" maxlength="80" :disabled="!a.isManager.value" required /></label>
              <div class="asset-field">
                <span>网站 Logo</span>
                <div class="asset-preview"><img :src="siteForm.logo_data_url || '/assets/yunnuo-mark.svg'" alt="Logo 预览" /></div>
                <label v-if="a.isManager.value" class="btn secondary small"><ImageUp :size="14" />上传图片<input type="file" accept="image/png,image/jpeg,image/webp,image/x-icon" @change="readBrandAsset($event, 'logo_data_url')" /></label>
                <button v-if="a.isManager.value && siteForm.logo_data_url" class="icon-btn" type="button" title="恢复默认 Logo" @click="siteForm.logo_data_url = ''"><RotateCcw :size="16" /></button>
              </div>
              <div class="asset-field">
                <span>网站图标</span>
                <div class="asset-preview favicon"><img :src="siteForm.favicon_data_url || '/assets/yunnuo-mark.svg'" alt="网站图标预览" /></div>
                <label v-if="a.isManager.value" class="btn secondary small"><ImageUp :size="14" />上传图片<input type="file" accept="image/png,image/jpeg,image/webp,image/x-icon" @change="readBrandAsset($event, 'favicon_data_url')" /></label>
                <button v-if="a.isManager.value && siteForm.favicon_data_url" class="icon-btn" type="button" title="恢复默认图标" @click="siteForm.favicon_data_url = ''"><RotateCcw :size="16" /></button>
              </div>
              <div v-if="a.isManager.value" class="form-actions"><button class="btn">保存站点设置</button></div>
            </div>
          </form>
          <section class="version-console">
            <div class="version-hero">
              <span><PackageCheck :size="18" /> 当前运行版本</span>
              <strong>v{{ a.data.system?.current_version || "0.2.0" }}</strong>
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
                <strong>在线升级</strong>
                <p>
                  升级服务尚未启用。后续版本将支持签名升级包、升级前检查和失败回滚。
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
          <form
            v-if="action === 'admin'"
            class="form-grid"
            @submit.prevent="create('/admin/users', $event, '管理员账号已创建')"
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
                <option value="super_admin">超级管理员</option>
              </select></label
            >
            <div class="form-actions">
              <button class="btn">创建管理员</button>
            </div>
          </form>
          <div v-if="a.isSuper.value" class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>账号</th>
                  <th>显示名</th>
                  <th>角色</th>
                  <th>状态</th>
                  <th>最近登录</th>
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
                </tr>
              </tbody>
            </table>
          </div>
          <div class="panel-head"><h3>修改我的密码</h3></div>
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
              <button class="btn">修改密码</button>
            </div>
          </form>
        </section>
      </div>
    </main>
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
