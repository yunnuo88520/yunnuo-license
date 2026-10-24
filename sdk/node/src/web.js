const escape = (text) => String(text ?? "").replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"})[c]);

export function renderLicenseBlockPage(decision, unavailable = false) {
  const title = unavailable ? "授权服务暂时不可用" : decision.action === "block_page" ? "授权安全拦截" : "应用授权暂不可用";
  const message = unavailable ? "当前无法确认授权状态，请稍后重试。" : decision.message;
  let support = "";
  try { const u = new URL(decision.support_url); if (["https:", "http:"].includes(u.protocol) && !u.username && !u.password) support = `<a href="${escape(u.href)}" rel="noopener noreferrer">联系服务提供方</a>`; } catch { /* No configured support link. */ }
  return `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${escape(title)}</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f3f5f2;color:#17201b;font:17px/1.8 system-ui,sans-serif}main{max-width:620px;margin:30px;padding:48px;background:white;border:1px solid #d8e1da;border-radius:18px}small{color:#68746b}h1{font-size:32px;margin:12px 0 24px}.mark{font-size:15px;color:#8e3a28}p{overflow-wrap:anywhere}a{display:inline-block;margin-top:24px;color:#275c3d}code{display:block;font-size:13px}</style><main><span class="mark">${unavailable ? "服务连接异常" : decision.action === "block_page" ? "已确认的授权安全处置" : "授权状态核验"}</span><h1>${escape(title)}</h1><p>${escape(message)}</p>${decision.case_id ? `<small>处置编号<code>${escape(decision.case_id)}</code>授权用户可登录授权中心提交申诉，管理员核实后解除。</small>` : ""}${support}</main></html>`;
}

// Returns true after responding; false means the application can continue serving.
export async function enforceWebLicense(client, binding, response) {
  let decision;
  try {
    decision = await client.securityDecision(binding);
    if (!decision || typeof decision.can_use !== "boolean") throw new Error("invalid security decision");
  }
  catch (error) {
    const authorizationFailure = [400,401,403,404].includes(error?.httpStatus);
    response.writeHead(authorizationFailure ? 403 : 503, {"Content-Type":"text/html; charset=utf-8","Cache-Control":"no-store","Content-Security-Policy":"default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'"});
    response.end(renderLicenseBlockPage({action:"authorization_unavailable",message:"当前授权信息未通过核验，请联系服务提供方。"}, !authorizationFailure));
    return true;
  }
  if (decision.can_use === true) return false;
  response.writeHead(403, {"Content-Type":"text/html; charset=utf-8","Cache-Control":"no-store","Content-Security-Policy":"default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'"});
  response.end(renderLicenseBlockPage(decision));
  return true;
}
