import { computed, reactive, ref } from "vue";
import { ApiError, request } from "../shared/api";
import { toast } from "../shared/useToast";
import { applyBranding } from "../shared/branding";

export function useAdmin() {
  const token = ref(sessionStorage.getItem("yn.admin_token") || "");
  const profile = ref<any>();
  const loading = ref(false);
  const loginError = ref("");
  const loginChallenge = ref<{ challenge: string; expires_at: string } | null>(null);
  const data = reactive<any>({
    products: [],
    agents: [],
    batches: [],
    licenses: [],
    licensePage: { items: [], total: 0, page: 1, page_size: 20 },
    offline: [],
    users: [],
    audit: [],
    riskSummary: {},
    riskBlocks: [],
    riskAlerts: [],
    batchCards: [],
    bindings: [],
    agentUsers: [],
    agentQuotas: [],
    keys: null,
    system: null,
    siteSettings: null,
  });
  const selected = reactive({
    product: localStorage.getItem("yn.product_id") || "",
    agent: localStorage.getItem("yn.agent_id") || "",
    batch: "",
    license: "",
  });
  const filters = reactive({
    license: { status: "", product_id: "", agent_id: "", q: "" },
    audit: { actor_type: "", result: "", action: "" },
    risk: { status: "open", severity: "", product_id: "" },
  });
  const isManager = computed(() =>
    ["super_admin", "admin"].includes(profile.value?.role),
  );
  const isSuper = computed(() => profile.value?.role === "super_admin");
  async function api<T = any>(path: string, options: RequestInit = {}) {
    try {
      return await request<T>(
        path,
        options,
        path.startsWith("/admin/login") ? "" : token.value,
      );
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && !path.startsWith("/admin/login"))
        logout("登录状态已失效，请重新登录");
      throw e;
    }
  }
  const params = (o: any) => {
    const p = new URLSearchParams();
    Object.entries(o).forEach(([k, v]) => {
      if (v !== "" && v != null) p.set(k, String(v));
    });
    return p.toString();
  };
  const loginMessage = (error: any) => String(error.message || error).replace(/^[A-Z_]+:\s*/, "");
  async function finishLogin(result: any) {
    token.value = result.access_token;
    sessionStorage.setItem("yn.admin_token", token.value);
    loginChallenge.value = null;
    profile.value = await api("/admin/profile");
    await refresh();
  }
  async function signIn(input: any) {
    if (loading.value) return;
    loading.value = true;
    loginError.value = "";
    try {
      const r = await api("/admin/login/start", {
        method: "POST",
        body: JSON.stringify(input),
      });
      if (r.mfa) {
        loginChallenge.value = r.mfa;
        return "mfa";
      }
      await finishLogin(r.login);
      return "signed_in";
    } catch (e: any) {
      loginError.value = loginMessage(e);
    } finally {
      loading.value = false;
    }
  }
  async function completeSignIn(code: string) {
    if (!loginChallenge.value || loading.value) return;
    loading.value = true;
    loginError.value = "";
    try {
      const result = await api("/admin/login/mfa", {
        method: "POST",
        body: JSON.stringify({ challenge: loginChallenge.value.challenge, code }),
      });
      await finishLogin(result);
    } catch (e: any) {
      if (e instanceof ApiError && ["MFA_CHALLENGE_INVALID", "MFA_CHALLENGE_EXPIRED"].includes(e.code))
        loginChallenge.value = null;
      loginError.value = loginMessage(e);
    } finally {
      loading.value = false;
    }
  }
  function resetLoginStep() {
    loginChallenge.value = null;
    loginError.value = "";
  }
  function logout(message = "") {
    resetLoginStep();
    token.value = "";
    profile.value = null;
    sessionStorage.removeItem("yn.admin_token");
    loginError.value = message;
  }
  async function refresh() {
    loading.value = true;
    try {
      const lp = {
        ...filters.license,
        page: data.licensePage.page,
        page_size: 20,
      };
      const [
        products,
        agents,
        batches,
        licensePage,
        offline,
        users,
        audit,
        riskSummary,
        riskBlocks,
        riskAlerts,
        system,
        siteSettings,
        stats,
      ] = await Promise.all([
        api("/admin/products"),
        api("/admin/agents"),
        api("/admin/card-batches"),
        api(`/admin/licenses?${params(lp)}`),
        api("/admin/offline-licenses"),
        isSuper.value ? api("/admin/users") : [],
        api(`/admin/audit-logs?${params({ ...filters.audit, limit: 100 })}`),
        api("/admin/risk/summary"),
        api("/admin/risk/blocks"),
        api(`/admin/risk/alerts?${params({ ...filters.risk, limit: 100 })}`),
        api("/admin/system/version"),
        api("/v1/site/settings"),
        api("/admin/stats"),
      ]);
      const arrays = {
        products: products || [],
        agents: agents || [],
        batches: batches || [],
        offline: offline || [],
        users: users || [],
        audit: audit || [],
        riskBlocks: riskBlocks || [],
        riskAlerts: riskAlerts || [],
      };
      Object.assign(data, {
        ...arrays,
        licensePage: licensePage || { items: [], total: 0, page: 1, page_size: 20 },
        licenses: licensePage?.items || [],
        riskSummary,
        system,
        siteSettings,
        stats: stats || null,
      });
      applyBranding(siteSettings, "管理控制台");
      if (!selected.product && arrays.products[0]) selected.product = arrays.products[0].id;
      if (!selected.agent && arrays.agents[0]) selected.agent = arrays.agents[0].id;
    } catch (e) {
      toast(e);
    } finally {
      loading.value = false;
    }
  }
  async function mutate(
    path: string,
    body?: any,
    message = "操作成功",
    refreshAfter = true,
  ) {
    try {
      const r = await api(path, {
        method: "POST",
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      toast(message);
      if (refreshAfter) await refresh();
      return r;
    } catch (e) {
      toast(e);
      throw e;
    }
  }
  async function showBatch(id: string) {
    selected.batch = id;
    data.batchCards = (await api(
      `/admin/card-batches/${encodeURIComponent(id)}/cards`,
    )) || [];
  }
  async function showBindings(no: string) {
    selected.license = no;
    data.bindings = (await api(
      `/admin/licenses/${encodeURIComponent(no)}/bindings`,
    )) || [];
  }
  async function showAgent(id: string) {
    selected.agent = id;
    localStorage.setItem("yn.agent_id", id);
    const [quotas, users] = await Promise.all([
      api(`/admin/agents/${id}/quotas`),
      api(`/admin/agents/${id}/users`),
    ]);
    // 空列表在后端序列化为 null，直接赋值会让模板的 .length 抛错。
    data.agentQuotas = quotas || [];
    data.agentUsers = users || [];
  }
  async function showKeys(id: string) {
    data.keys = await api(`/admin/products/${id}/keys`);
  }
  return {
    token,
    profile,
    loading,
    loginError,
    loginChallenge,
    data,
    selected,
    filters,
    isManager,
    isSuper,
    api,
    signIn,
    completeSignIn,
    resetLoginStep,
    logout,
    refresh,
    mutate,
    showBatch,
    showBindings,
    showAgent,
    showKeys,
  };
}
