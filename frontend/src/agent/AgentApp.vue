<script setup lang="ts">
import AgentLicensing from "./AgentLicensing.vue";
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
import { downloadText, isInvalidSession, request } from "../shared/api";
import PasswordChangeForm from "../shared/components/PasswordChangeForm.vue";
import FormDialog from "../shared/components/FormDialog.vue";
import type { PasswordChangeInput } from "../shared/password";
import { accountToken, saveAccountToken, clearAccountToken } from "../shared/accountSession";
import { friendlyError } from "../shared/errors";
import { formatTime, roleLabel } from "../shared/format";
import AppToast from "../shared/components/AppToast.vue";
import StatusBadge from "../shared/components/StatusBadge.vue";
import UserMenu from "../shared/components/UserMenu.vue";
import { toast } from "../shared/useToast";
import { branding, loadBranding, logoSource } from "../shared/branding";

const token = ref(accountToken("agent"));
const licensingPanel=ref<any>(null);
const profile = ref<any>();
const loading = ref(false);
const loginError = ref("");
const loginNotice = ref("");
const passwordOpen = ref(false);
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
      { id: "ledgers", label: "资金流水", icon: ListRestart },
    ],
  },
];
const canManage = computed(() =>
  ["agent_owner", "agent_manager"].includes(profile.value?.role),
);
const metrics = computed(() => [
  money(account.value?.balance || 0),
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
    if (isInvalidSession(e) && path !== "/agent/login")
      logout("登录状态已失效，请重新登录");
    throw e;
  }
}
async function signIn() {
  login.captcha_id = captchaField.value?.captchaId || "";
  loading.value = true;
  loginError.value = "";
  loginNotice.value = "";
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
  passwordOpen.value = false;
  token.value = "";
  profile.value = null;
  clearAccountToken();
  loginError.value = message;
}
function changePassword(input: PasswordChangeInput) {
  return api<{ changed: boolean }>("/user/password", { method: "POST", body: JSON.stringify(input) });
}
function passwordChanged() {
  logout();
  loginNotice.value = "密码已修改，请使用新密码重新登录。";
}
async function refresh() {
  loading.value = true;
  try {
    account.value=await api("/agent/account");
    await licensingPanel.value?.refresh();
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
          <p v-if="loginNotice" class="notice" role="status">{{ loginNotice }}</p>
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
            password-action="修改密码"
            @password="passwordOpen = true"
            @logout="logout()"
          />
        </div>
      </header>
      <div class="workspace"><AgentLicensing ref="licensingPanel" :api="api" :section="page" :can-manage="canManage" :profile="profile" /></div>
    </main>
    <FormDialog :open="passwordOpen" title="修改密码" @close="passwordOpen = false">
      <PasswordChangeForm :change="changePassword" submit-label="确认修改" @changed="passwordChanged" />
    </FormDialog>
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
