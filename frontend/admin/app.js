const state = {
  products: [],
  agents: [],
  batches: [],
  licenses: [],
  licensePage: { total: 0, page: 1, page_size: 20 },
  offlineLicenses: [],
  adminUsers: [],
  auditLogs: [],
  batchCards: [],
  selectedBatchId: "",
  adminToken: sessionStorage.getItem("yn.admin_token") || "",
  adminProfile: null,
  selectedProductId: localStorage.getItem("yn.product_id") || "",
  selectedAppKey: localStorage.getItem("yn.app_key") || "",
  lastCardCode: localStorage.getItem("yn.card_code") || "",
  lastLicenseNo: localStorage.getItem("yn.license_no") || "",
  selectedAgentId: localStorage.getItem("yn.agent_id") || "",
};

const $ = (selector) => document.querySelector(selector);

document.addEventListener("DOMContentLoaded", () => {
  $("#loginForm").addEventListener("submit", onAdminLogin);
  $("#logoutBtn").addEventListener("click", () => logoutAdmin());
  $("#refreshBtn").addEventListener("click", refreshAll);
  $("#seedBtn").addEventListener("click", createSeedData);
  $("#productForm").addEventListener("submit", onCreateProduct);
  $("#agentForm").addEventListener("submit", onCreateAgent);
  $("#agentUserForm").addEventListener("submit", onCreateAgentUser);
  $("#agentPolicyForm").addEventListener("submit", onSaveAgentPolicy);
  $("#agentQuotaForm").addEventListener("submit", onGrantAgentQuota);
  $("#agentBatchForm").addEventListener("submit", onAgentCreateBatch);
  $("#batchForm").addEventListener("submit", onCreateBatch);
  $("#activateForm").addEventListener("submit", onActivate);
  $("#verifyForm").addEventListener("submit", onVerify);
  $("#adminUserForm").addEventListener("submit", onCreateAdminUser);
  $("#passwordForm").addEventListener("submit", onChangeAdminPassword);
  $("#auditFilterForm").addEventListener("submit", onAuditFilter);
  $("#exportBatchBtn").addEventListener("click", exportSelectedBatch);
  $("#licenseFilterForm").addEventListener("submit", onLicenseFilter);
  $("#licensePrevBtn").addEventListener("click", () => changeLicensePage(-1));
  $("#licenseNextBtn").addEventListener("click", () => changeLicensePage(1));
  $("#offlineLicenseForm").addEventListener("submit", onCreateOfflineLicense);
  $("#cardCodeInput").value = state.lastCardCode;
  $("#licenseNoInput").value = state.lastLicenseNo;
  window.addEventListener("hashchange", syncAdminPage);
  syncAdminPage();
  initializeAdmin();
});

const adminPages = {
  overview: "业务概览",
  products: "产品管理",
  agents: "代理管理",
  cards: "卡密批次",
  licenses: "授权管理",
  tools: "联调工具",
  offline: "离线授权",
  audit: "审计日志",
  admins: "系统设置",
};

function currentAdminPage() {
  const page = window.location.hash.slice(1);
  return adminPages[page] ? page : "overview";
}

function navigateAdminPage(page) {
  window.location.hash = adminPages[page] ? page : "overview";
}

function openAdminAction(page, panel) {
  navigateAdminPage(page);
  window.setTimeout(() => toggleActionPanel(panel, true), 0);
}

function toggleActionPanel(name, forceOpen) {
  const panel = document.querySelector(`[data-action-panel="${name}"]`);
  if (!panel) return;
  const open = forceOpen ?? !panel.classList.contains("open");
  panel.classList.toggle("open", open);
  const trigger = document.querySelector(`[data-action-trigger="${name}"]`);
  if (trigger) trigger.classList.toggle("active", open);
  if (open) panel.querySelector("input, select")?.focus();
}

function showAgentAction(name) {
  const target = document.querySelector(`[data-action-panel="${name}"]`);
  const willOpen = target && !target.classList.contains("open");
  for (const panel of document.querySelectorAll("#agents [data-action-panel]")) {
    panel.classList.remove("open");
  }
  for (const trigger of document.querySelectorAll("#agents [data-agent-action], #agents [data-action-trigger]")) {
    trigger.classList.remove("active");
  }
  if (willOpen) {
    target.classList.add("open");
    document.querySelector(`#agents [data-agent-action="${name}"], #agents [data-action-trigger="${name}"]`)?.classList.add("active");
    target.querySelector("input, select")?.focus();
  }
}

function syncAdminPage() {
  const page = currentAdminPage();
  $("#pageTitle").textContent = adminPages[page];
  for (const view of document.querySelectorAll(".page-view")) {
    view.classList.toggle("is-current", view.dataset.page === page);
  }
  for (const link of document.querySelectorAll("[data-page-link]")) {
    const current = link.dataset.pageLink === page;
    link.classList.toggle("active", current);
    if (current) link.setAttribute("aria-current", "page");
    else link.removeAttribute("aria-current");
  }
  for (const panel of document.querySelectorAll(".page-view:not(.is-current) [data-action-panel]")) {
    panel.classList.remove("open");
  }
  for (const trigger of document.querySelectorAll(".page-view:not(.is-current) [data-action-trigger], .page-view:not(.is-current) [data-agent-action]")) {
    trigger.classList.remove("active");
  }
}

