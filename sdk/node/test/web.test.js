import assert from "node:assert/strict";
import test from "node:test";
import { enforceWebLicense, renderLicenseBlockPage } from "../src/index.js";

test("web guard permits usage when only updates are restricted", async () => {
  const handled = await enforceWebLicense({securityDecision:async()=>({action:"deny_updates",can_use:true,can_update:false})}, {}, {writeHead(){assert.fail("allowed usage was blocked")}});
  assert.equal(handled,false);
});
test("web guard renders a safe case-specific block page and prevents caching", async () => {
  const result={};const decision={action:"block_page",can_use:false,case_id:'case_<script>',message:'<img src=x onerror=alert(1)>',support_url:'javascript:alert(1)'};
  assert.equal(await enforceWebLicense({securityDecision:async()=>decision},{},{writeHead(status,headers){Object.assign(result,{status,headers})},end(body){result.body=body}}),true);
  assert.equal(result.status,403);assert.equal(result.headers["Cache-Control"],"no-store");
  assert.match(result.body,/授权安全拦截/);assert.ok(!result.body.includes('<img'));assert.ok(!result.body.includes('href="javascript:'));assert.match(result.body,/&lt;script&gt;/);
});
test("network failures and ordinary expiry are not labeled as confirmed piracy", async () => {
  const result={};await enforceWebLicense({securityDecision:async()=>{throw Error("timeout")}}, {}, {writeHead(status){result.status=status},end(body){result.body=body}});
  assert.equal(result.status,503);assert.match(result.body,/授权服务暂时不可用/);assert.ok(!result.body.includes("已确认的授权安全处置"));
  const html=renderLicenseBlockPage({action:"authorization_unavailable",message:"授权已过期"});assert.ok(!html.includes("授权安全拦截"));assert.ok(!html.includes("已确认的授权安全处置"));
});
