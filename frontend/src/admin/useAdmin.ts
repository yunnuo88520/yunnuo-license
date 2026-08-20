import { computed, reactive, ref } from "vue";
import { ApiError, request } from "../shared/api";
import { toast } from "../shared/useToast";
import { applyBranding } from "../shared/branding";

export function useAdmin() {
  const token = ref(sessionStorage.getItem("yn.admin_token") || "");
  const profile = ref<any>();
  const loading = ref(false);
  const loginError = ref("");
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
        path === "/admin/login" ? "" : token.value,
      );
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && path !== "/admin/login")
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
  async function signIn(input: any) {
    loading.value = true;
    loginError.value = "";
    try {
      const r = await api("/admin/login", {
        method: "POST",
        body: JSON.stringify(input),
      });
      token.value = r.access_token;
      sessionStorage.setItem("yn.admin_token", token.value);
      profile.value = await api("/admin/profile");
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
    data.batchCards = await api(
      `/admin/card-batches/${encodeURIComponent(id)}/cards`,
    );
  }
  async function showBindings(no: string) {
    selected.license = no;
    data.bindings = await api(
      `/admin/licenses/${encodeURIComponent(no)}/bindings`,
    );
  }
  async function showAgent(id: string) {
    selected.agent = id;
    localStorage.setItem("yn.agent_id", id);
    [data.agentQuotas, data.agentUsers] = await Promise.all([
      api(`/admin/agents/${id}/quotas`),
      api(`/admin/agents/${id}/users`),
    ]);
  }
  async function showKeys(id: string) {
    data.keys = await api(`/admin/products/${id}/keys`);
  }
  return {
    token,
    profile,
    loading,
    loginError,
    data,
    selected,
    filters,
    isManager,
    isSuper,
    api,
    signIn,
    logout,
    refresh,
    mutate,
    showBatch,
    showBindings,
    showAgent,
    showKeys,
  };
}