async function api(path, options = {}) {
  const headers = { "Content-Type": "application/json", ...(options.headers || {}) };
  if (path.startsWith("/admin/") && path !== "/admin/login" && state.adminToken) {
    headers.Authorization = `Bearer ${state.adminToken}`;
  }
  const response = await fetch(path, {
    ...options,
    headers,
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok || body.success === false) {
    const code = body.error?.code || "REQUEST_FAILED";
    const message = body.error?.message || response.statusText;
    const error = new Error(`${code}: ${message}`);
    error.code = code;
    error.status = response.status;
    if (path !== "/admin/login" && response.status === 401 && path.startsWith("/admin/")) {
      logoutAdmin("登录状态已失效，请重新登录");
    }
    throw error;
  }
  return body.data;
}

async function initializeAdmin() {
  if (!state.adminToken) {
    showLogin();
    return;
  }
  try {
    const profile = await api("/admin/profile");
    showApp(profile);
    await refreshAll();
  } catch (error) {
    showLogin(error.status === 401 ? "登录状态已失效，请重新登录" : error.message);
  }
}

async function onAdminLogin(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const submit = form.querySelector("button[type='submit']");
  submit.disabled = true;
  $("#loginError").textContent = "";
  try {
    const result = await api("/admin/login", {
      method: "POST",
      body: JSON.stringify(formJSON(form)),
    });
    state.adminToken = result.access_token;
    sessionStorage.setItem("yn.admin_token", state.adminToken);
    const profile = await api("/admin/profile");
    showApp(profile);
    form.reset();
    await refreshAll();
  } catch (error) {
    showLogin(error.message);
  } finally {
    submit.disabled = false;
  }
}

function showLogin(message = "") {
  document.body.classList.remove("authenticated");
  $("#loginScreen").hidden = false;
  $("#loginError").textContent = message;
}

function showApp(profile) {
  state.adminProfile = profile;
  $("#loginScreen").hidden = true;
  document.body.classList.add("authenticated");
  $("#adminIdentity").textContent = `${profile.display_name || profile.username} · ${roleLabel(profile.role)}`;
  applyRoleVisibility();
  syncAdminPage();
}

function logoutAdmin(message = "") {
  state.adminToken = "";
  state.adminProfile = null;
  state.adminUsers = [];
  sessionStorage.removeItem("yn.admin_token");
  showLogin(message);
}

function applyRoleVisibility() {
  const role = state.adminProfile?.role || "";
  for (const element of document.querySelectorAll("[data-roles]")) {
    const roles = element.dataset.roles.split(",");
    element.hidden = !roles.includes(role);
  }
}

function hasAdminRole(...roles) {
  return roles.includes(state.adminProfile?.role);
}

async function refreshAll() {
  try {
    const health = await api("/healthz");
    $("#healthText").textContent = `服务正常 · ${health.status}`;
    const [products, agents, batches, licensePage, offlineLicenses, adminUsers, auditLogs] = await Promise.all([
      api("/admin/products"),
      api("/admin/agents"),
      api("/admin/card-batches"),
      api(licenseURL()),
      api("/admin/offline-licenses"),
      hasAdminRole("super_admin") ? api("/admin/users") : Promise.resolve([]),
      api(auditURL()),
    ]);
    state.products = products || [];
    state.agents = agents || [];
    state.batches = batches || [];
    state.licensePage = licensePage || { items: [], total: 0, page: 1, page_size: 20 };
    state.licenses = state.licensePage.items || [];
    state.offlineLicenses = offlineLicenses || [];
    state.adminUsers = adminUsers || [];
    state.auditLogs = auditLogs || [];
    if (!state.selectedProductId && state.products.length > 0) {
      rememberProduct(state.products[0]);
    }
    render();
  } catch (error) {
    if (error.status !== 401) {
      $("#healthText").textContent = "服务异常";
      toast(error.message);
    }
  }
}

function render() {
  renderMetrics();
  renderOverviewActivity();
  renderProductOptions();
  renderProducts();
  renderAgents();
  renderBatches();
  renderLicenses();
  renderOfflineLicenses();
  renderAdminUsers();
  renderAuditLogs();
  renderBatchCards();
}

function renderOverviewActivity() {
  const productName = new Map(state.products.map((product) => [product.id, product.name]));
  $("#recentLicenses").innerHTML = state.licenses.slice(0, 5).map((license) => `
    <button type="button" onclick="navigateAdminPage('licenses')">
      <span><strong>${esc(productName.get(license.product_id) || license.product_id)}</strong><code>${esc(license.license_no)}</code></span>
      <span><em class="status ${esc(license.status)}">${esc(licenseStatusLabel(license.status))}</em><small>${formatTime(license.activated_at)}</small></span>
    </button>
  `).join("") || `<div class="activity-empty">暂无授权记录</div>`;
  $("#recentBatches").innerHTML = state.batches.slice(0, 5).map((batch) => `
    <button type="button" onclick="navigateAdminPage('cards')">
      <span><strong>${esc(batch.name)}</strong><small>${esc(productName.get(batch.product_id) || batch.product_id)}</small></span>
      <span><b>${esc(batch.quantity)} 张</b><small>${formatTime(batch.created_at)}</small></span>
    </button>
  `).join("") || `<div class="activity-empty">暂无卡密批次</div>`;
}

function renderMetrics() {
  $("#metricProducts").textContent = state.products.length;
  $("#metricBatches").textContent = state.batches.length;
  $("#metricAgents").textContent = state.agents.length;
  $("#metricLicenses").textContent = state.licensePage.total || 0;
  $("#metricLatest").textContent = state.licenses.filter((license) => license.status === "active").length;
}

function renderProductOptions() {
  const options = state.products
    .map((product) => `<option value="${esc(product.id)}">${esc(product.name)} · ${esc(product.code)}</option>`)
    .join("");
  for (const select of [$("#batchProduct"), $("#activateProduct"), $("#policyProduct"), $("#quotaProduct"), $("#agentBatchProduct"), $("#offlineProduct")]) {
    select.innerHTML = options || "<option value=''>暂无产品</option>";
    select.value = state.selectedProductId || state.products[0]?.id || "";
  }
  const agentOptions = state.agents
    .map((agent) => `<option value="${esc(agent.id)}">${esc(agent.name)} · ${esc(agent.login_code || agent.agent_no)}</option>`)
    .join("");
  for (const select of [$("#policyAgent"), $("#quotaAgent"), $("#agentBatchAgent"), $("#userAgent")]) {
    select.innerHTML = agentOptions || "<option value=''>暂无代理</option>";
    select.value = state.selectedAgentId || state.agents[0]?.id || "";
  }
  renderLicenseFilterOptions(options, agentOptions);
}

function renderLicenseFilterOptions(productOptions, agentOptions) {
  const productSelect = $("#licenseFilterProduct");
  const agentSelect = $("#licenseFilterAgent");
  const currentProduct = productSelect.value;
  const currentAgent = agentSelect.value;
  productSelect.innerHTML = `<option value="">全部产品</option>${productOptions}`;
  agentSelect.innerHTML = `<option value="">全部代理</option>${agentOptions}`;
  productSelect.value = currentProduct;
  agentSelect.value = currentAgent;
}

function renderProducts() {
  const canManage = hasAdminRole("super_admin", "admin");
  const canRotateKeys = hasAdminRole("super_admin");
  $("#productsTable").innerHTML = state.products
    .map(
      (product) => `
        <tr>
          <td>${esc(product.name)}</td>
          <td>${esc(product.code)}</td>
          <td><code>${esc(product.app_key)}</code></td>
          <td>v${esc(product.key_version || 1)}</td>
          <td>${esc(bindModeLabel(product.bind_mode))}</td>
          <td>${esc(product.max_bind_count)}</td>
          <td><span class="status ${esc(product.status)}">${esc(productStatusLabel(product.status))}</span></td>
          <td>
            <div class="row-actions">
              <button class="secondary" type="button" onclick="showProductKeys('${escAttr(product.id)}')">公钥</button>
              ${canRotateKeys ? `<button class="secondary" type="button" onclick="rotateProductKey('${escAttr(product.id)}')">轮换</button>` : ""}
              ${canManage ? product.status === "active" ? `<button class="danger" type="button" onclick="setProductStatus('${escAttr(product.id)}','disable')">停用</button>` : `<button type="button" onclick="setProductStatus('${escAttr(product.id)}','enable')">恢复</button>` : ""}
            </div>
          </td>
        </tr>
      `,
    )
    .join("");
}

function renderAgents() {
  const canManage = hasAdminRole("super_admin", "admin");
  $("#agentsTable").innerHTML = state.agents
    .map(
      (agent) => `
        <tr>
          <td>${esc(agent.name)}</td>
          <td><code>${esc(agent.login_code || agent.agent_no)}</code></td>
          <td>${esc(agent.contact_name || "-")}</td>
          <td><span class="status ${esc(agent.status)}">${esc(agentStatusLabel(agent.status))}</span></td>
          <td>${formatTime(agent.created_at)}</td>
          <td>
            <div class="row-actions">
              <button class="secondary" type="button" onclick="selectAgent('${escAttr(agent.id)}')">详情</button>
              ${
                canManage
                  ? agent.status === "active"
                    ? `<button class="secondary" type="button" onclick="setAgentStatus('${escAttr(agent.id)}','suspend')">暂停</button><button class="danger" type="button" onclick="setAgentStatus('${escAttr(agent.id)}','disable')">停用</button>`
                    : `<button type="button" onclick="setAgentStatus('${escAttr(agent.id)}','enable')">恢复</button>${agent.status === "suspended" ? `<button class="danger" type="button" onclick="setAgentStatus('${escAttr(agent.id)}','disable')">停用</button>` : ""}`
                  : ""
              }
            </div>
          </td>
        </tr>
      `,
    )
    .join("");
  const current = state.agents.find((agent) => agent.id === state.selectedAgentId) || state.agents[0];
  if (current) {
    selectAgent(current.id, { silent: true });
  }
}

function renderBatches() {
  const productName = new Map(state.products.map((product) => [product.id, product.name]));
  const agentName = new Map(state.agents.map((agent) => [agent.id, agent.name]));
  $("#batchesTable").innerHTML = state.batches
    .map(
      (batch) => `
        <tr>
          <td>${esc(batch.name)}</td>
          <td>${esc(productName.get(batch.product_id) || batch.product_id)}${batch.agent_id ? ` · ${esc(agentName.get(batch.agent_id) || batch.agent_id)}` : ""}</td>
          <td>${esc(batch.quantity)}</td>
          <td>${batch.is_permanent ? "永久" : `${esc(batch.duration_days)} 天`}</td>
          <td>${esc(batch.export_count)}</td>
          <td>${formatTime(batch.created_at)}</td>
          <td><button class="secondary" type="button" onclick="showBatchCards('${escAttr(batch.id)}')">明细</button></td>
        </tr>
      `,
    )
    .join("");
}

function renderBatchCards() {
  $("#cards").classList.toggle("detail-open", Boolean(state.selectedBatchId));
  const canVoid = hasAdminRole("super_admin", "admin");
  const batch = state.batches.find((item) => item.id === state.selectedBatchId);
  $("#batchCardsTitle").textContent = batch?.name || "批次卡密明细";
  $("#batchCardsMeta").textContent = batch ? `${batch.quantity} 张 · 已导出 ${batch.export_count} 次` : "选择批次后查看";
  $("#exportBatchBtn").disabled = !batch || !hasAdminRole("super_admin", "admin", "operator");
  $("#batchCardsTable").innerHTML = state.batchCards.length
    ? state.batchCards
        .map(
          (card) => `
            <tr>
              <td><code>${esc(shortID(card.id))}</code></td>
              <td><span class="status ${esc(card.status)}">${esc(cardStatusLabel(card.status))}</span></td>
              <td>${card.is_permanent ? "永久" : `${esc(card.duration_days)} 天`}</td>
              <td>${esc(card.void_reason || "-")}</td>
              <td>${formatTime(card.created_at)}</td>
              <td>${canVoid && card.status === "unused" ? `<button class="danger" type="button" onclick="voidBatchCard('${escAttr(card.id)}')">作废</button>` : "-"}</td>
            </tr>
          `,
        )
        .join("")
    : `<tr><td colspan="6" class="empty-cell">${batch ? "该批次暂无卡密" : "选择批次后查看卡密"}</td></tr>`;
}

function renderLicenses() {
  const productName = new Map(state.products.map((product) => [product.id, product.name]));
  const canManage = hasAdminRole("super_admin", "admin");
  const rows = state.licenses
    .map(
      (license) => `
        <tr>
          <td><code>${esc(license.license_no)}</code></td>
          <td>${esc(productName.get(license.product_id) || license.product_id)}</td>
          <td><span class="status ${esc(license.status)}">${esc(license.status)}</span></td>
          <td>${license.expired_at ? formatTime(license.expired_at) : "永久"}</td>
          <td>${license.last_verify_at ? formatTime(license.last_verify_at) : "-"}</td>
          <td>
            <div class="row-actions">
              <button class="secondary" type="button" onclick="showBindings('${escAttr(license.license_no)}')">绑定</button>
              ${canManage ? `<button class="danger" type="button" onclick="revokeLicense('${escAttr(license.license_no)}')">吊销</button>` : ""}
            </div>
          </td>
        </tr>
      `,
    )
    .join("");
  $("#licensesTable").innerHTML = rows || `<tr><td colspan="6" class="empty-cell">暂无符合条件的授权</td></tr>`;
  const totalPages = Math.max(1, Math.ceil((state.licensePage.total || 0) / state.licensePage.page_size));
  $("#licensePageInfo").textContent = `第 ${state.licensePage.page} / ${totalPages} 页 · 共 ${state.licensePage.total || 0} 条`;
  $("#licensePrevBtn").disabled = state.licensePage.page <= 1;
  $("#licenseNextBtn").disabled = state.licensePage.page >= totalPages;
}

function renderOfflineLicenses() {
  const productName = new Map(state.products.map((product) => [product.id, product.name]));
  const canManage = hasAdminRole("super_admin", "admin");
  $("#offlineLicensesTable").innerHTML = state.offlineLicenses.length
    ? state.offlineLicenses
        .map(
          (license) => `
            <tr>
              <td><code>${esc(license.license_no)}</code></td>
              <td>${esc(productName.get(license.product_id) || license.product_id)}</td>
              <td>${esc(license.label || "-")}</td>
              <td><code>${esc(license.machine_code_masked)}</code></td>
              <td><span class="status ${esc(license.status)}">${esc(licenseStatusLabel(license.status))}</span></td>
              <td>${license.expired_at ? formatTime(license.expired_at) : "永久"}</td>
              <td>
                <div class="row-actions">
                  ${
                    canManage && license.status === "active"
                      ? `<button class="secondary" type="button" onclick="downloadOfflineLicense('${escAttr(license.id)}')">下载</button>
                         <button class="danger" type="button" onclick="revokeOfflineLicense('${escAttr(license.id)}')">吊销</button>`
                      : "-"
                  }
                </div>
              </td>
            </tr>
          `,
        )
        .join("")
    : `<tr><td colspan="7" class="empty-cell">暂无离线授权</td></tr>`;
}

function renderAdminUsers() {
  $("#adminUsersTable").innerHTML = state.adminUsers
    .map(
      (user) => `
        <tr>
          <td><code>${esc(user.username)}</code></td>
          <td>${esc(user.display_name || "-")}</td>
          <td>${esc(roleLabel(user.role))}</td>
          <td><span class="status ${esc(user.status)}">${esc(user.status)}</span></td>
          <td>${formatTime(user.last_login_at)}</td>
        </tr>
      `,
    )
    .join("");
}

function renderAuditLogs() {
  $("#auditTable").innerHTML = state.auditLogs.length
    ? state.auditLogs
        .map(
          (log) => `
            <tr>
              <td>${formatTime(log.created_at)}</td>
              <td>${esc(actorTypeLabel(log.actor_type))}</td>
              <td class="audit-action"><code>${esc(log.action)}</code></td>
              <td><span class="status ${esc(log.result)}">${esc(resultLabel(log.result))}</span></td>
              <td><code>${esc(shortID(log.agent_id))}</code></td>
              <td><code>${esc(shortID(log.product_id))}</code></td>
              <td><code>${esc(shortID(log.license_id))}</code></td>
              <td>${esc(log.error_code || "-")}</td>
            </tr>
          `,
        )
        .join("")
    : `<tr><td colspan="8" class="empty-cell">暂无审计记录</td></tr>`;
}

function licenseURL() {
  const params = new URLSearchParams();
  const form = $("#licenseFilterForm");
  if (form) {
    const input = formJSON(form);
    for (const key of ["status", "product_id", "agent_id", "q"]) {
      if (input[key]) params.set(key, input[key]);
    }
  }
  params.set("page", String(state.licensePage.page || 1));
  params.set("page_size", String(state.licensePage.page_size || 20));
  return `/admin/licenses?${params.toString()}`;
}

async function onLicenseFilter(event) {
  event.preventDefault();
  state.licensePage.page = 1;
  await refreshLicensePage();
}

async function changeLicensePage(delta) {
  const totalPages = Math.max(1, Math.ceil((state.licensePage.total || 0) / state.licensePage.page_size));
  const nextPage = Math.min(totalPages, Math.max(1, state.licensePage.page + delta));
  if (nextPage === state.licensePage.page) return;
  state.licensePage.page = nextPage;
  await refreshLicensePage();
}

async function refreshLicensePage() {
  try {
    state.licensePage = await api(licenseURL());
    state.licenses = state.licensePage.items || [];
    renderLicenses();
    renderMetrics();
  } catch (error) {
    toast(error.message);
  }
}

function auditURL() {
  const form = $("#auditFilterForm");
  const params = new URLSearchParams();
  if (form) {
    const input = formJSON(form);
    for (const key of ["actor_type", "result", "action"]) {
      if (input[key]) params.set(key, input[key]);
    }
  }
  params.set("limit", "100");
  return `/admin/audit-logs?${params.toString()}`;
}

async function onAuditFilter(event) {
  event.preventDefault();
  try {
    state.auditLogs = (await api(auditURL())) || [];
    renderAuditLogs();
  } catch (error) {
    toast(error.message);
  }
}

async function onCreateAdminUser(event) {
  event.preventDefault();
  const form = event.currentTarget;
  try {
    await api("/admin/users", { method: "POST", body: JSON.stringify(formJSON(form)) });
    form.reset();
    toggleActionPanel("admin-create", false);
    toast("管理员账号已创建");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function onChangeAdminPassword(event) {
  event.preventDefault();
  const form = event.currentTarget;
  try {
    await api("/admin/password", { method: "POST", body: JSON.stringify(formJSON(form)) });
    form.reset();
    logoutAdmin("密码已修改，请使用新密码登录");
  } catch (error) {
    toast(error.message);
  }
}

async function onCreateProduct(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  input.max_bind_count = Number(input.max_bind_count);
  input.offline_grace_days = Number(input.offline_grace_days);
  input.expire_grace_days = Number(input.expire_grace_days);
  try {
    const product = await api("/admin/products", { method: "POST", body: JSON.stringify(input) });
    rememberProduct(product);
    toggleActionPanel("product-create", false);
    toast("产品已创建");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function setProductStatus(productId, action) {
  try {
    await api(`/admin/products/${encodeURIComponent(productId)}/${action}`, { method: "POST" });
    toast(action === "enable" ? "产品已恢复" : "产品已停用");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function showProductKeys(productId) {
  try {
    const ring = await api(`/admin/products/${encodeURIComponent(productId)}/keys`);
    $("#keyRingTitle").textContent = `${ring.product_code} 公钥 · 当前 v${ring.current_version}`;
    $("#keyRingContent").innerHTML = ring.keys
      .map((key) => `<section><strong>v${esc(key.key_version)}</strong><span>${formatTime(key.created_at)}</span><pre>${esc(key.public_key_pem)}</pre></section>`)
      .join("");
    $("#keyRingDialog").showModal();
  } catch (error) {
    toast(error.message);
  }
}

function closeKeyRing() {
  $("#keyRingDialog").close();
}

async function rotateProductKey(productId) {
  if (!window.confirm("确定轮换产品签名密钥吗？客户端需要保留历史公钥，才能继续验证旧授权。")) return;
  try {
    const ring = await api(`/admin/products/${encodeURIComponent(productId)}/keys/rotate`, { method: "POST" });
    toast(`密钥已轮换至 v${ring.current_version}`);
    await refreshAll();
    await showProductKeys(productId);
  } catch (error) {
    toast(error.message);
  }
}

async function onCreateAgent(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  try {
    const agent = await api("/admin/agents", { method: "POST", body: JSON.stringify(input) });
    state.selectedAgentId = agent.id;
    localStorage.setItem("yn.agent_id", agent.id);
    showAgentAction("agent-create");
    toast("代理已创建");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function onCreateAgentUser(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  const agentId = input.agent_id;
  delete input.agent_id;
  try {
    await api(`/admin/agents/${encodeURIComponent(agentId)}/users`, {
      method: "POST",
      body: JSON.stringify(input),
    });
    toast("代理账号已创建");
    showAgentAction("agent-user");
    await selectAgent(agentId, { silent: true });
  } catch (error) {
    toast(error.message);
  }
}

async function onSaveAgentPolicy(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  input.can_generate = Boolean(input.can_generate);
  input.can_export_plain_code = Boolean(input.can_export_plain_code);
  input.allow_permanent = Boolean(input.allow_permanent);
  input.max_batch_quantity = Number(input.max_batch_quantity);
  input.allowed_duration_days = String(input.allowed_duration_days || "")
    .split(",")
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isFinite(item) && item > 0);
  try {
    await api("/admin/agent-policies", { method: "POST", body: JSON.stringify(input) });
    toast("代理政策已保存");
    showAgentAction("agent-policy");
  } catch (error) {
    toast(error.message);
  }
}

async function onGrantAgentQuota(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  input.duration_days = Number(input.duration_days);
  input.quantity = Number(input.quantity);
  input.is_permanent = false;
  try {
    await api("/admin/agent-quotas/grant", { method: "POST", body: JSON.stringify(input) });
    toast("额度已发放");
    showAgentAction("agent-quota");
    await showAgentQuota(input.agent_id);
  } catch (error) {
    toast(error.message);
  }
}

async function onAgentCreateBatch(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  const agentId = input.agent_id;
  input.quantity = Number(input.quantity);
  input.duration_days = Number(input.duration_days);
  delete input.agent_id;
  try {
    const result = await api(`/admin/agents/${encodeURIComponent(agentId)}/card-batches`, {
      method: "POST",
      body: JSON.stringify(input),
    });
    if (result.codes?.length) {
      state.lastCardCode = result.codes[0];
      localStorage.setItem("yn.card_code", state.lastCardCode);
      $("#cardCodeInput").value = state.lastCardCode;
      $("#codeBox").textContent = result.codes.join("\n");
    }
    toast("代理卡密已生成");
    showAgentAction("agent-batch");
    await showAgentQuota(agentId);
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function onCreateBatch(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  input.quantity = Number(input.quantity);
  input.duration_days = Number(input.duration_days);
  input.is_permanent = Boolean(input.is_permanent);
  try {
    const result = await api("/admin/card-batches", { method: "POST", body: JSON.stringify(input) });
    if (result.codes?.length) {
      state.lastCardCode = result.codes[0];
      localStorage.setItem("yn.card_code", state.lastCardCode);
      $("#cardCodeInput").value = state.lastCardCode;
      $("#codeBox").textContent = result.codes.join("\n");
    }
    toast("卡密已生成");
    toggleActionPanel("batch-create", false);
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function onCreateOfflineLicense(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const input = formJSON(form);
  input.duration_days = Number(input.duration_days);
  input.is_permanent = Boolean(input.is_permanent);
  try {
    await api("/admin/offline-licenses", { method: "POST", body: JSON.stringify(input) });
    form.reset();
    $("#offlineProduct").value = state.selectedProductId || state.products[0]?.id || "";
    toast("离线授权已生成");
    toggleActionPanel("offline-create", false);
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function downloadOfflineLicense(id) {
  try {
    const response = await fetch(`/admin/offline-licenses/${encodeURIComponent(id)}/download`, {
      headers: { Authorization: `Bearer ${state.adminToken}` },
    });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new Error(`${body.error?.code || "DOWNLOAD_FAILED"}: ${body.error?.message || response.statusText}`);
    }
    const blob = await response.blob();
    const disposition = response.headers.get("Content-Disposition") || "";
    const filename = disposition.match(/filename="?([^";]+)"?/i)?.[1] || "license.key";
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
    toast("离线授权文件已下载");
  } catch (error) {
    toast(error.message);
  }
}

async function revokeOfflineLicense(id) {
  if (!window.confirm("确定吊销这份离线授权吗？吊销后将不能再次下载。")) return;
  try {
    await api(`/admin/offline-licenses/${encodeURIComponent(id)}/revoke`, {
      method: "POST",
      body: JSON.stringify({ reason: "admin_revoked" }),
    });
    toast("离线授权已吊销");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function showBatchCards(batchId) {
  try {
    state.selectedBatchId = batchId;
    state.batchCards = (await api(`/admin/card-batches/${encodeURIComponent(batchId)}/cards`)) || [];
    renderBatchCards();
    $("#batchCardsTitle").scrollIntoView({ behavior: "smooth", block: "center" });
  } catch (error) {
    toast(error.message);
  }
}

async function exportSelectedBatch() {
  if (!state.selectedBatchId) return;
  try {
    const result = await api(`/admin/card-batches/${encodeURIComponent(state.selectedBatchId)}/export`, { method: "POST" });
    $("#codeBox").textContent = (result.codes || []).join("\n") || "该批次暂无卡密";
    toast("批次明文已导出");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function voidBatchCard(cardId) {
  if (!window.confirm("确定作废这张未使用卡密吗？此操作不能撤销。")) return;
  try {
    await api("/admin/cards/void", {
      method: "POST",
      body: JSON.stringify({ card_id: cardId, reason: "admin_void" }),
    });
    toast("卡密已作废");
    await showBatchCards(state.selectedBatchId);
    state.auditLogs = (await api(auditURL())) || [];
    renderAuditLogs();
  } catch (error) {
    toast(error.message);
  }
}

async function showAgentQuota(agentId) {
  if (!agentId) return;
  try {
    const [quotas, users] = await Promise.all([
      api(`/admin/agents/${encodeURIComponent(agentId)}/quotas`),
      api(`/admin/agents/${encodeURIComponent(agentId)}/users`),
    ]);
    const quotaItems = quotas || [];
    const userItems = users || [];
    const canManage = hasAdminRole("super_admin", "admin");
    const productName = new Map(state.products.map((product) => [product.id, product.name]));
    $("#agentQuotaBox").innerHTML = `
      <div class="binding-list">
        ${
          userItems.length
            ? userItems
                .map(
                  (user) => `
                    <div class="binding-item">
                      <div>
                        <strong>${esc(user.display_name || user.username)}</strong>
                        <code>${esc(user.username)} · ${esc(user.role)} · ${esc(user.status)}</code>
                      </div>
                      ${
                        canManage
                          ? user.status === "active"
                            ? `<button class="danger" type="button" onclick="setAgentUserStatus('${escAttr(agentId)}','${escAttr(user.id)}','disable')">停用</button>`
                            : `<button type="button" onclick="setAgentUserStatus('${escAttr(agentId)}','${escAttr(user.id)}','enable')">恢复</button>`
                          : ""
                      }
                    </div>
                  `,
                )
                .join("")
            : `<div class="binding-item"><div><strong>暂无代理账号</strong></div></div>`
        }
      </div>
      ${
        quotaItems.length
          ? `
        <div class="binding-list">
          ${quotaItems
            .map(
              (quota) => `
                <div class="binding-item">
                  <div>
                    <strong>${esc(productName.get(quota.product_id) || quota.product_id)} · ${quota.is_permanent ? "永久" : `${esc(quota.duration_days)} 天`}</strong>
                    <code>剩余额度 ${esc(quota.balance)}</code>
                  </div>
                </div>
              `,
            )
            .join("")}
        </div>
      `
          : `<div class="binding-list"><div class="binding-item"><div><strong>暂无额度</strong><code>${esc(agentId)}</code></div></div></div>`
      }
    `;
  } catch (error) {
    toast(error.message);
  }
}

async function setAgentStatus(agentId, action) {
  try {
    await api(`/admin/agents/${encodeURIComponent(agentId)}/${action}`, { method: "POST" });
    toast(action === "enable" ? "代理已恢复" : action === "suspend" ? "代理已暂停" : "代理已停用");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function setAgentUserStatus(agentId, userId, action) {
  try {
    await api(`/admin/agents/${encodeURIComponent(agentId)}/users/${encodeURIComponent(userId)}/${action}`, { method: "POST" });
    toast(action === "enable" ? "代理账号已恢复" : "代理账号已停用");
    await showAgentQuota(agentId);
    state.auditLogs = (await api(auditURL())) || [];
    renderAuditLogs();
  } catch (error) {
    toast(error.message);
  }
}

async function selectAgent(agentId, options = {}) {
  if (!agentId) return;
  state.selectedAgentId = agentId;
  localStorage.setItem("yn.agent_id", agentId);
  for (const select of [$("#policyAgent"), $("#quotaAgent"), $("#agentBatchAgent"), $("#userAgent")]) {
    if (select) select.value = agentId;
  }
  await showAgentQuota(agentId);
  if (!options.silent) {
    toast("代理详情已加载");
  }
}

async function onActivate(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  const product = state.products.find((item) => item.id === input.product_id);
  if (!product) {
    toast("请选择产品");
    return;
  }
  rememberProduct(product);
  const payload = {
    app_key: product.app_key,
    card_code: input.card_code,
    bind_mode: product.bind_mode,
    bind_value: input.bind_value,
    device_name: input.device_name,
  };
  try {
    const result = await api("/v1/licenses/activate", { method: "POST", body: JSON.stringify(payload) });
    state.lastCardCode = input.card_code;
    state.lastLicenseNo = result.license_no;
    localStorage.setItem("yn.card_code", state.lastCardCode);
    localStorage.setItem("yn.license_no", state.lastLicenseNo);
    $("#licenseNoInput").value = result.license_no;
    $("#resultBox").textContent = pretty(result);
    toast("激活成功");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function onVerify(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  const product = selectedProduct();
  if (!product) {
    toast("请选择产品");
    return;
  }
  try {
    const result = await api("/v1/licenses/verify", {
      method: "POST",
      body: JSON.stringify({
        app_key: product.app_key,
        license_no: input.license_no,
        bind_mode: product.bind_mode,
        bind_value: input.bind_value,
      }),
    });
    state.lastLicenseNo = input.license_no;
    localStorage.setItem("yn.license_no", state.lastLicenseNo);
    $("#resultBox").textContent = pretty(result);
    toast("验证通过");
    await refreshAll();
  } catch (error) {
    $("#resultBox").textContent = error.message;
    toast(error.message);
  }
}

async function showBindings(licenseNo) {
  try {
    const bindings = await api(`/admin/licenses/${encodeURIComponent(licenseNo)}/bindings`);
    const canManage = hasAdminRole("super_admin", "admin");
    $("#bindingsBox").innerHTML = `
      <div class="binding-list">
        ${bindings
          .map(
            (binding) => `
              <div class="binding-item">
                <div>
                  <strong>${esc(bindModeLabel(binding.bind_mode))} · <span class="status ${esc(binding.status)}">${esc(bindingStatusLabel(binding.status))}</span></strong>
                  <code>${esc(binding.id)}</code>
                  <code>${esc(binding.display_name || "-")} · ${formatTime(binding.activated_at)}</code>
                </div>
                ${canManage ? `<button class="secondary" type="button" onclick="adminUnbind('${escAttr(licenseNo)}','${escAttr(binding.id)}')">解绑</button>` : ""}
              </div>
            `,
          )
          .join("")}
      </div>
    `;
  } catch (error) {
    toast(error.message);
  }
}

async function adminUnbind(licenseNo, bindingId) {
  try {
    await api("/admin/licenses/unbind", {
      method: "POST",
      body: JSON.stringify({ license_no: licenseNo, binding_id: bindingId, reason: "admin" }),
    });
    toast("绑定已解绑");
    await showBindings(licenseNo);
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function revokeLicense(licenseNo) {
  try {
    await api("/admin/licenses/revoke", {
      method: "POST",
      body: JSON.stringify({ license_no: licenseNo, reason: "admin" }),
    });
    toast("授权已吊销");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

async function createSeedData() {
  try {
    const product = await api("/admin/products", {
      method: "POST",
      body: JSON.stringify({
        name: `测试产品 ${Date.now().toString().slice(-4)}`,
        code: `YN${Date.now().toString().slice(-2)}`,
        bind_mode: "device",
        max_bind_count: 1,
        bind_conflict_strategy: "reject",
        offline_grace_days: 15,
        expire_grace_days: 3,
      }),
    });
    rememberProduct(product);
    const batch = await api("/admin/card-batches", {
      method: "POST",
      body: JSON.stringify({
        product_id: product.id,
        name: "快速测试",
        quantity: 1,
        duration_days: 30,
      }),
    });
    const cardCode = batch.codes[0];
    $("#codeBox").textContent = cardCode;
    $("#cardCodeInput").value = cardCode;
    state.lastCardCode = cardCode;
    localStorage.setItem("yn.card_code", cardCode);
    toast("测试产品和卡密已创建");
    await refreshAll();
  } catch (error) {
    toast(error.message);
  }
}

function selectedProduct() {
  const id = $("#activateProduct").value || state.selectedProductId;
  return state.products.find((item) => item.id === id);
}

function rememberProduct(product) {
  state.selectedProductId = product.id;
  state.selectedAppKey = product.app_key;
  localStorage.setItem("yn.product_id", product.id);
  localStorage.setItem("yn.app_key", product.app_key);
}

function formJSON(form) {
  const data = new FormData(form);
  const out = {};
  for (const [key, value] of data.entries()) {
    out[key] = value;
  }
  for (const input of form.querySelectorAll("input[type='checkbox']")) {
    out[input.name] = input.checked;
  }
  return out;
}

function formatTime(value) {
  if (!value) return "-";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function roleLabel(role) {
  return {
    super_admin: "超级管理员",
    admin: "管理员",
    operator: "运营人员",
    auditor: "审计人员",
  }[role] || role;
}

function agentStatusLabel(status) {
  return { active: "正常", suspended: "已暂停", disabled: "已停用" }[status] || status;
}

function productStatusLabel(status) {
  return { active: "正常", disabled: "已停用" }[status] || status;
}

function bindModeLabel(mode) {
  return { device: "设备", account: "账号", domain: "域名", ip: "IP", none: "无绑定" }[mode] || mode;
}

function bindingStatusLabel(status) {
  return { active: "有效", unbound: "已解绑", kicked: "已移除", revoked: "已吊销" }[status] || status;
}

function cardStatusLabel(status) {
  return {
    unused: "未使用",
    activated: "已激活",
    consumed_for_renewal: "已续费使用",
    voided: "已作废",
  }[status] || status;
}

function licenseStatusLabel(status) {
  return { active: "有效", expired: "已过期", revoked: "已吊销" }[status] || status;
}

function actorTypeLabel(type) {
  return { admin: "管理员", agent: "代理", client: "客户端" }[type] || type;
}

function resultLabel(result) {
  return { success: "成功", failed: "失败" }[result] || result;
}

function shortID(value) {
  if (!value) return "-";
  return value.length > 16 ? `${value.slice(0, 13)}...` : value;
}

function pretty(value) {
  return JSON.stringify(value, null, 2);
}

function esc(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function escAttr(value) {
  return esc(value).replaceAll("`", "&#096;");
}

function toast(message) {
  const el = $("#toast");
  el.textContent = message;
  el.classList.add("show");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => el.classList.remove("show"), 2600);
}
