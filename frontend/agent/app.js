const state = {
  token: sessionStorage.getItem("yn.agent_token") || "",
  profile: null,
  products: [],
  quotas: [],
  ledgers: [],
  batches: [],
  licenses: [],
  lastCodes: [],
  rememberedLoginCode: localStorage.getItem("yn.agent_login_code") || "",
};

const $ = (selector) => document.querySelector(selector);

document.addEventListener("DOMContentLoaded", () => {
  $("#loginForm").addEventListener("submit", onLogin);
  $("#logoutBtn").addEventListener("click", () => logout());
  $("#refreshBtn").addEventListener("click", refreshAll);
  $("#batchForm").addEventListener("submit", onGenerateBatch);
  $("#batchProduct").addEventListener("change", updateBatchOptions);
  $("#batchPermanent").addEventListener("change", updatePermanentState);
  $("#copyCodesBtn").addEventListener("click", copyCodes);
  $("#downloadCodesBtn").addEventListener("click", downloadCodes);
  $("#loginForm [name='login_code']").value = state.rememberedLoginCode;
  window.addEventListener("hashchange", syncAgentPage);
  syncAgentPage();
  initialize();
});

const agentPages = {
  overview: "业务概览",
  generate: "生成卡密",
  batches: "卡密批次",
  licenses: "授权记录",
  ledgers: "额度流水",
};

function currentAgentPage() {
  const page = window.location.hash.slice(1);
  return agentPages[page] ? page : "overview";
}

function syncAgentPage() {
  let page = currentAgentPage();
  if (page === "generate" && !canManageCards()) page = "overview";
  $("#pageTitle").textContent = agentPages[page];
  for (const view of document.querySelectorAll(".page-view")) {
    view.classList.toggle("is-current", view.dataset.page === page);
  }
  for (const link of document.querySelectorAll("[data-page-link]")) {
    const current = link.dataset.pageLink === page;
    link.classList.toggle("active", current);
    if (current) link.setAttribute("aria-current", "page");
    else link.removeAttribute("aria-current");
  }
}

async function api(path, options = {}) {
  const headers = { "Content-Type": "application/json", ...(options.headers || {}) };
  if (path !== "/agent/login" && state.token) {
    headers.Authorization = `Bearer ${state.token}`;
  }
  const response = await fetch(path, { ...options, headers });
  const body = await response.json().catch(() => ({}));
  if (!response.ok || body.success === false) {
    const code = body.error?.code || "REQUEST_FAILED";
    const message = body.error?.message || response.statusText;
    const error = new Error(`${code}: ${message}`);
    error.code = code;
    error.status = response.status;
    if (path !== "/agent/login" && response.status === 401) {
      logout("登录状态已失效，请重新登录");
    }
    throw error;
  }
  return body.data;
}

async function initialize() {
  if (!state.token) {
    showLogin();
    return;
  }
  try {
    const profile = await api("/agent/profile");
    showApp(profile);
    await refreshAll();
  } catch (error) {
    showLogin(error.status === 401 ? "登录状态已失效，请重新登录" : error.message);
  }
}

