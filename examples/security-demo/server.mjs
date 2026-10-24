import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { LicenseClient, enforceWebLicense } from "../../sdk/node/src/index.js";

const config=JSON.parse(await readFile(process.env.YN_SECURITY_DEMO_CONFIG,"utf8"));
const client=new LicenseClient({baseUrl:config.baseUrl,appKey:config.appKey,timeoutMs:10000});
const html=`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Ebox · 授权接入演示</title><style>body{background:#eff4ef;color:#183b2b;font:18px/1.8 system-ui;margin:0;display:grid;place-items:center;min-height:100vh}main{background:white;max-width:800px;padding:64px;border-radius:24px;border:1px solid #d4e1d7}h1{font-size:48px;line-height:1.4}button{background:#174d32;color:white;padding:16px 30px;border:0;border-radius:8px;font-size:17px}#result{padding-top:24px;color:#8a3420}</style><main><small>EBOX / 企业网站</small><h1>Ebox 网站正常运行</h1><p>客户内容与应用功能保持可用。当前访问已通过授权服务的在线检查。</p><button id="update">检查在线更新</button><p id="result" role="status"></p></main><script>document.getElementById('update').onclick=async()=>{const r=await fetch('/api/update',{method:'POST'});const d=await r.json();document.getElementById('result').textContent=d.code==='SECURITY_UPDATES_DENIED'?'仅更新服务被限制，当前网站仍可使用。':d.code||'更新服务正常';};</script></html>`;
createServer(async(req,res)=>{
  if(req.url==="/healthz"){res.writeHead(200);res.end("ok");return}
  if(req.url==="/api/update"&&req.method==="POST"){
    try{const data=await client.checkUpdate({...config.binding,current_version:"1.0.0"});res.writeHead(200,{"Content-Type":"application/json"});res.end(JSON.stringify(data))}
    catch(error){res.writeHead(error.httpStatus||503,{"Content-Type":"application/json"});res.end(JSON.stringify({code:error.code||"SERVICE_UNAVAILABLE"}))}return
  }
  if(await enforceWebLicense(client,config.binding,res))return;
  res.writeHead(200,{"Content-Type":"text/html; charset=utf-8","Cache-Control":"no-store"});res.end(html);
}).listen(Number(process.env.YN_SECURITY_DEMO_PORT),"127.0.0.1");
