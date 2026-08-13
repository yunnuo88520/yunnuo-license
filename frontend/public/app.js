const $ = (selector) => document.querySelector(selector);

document.addEventListener("DOMContentLoaded", () => {
  $("#queryForm").addEventListener("submit", queryAuthorization);
  $("#clearBtn").addEventListener("click", clearResult);
});

async function queryAuthorization(event) {
  event.preventDefault();
  const value = $("#queryValue").value.trim();
  const button = event.currentTarget.querySelector("button[type='submit']");
  const payload = value.toLowerCase().startsWith("lic_") ? { license_no: value } : { card_code: value };
  button.disabled = true;
  $("#queryError").textContent = "";
  try {
    const response = await fetch("/v1/licenses/query", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const body = await response.json().catch(() => ({}));
    if (!response.ok || body.success === false) {
      const code = body.error?.code || "REQUEST_FAILED";
      throw new Error(code === "AUTHORIZATION_NOT_FOUND" ? "未查询到该授权" : "查询失败，请稍后重试");
    }
    renderResult(body.data);
  } catch (error) {
    $("#resultSection").hidden = true;
    $("#queryError").textContent = error.message;
  } finally {
    button.disabled = false;
  }
}

function renderResult(result) {
  const status = result.license_status || result.card_status || "unknown";
  $("#statusBadge").textContent = statusLabel(status);
  $("#statusBadge").className = `status ${status}`;
  $("#productName").textContent = result.product_name;
  $("#productCode").textContent = result.product_code;
  $("#licenseNo").textContent = result.license_no || "-";
  $("#cardStatus").textContent = statusLabel(result.card_status);
  $("#licenseStatus").textContent = statusLabel(result.license_status);
  $("#duration").textContent = result.is_permanent ? "永久" : result.duration_days ? `${result.duration_days} 天` : "按授权到期时间";
  $("#activatedAt").textContent = formatTime(result.activated_at);
  $("#expiredAt").textContent = result.is_permanent ? "永久" : formatTime(result.expired_at);
  $("#lastVerifyAt").textContent = formatTime(result.last_verify_at);
  $("#serverTime").textContent = formatTime(result.server_time);
  $("#licenseNoItem").hidden = !result.license_no;
  $("#cardStatusItem").hidden = !result.card_status;
  $("#resultSection").hidden = false;
  $("#resultSection").scrollIntoView({ behavior: "smooth", block: "start" });
}

function clearResult() {
  $("#resultSection").hidden = true;
  $("#queryValue").value = "";
  $("#queryError").textContent = "";
  $("#queryValue").focus();
  window.scrollTo({ top: 0, behavior: "smooth" });
}

function statusLabel(status) {
  return {
    unused: "未激活",
    activated: "已激活",
    consumed_for_renewal: "已用于续费",
    voided: "已作废",
    active: "有效",
    expired: "已过期",
    revoked: "已吊销",
  }[status] || (status ? status : "-");
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