async function onLogin(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const submit = form.querySelector("button[type='submit']");
  submit.disabled = true;
  $("#loginError").textContent = "";
  try {
    const credentials = formJSON(form);
    const result = await api("/agent/login", {
      method: "POST",
      body: JSON.stringify(credentials),
    });
	state.rememberedLoginCode = credentials.login_code.toUpperCase();
	localStorage.setItem("yn.agent_login_code", state.rememberedLoginCode);
    state.token = result.access_token;
    sessionStorage.setItem("yn.agent_token", state.token);
    const profile = await api("/agent/profile");
    showApp(profile);
    form.reset();
	$("#loginForm [name='login_code']").value = state.rememberedLoginCode;
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
  state.profile = profile;
  $("#loginScreen").hidden = true;
  document.body.classList.add("authenticated");
  $("#agentName").textContent = profile.agent_name || profile.agent_no || "代理账号";
  $("#userIdentity").textContent = `${profile.display_name || profile.username} · ${roleLabel(profile.role)}`;
  applyRoleVisibility();
  syncAgentPage();
}

function logout(message = "") {
  state.token = "";
  state.profile = null;
  state.lastCodes = [];
  sessionStorage.removeItem("yn.agent_token");
  showLogin(message);
}

function applyRoleVisibility() {
  const role = state.profile?.role || "";
  for (const element of document.querySelectorAll("[data-roles]")) {
    element.hidden = !element.dataset.roles.split(",").includes(role);
  }
}

function canManageCards() {
  return ["agent_owner", "agent_manager"].includes(state.profile?.role);
}

async function refreshAll() {
  try {
    const [health, products, quotas, ledgers, batches, licenses] = await Promise.all([
      api("/healthz"),
      api("/agent/products"),
      api("/agent/quotas"),
      api("/agent/quota-ledgers"),
      api("/agent/card-batches"),
      api("/agent/licenses"),
    ]);
    $("#healthText").textContent = `服务正常 · ${health.status}`;
    state.products = products || [];
    state.quotas = quotas || [];
    state.ledgers = ledgers || [];
    state.batches = batches || [];
    state.licenses = licenses || [];
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
  renderProducts();
  renderBatchOptions();
  renderBatches();
  renderLicenses();
  renderLedgers();
}

function renderMetrics() {
  const quota = state.quotas.reduce((sum, item) => sum + Math.max(0, item.balance), 0);
  const cards = state.batches.reduce((sum, item) => sum + item.quantity, 0);
  $("#metricQuota").textContent = quota;
  $("#metricBatches").textContent = state.batches.length;
  $("#metricCards").textContent = cards;
  $("#metricLicenses").textContent = state.licenses.length;
}

function renderProducts() {
  $("#productsTable").innerHTML = state.products.length
    ? state.products
        .map((product) => {
          const policy = product.policy;
          const durations = policy.allowed_duration_days?.map((days) => `${days} 天`) || [];
          if (policy.allow_permanent) durations.push("永久");
          const quotaLines = state.quotas
            .filter((item) => item.product_id === product.id)
            .map((item) => `${item.is_permanent ? "永久" : `${item.duration_days} 天`}: ${item.balance}`);
          return `
            <tr>
              <td><strong>${esc(product.name)}</strong><code>${esc(product.code)}</code></td>
              <td>${esc(durations.join("、") || "未配置")}</td>
              <td>${quotaLines.length ? quotaLines.map(esc).join("<br>") : "0"}</td>
              <td>${esc(policy.max_batch_quantity)}</td>
              <td>${policy.can_export_plain_code ? "允许" : "禁止"}</td>
            </tr>
          `;
        })
        .join("")
    : emptyRow(5, "暂无可售产品");
}

function renderBatchOptions() {
  const selectable = state.products.filter(
    (product) => product.status === "active" && product.policy.status === "active" && product.policy.can_generate,
  );
  $("#batchProduct").innerHTML = selectable.length
    ? selectable.map((product) => `<option value="${esc(product.id)}">${esc(product.name)} · ${esc(product.code)}</option>`).join("")
    : "<option value=''>暂无可发卡产品</option>";
  updateBatchOptions();
}

function updateBatchOptions() {
  const product = selectedProduct();
  const durations = product?.policy.allowed_duration_days || [];
  $("#batchDuration").innerHTML = durations.length
    ? durations.map((days) => `<option value="${esc(days)}">${esc(days)} 天</option>`).join("")
    : "<option value='0'>未配置时长</option>";
  $("#batchPermanent").checked = false;
  $("#batchPermanent").disabled = !product?.policy.allow_permanent;
  $("#batchQuantity").max = product?.policy.max_batch_quantity || 1;
  updatePermanentState();
}

function updatePermanentState() {
  $("#batchDuration").disabled = $("#batchPermanent").checked;
}

function renderBatches() {
  $("#batchesTable").innerHTML = state.batches.length
    ? state.batches
        .map((batch) => {
          const product = productByID(batch.product_id);
          const canExport = canManageCards() && product?.policy.can_export_plain_code;
          return `
            <tr>
              <td>${esc(batch.name)}</td>
              <td>${esc(product?.name || batch.product_id)}</td>
              <td>${esc(batch.quantity)}</td>
              <td>${batch.is_permanent ? "永久" : `${esc(batch.duration_days)} 天`}</td>
              <td>${formatTime(batch.created_at)}</td>
              <td>
                <div class="row-actions">
                  <button class="secondary" type="button" onclick="showBatchCards('${escAttr(batch.id)}')">状态</button>
                  ${canExport ? `<button type="button" onclick="exportBatch('${escAttr(batch.id)}')">导出</button>` : ""}
                </div>
              </td>
            </tr>
          `;
        })
        .join("")
    : emptyRow(6, "暂无卡密批次");
}

function renderLicenses() {
  $("#licensesTable").innerHTML = state.licenses.length
    ? state.licenses
        .map(
          (license) => `
            <tr>
              <td><code>${esc(license.license_no)}</code></td>
              <td>${esc(productByID(license.product_id)?.name || license.product_id)}</td>
              <td><span class="status ${esc(license.status)}">${esc(license.status)}</span></td>
              <td>${formatTime(license.activated_at)}</td>
              <td>${license.expired_at ? formatTime(license.expired_at) : "永久"}</td>
              <td>${formatTime(license.last_verify_at)}</td>
            </tr>
          `,
        )
        .join("")
    : emptyRow(6, "暂无授权记录");
}

function renderLedgers() {
  $("#ledgersTable").innerHTML = state.ledgers.length
    ? state.ledgers
        .map(
          (ledger) => `
            <tr>
              <td>${formatTime(ledger.created_at)}</td>
              <td>${esc(productByID(ledger.product_id)?.name || ledger.product_id)}</td>
              <td>${ledger.is_permanent ? "永久" : `${esc(ledger.duration_days)} 天`}</td>
              <td>${esc(ledgerTypeLabel(ledger.change_type))}</td>
              <td class="${ledger.change_quantity >= 0 ? "positive" : "negative"}">${ledger.change_quantity >= 0 ? "+" : ""}${esc(ledger.change_quantity)}</td>
              <td>${esc(ledger.balance_after)}</td>
              <td>${esc(ledger.remark || "-")}</td>
            </tr>
          `,
        )
        .join("")
    : emptyRow(7, "暂无额度流水");
}

async function onGenerateBatch(event) {
  event.preventDefault();
  const input = formJSON(event.currentTarget);
  input.quantity = Number(input.quantity);
  input.duration_days = Number(input.duration_days);
  input.is_permanent = Boolean(input.is_permanent);
  try {
    const result = await api("/agent/card-batches", {
      method: "POST",
      body: JSON.stringify(input),
    });
    showCodes(result.codes || []);
    toast("卡密已生成，请及时保存明文");
    await refreshAll();
    await showBatchCards(result.batch.id);
  } catch (error) {
    toast(error.message);
  }
}

async function showBatchCards(batchID) {
  try {
    const cards = await api(`/agent/card-batches/${encodeURIComponent(batchID)}/cards`);
    $("#cardsEmpty").hidden = true;
    $("#cardsTable").innerHTML = cards.length
      ? cards
          .map(
            (card) => `
              <tr>
                <td><code>${esc(card.code_prefix)}...</code></td>
                <td><span class="status ${esc(card.status)}">${esc(card.status)}</span></td>
                <td>${card.is_permanent ? "永久" : `${esc(card.duration_days)} 天`}</td>
                <td>${formatTime(card.created_at)}</td>
              </tr>
            `,
          )
          .join("")
      : emptyRow(4, "该批次暂无卡密");
    $("#batchDetail").scrollIntoView({ behavior: "smooth", block: "start" });
  } catch (error) {
    toast(error.message);
  }
}

async function exportBatch(batchID) {
  try {
    const result = await api(`/agent/card-batches/${encodeURIComponent(batchID)}/export`, { method: "POST" });
    showCodes(result.codes || []);
    $("#generate").scrollIntoView({ behavior: "smooth", block: "start" });
    toast("批次明文已加载");
  } catch (error) {
    toast(error.message);
  }
}

function showCodes(codes) {
  state.lastCodes = codes;
  $("#codeBox").textContent = codes.length ? codes.join("\n") : "没有可显示的卡密";
  $("#copyCodesBtn").disabled = !codes.length;
  $("#downloadCodesBtn").disabled = !codes.length;
}

async function copyCodes() {
  if (!state.lastCodes.length) return;
  try {
    await navigator.clipboard.writeText(state.lastCodes.join("\n"));
    toast("卡密已复制");
  } catch {
    toast("复制失败，请使用下载");
  }
}

function downloadCodes() {
  if (!state.lastCodes.length) return;
  const blob = new Blob([state.lastCodes.join("\n") + "\n"], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `yn-cards-${new Date().toISOString().slice(0, 10)}.txt`;
  link.click();
  URL.revokeObjectURL(url);
}

function selectedProduct() {
  return productByID($("#batchProduct").value);
}

function productByID(productID) {
  return state.products.find((product) => product.id === productID);
}

function formJSON(form) {
  const data = new FormData(form);
  const out = {};
  for (const [key, value] of data.entries()) out[key] = value;
  for (const input of form.querySelectorAll("input[type='checkbox']")) out[input.name] = input.checked;
  return out;
}

function emptyRow(columns, text) {
  return `<tr><td class="empty-cell" colspan="${columns}">${esc(text)}</td></tr>`;
}

function ledgerTypeLabel(type) {
  return { grant: "额度发放", revoke: "额度扣减", generate_cards: "生成卡密" }[type] || type;
}

function roleLabel(role) {
  return {
    agent_owner: "主账号",
    agent_manager: "经理",
    agent_staff: "员工",
    agent_readonly: "只读",
  }[role] || role;
}

function formatTime(value) {
  if (!value) return "-";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
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
  const element = $("#toast");
  element.textContent = message;
  element.classList.add("show");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => element.classList.remove("show"), 2600);
}
