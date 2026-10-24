const escape = (text) => String(text ?? "").replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"})[c]);

export function renderLicenseBlockPage(decision, unavailable = false) {
  const blocked = !unavailable && decision.action === "block_page";
  const title = unavailable ? "授权服务暂时不可用" : blocked ? "授权安全拦截" : "应用授权暂不可用";
  const message = unavailable ? "当前无法确认授权状态，请稍后重试。" : decision.message;
  let support = "";
  try { const u = new URL(decision.support_url); if (["https:", "http:"].includes(u.protocol) && !u.username && !u.password) support = `<a class="contact" href="${escape(u.href)}" target="_blank" rel="noopener noreferrer">联系服务提供方 <span aria-hidden="true">↗</span></a>`; } catch { /* No configured support link. */ }
  const shield = `<svg viewBox="0 0 32 32" fill="none" aria-hidden="true"><path d="m16 3 10 4v8c0 6-4 10-10 14C10 25 6 21 6 15V7l10-4Z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="M16 10v7m0 4v.2" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`;
  return `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'"><title>${escape(title)}</title>
<style>
*{box-sizing:border-box}body{margin:0;min-height:100vh;background:#f4f5f1;color:#202821;font:15px/1.7 system-ui,-apple-system,"Segoe UI","Microsoft YaHei",sans-serif;-webkit-font-smoothing:antialiased}body::before{content:"";position:fixed;inset:0;z-index:-1;background-image:linear-gradient(#243b2410 1px,transparent 1px),linear-gradient(90deg,#243b2410 1px,transparent 1px);background-size:48px 48px;mask-image:linear-gradient(#0006,transparent 85%)}.shell{max-width:1032px;padding:40px 28px 28px;margin:auto}.brand-bar{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:42px}.brand{display:flex;align-items:center;gap:12px}.brand-symbol{display:grid;place-items:center;width:42px;height:46px;background:#24392c;color:#f4f8ef;border-radius:12px}.brand-symbol svg{width:27px;height:27px}.brand strong{font-size:16px;letter-spacing:.04em}.brand small{display:block;font-size:11px;letter-spacing:.12em;color:#788176}.status{display:flex;align-items:center;gap:8px;color:#726658;font-size:12px}.status::before{content:"";width:7px;height:7px;background:#be6a42;border-radius:50%}.block-panel{background:#fff;border:1px solid #dce1d8;border-radius:24px;box-shadow:0 20px 70px #263a2810;overflow:hidden}.hero{padding:44px 44px 30px;display:flex;align-items:flex-start;gap:22px}.hero-symbol{flex:none;display:grid;place-items:center;width:66px;height:72px;background:#fff0e7;color:#af542f;border:1px solid #f2d9c9;border-radius:18px}.hero-symbol svg{width:42px;height:42px}.eyebrow{margin:0 0 6px;color:#a45b3c;font-size:12px;letter-spacing:.08em;font-weight:600}h1{font-size:36px;line-height:1.4;letter-spacing:.02em;margin:0 0 12px;font-weight:700}.intro{margin:0;color:#737b71;line-height:1.8}.announcement{margin:0 44px 30px;padding:25px 28px 28px;background:#fff7f0;border:1px solid #f1decf;border-left:4px solid #b96840;border-radius:12px}.announcement h2{display:flex;align-items:center;gap:8px;margin:0 0 16px;font-size:12px;font-weight:600;letter-spacing:.1em;color:#9b5738}.announcement h2::before{content:"";width:5px;height:5px;background:currentColor;border-radius:50%}.announcement-text{margin:0;font-size:24px;line-height:1.85;font-weight:600;color:#573b2c;white-space:pre-wrap;overflow-wrap:anywhere}.guidance{padding:0 44px 32px}.guidance>p{color:#697469;max-width:680px;margin:0 0 20px}.steps{display:flex;flex-wrap:wrap;gap:14px 28px;padding:0;margin:0;list-style:none;color:#455247;font-size:13px}.steps li{display:flex;align-items:center;gap:9px}.steps span{color:#8d9989;font-size:11px;font-variant-numeric:tabular-nums}.panel-bottom{padding:24px 44px;border-top:1px solid #e9ede5;background:#fafbf8;display:flex;justify-content:space-between;gap:22px;align-items:center}.reference{min-width:0}.reference small{font-size:11px;color:#7d877a}.reference code{display:block;color:#586552;font:11px/1.8 ui-monospace,SFMono-Regular,Consolas,monospace;overflow-wrap:anywhere}.appeal-hint{margin:7px 0 0;font-size:12px;color:#7b8577}.contact{flex:none;display:inline-flex;align-items:center;justify-content:center;gap:22px;min-height:44px;padding:10px 20px;border:1px solid #24392c;border-radius:8px;background:#24392c;color:white;text-decoration:none;font-size:13px;font-weight:600}.contact:hover{background:#36533e}.contact:focus-visible{outline:3px solid #acbd8f;outline-offset:3px}.page-footer{text-align:center;margin:24px 0 0;color:#899180;font-size:11px;letter-spacing:.05em}.neutral .hero-symbol{background:#edf2f3;color:#56717a;border-color:#dce5e7}.neutral .eyebrow{color:#56717a}.neutral .announcement{background:#f3f6f6;border-color:#dfe7e7;border-left-color:#6e878c}.neutral .announcement h2,.neutral .announcement-text{color:#405c63}.neutral .status::before{background:#7a9599}
@media(min-width:800px) and (min-height:850px){.shell{padding-top:70px}.brand-bar{margin-bottom:62px}}
@media(max-width:600px){.shell{padding:24px 16px}.brand-bar{margin-bottom:26px}.brand small{font-size:9px}.status{font-size:11px}.hero{padding:28px 24px 24px;gap:16px}.hero-symbol{width:46px;height:52px;border-radius:13px}.hero-symbol svg{width:30px;height:30px}h1{font-size:27px;margin-bottom:10px}.eyebrow{font-size:10px}.intro{font-size:13px}.announcement{margin:0 24px 24px;padding:20px}.announcement-text{font-size:20px}.guidance{padding:0 24px 24px}.guidance>p{font-size:13px}.steps{gap:9px;display:grid}.panel-bottom{padding:20px 24px;flex-direction:column;align-items:stretch;gap:18px}.contact{width:100%}.block-panel{border-radius:18px}.page-footer{font-size:10px}}
</style></head>
<body class="${blocked ? "restriction" : "neutral"}"><div class="shell">
<header class="brand-bar"><div class="brand"><span class="brand-symbol">${shield}</span><div><strong>${blocked ? "正版授权保护" : "应用授权服务"}</strong><small>${blocked ? "保护原创 · 支持正版" : "应用授权状态核验"}</small></div></div><span class="status">${unavailable ? "连接暂时中断" : "当前访问受限"}</span></header>
<main class="block-panel"><div class="hero"><span class="hero-symbol">${shield}</span><div><p class="eyebrow">${blocked ? "软件正版保护公告" : unavailable ? "服务连接提示" : "授权状态提示"}</p><h1>${escape(title)}</h1><p class="intro">${blocked ? "当前站点已暂停访问，请查看下方公告并联系服务提供方处理。" : unavailable ? "当前无法连接授权服务，请稍后重试。" : "请核对授权有效期及部署信息，或联系服务提供方处理。"}</p></div></div>
<section class="announcement" aria-labelledby="announcement-title"><h2 id="announcement-title">${blocked ? "打击盗版公告" : "授权说明"}</h2><p class="announcement-text">${escape(message)}</p></section>
${blocked ? `<div class="guidance"><p>如您是站点管理员，请核实软件来源与授权信息。授权用户也可登录授权中心提交申诉，由管理员核实处理。</p><ol class="steps"><li><span>01</span> 核对软件来源</li><li><span>02</span> 联系服务提供方</li><li><span>03</span> 核实后恢复访问</li></ol></div>` : ""}
${decision.case_id || support ? `<footer class="panel-bottom"><div class="reference">${decision.case_id ? `<small>联系时请提供此处置编号</small><code>${escape(decision.case_id)}</code>` : `<small>请按公告中的联系方式咨询</small>`}${blocked ? `<p class="appeal-hint">完成授权核实后，由管理员解除访问限制。</p>` : ""}</div>${support}</footer>` : ""}</main>
<footer class="page-footer">${blocked ? "支持正版软件 · 尊重开发者劳动成果" : "应用授权服务"}</footer></div></body></html>`;
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
