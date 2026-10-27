import test from "node:test";
import assert from "node:assert/strict";
import {generateKeyPairSync,sign} from "node:crypto";
import {mkdtemp,rm,readFile,stat} from "node:fs/promises";
import {tmpdir} from "node:os";
import {join} from "node:path";
import {RuntimeLicense,HighWaterGuard,bindingDigest,bindingForWebRequest} from "../src/index.js";

const {privateKey,publicKey}=generateKeyPairSync("rsa",{modulusLength:2048});
const signed=claims=>{const raw=Buffer.from(JSON.stringify(claims));return raw.toString("base64url")+"."+sign("RSA-SHA256",raw,privateKey).toString("base64url")};
async function fixture(options={}){
 const stateDir=await mkdtemp(join(tmpdir(),"license-runtime-"));
 let now=Date.parse("2026-10-27T00:00:00Z"),failure="",count=0,highWater=null,wrongInstance=false;
 const terms={bind_mode:"domain",license_mode:"subscription",require_instance:true,features:["export"]};
 const deadline=new Date(now+5000).toISOString();
 const fetch=async(url,request)=>{
  const input=JSON.parse(request.body);
  if(failure==="network")throw Error("network unavailable");
  if(failure==="revoked")return Response.json({success:false,error:{code:"LICENSE_REVOKED"}},{status:403});
  const base={type:"license",product_code:"DEMO",license_no:"lic_demo",key_version:1,bind_mode:"domain",bind_digest:bindingDigest("domain",input.bind_value||"app.example.com"),instance_id:wrongInstance?"other_installation_0001":input.instance_id,issued_at:"2026-10-27T00:00:00Z",can_use:true,can_update:true,license_mode:"subscription",usage_expired_at:deadline,updates_expired_at:new Date(Date.parse(deadline)-2000).toISOString(),grace_until:deadline,authorization_terms:terms};
  if(url.endsWith("/requirements"))return Response.json({success:true,data:{authorization_terms:terms}});
  if(url.endsWith("/verify")){count++;if(now>=Date.parse(deadline))return Response.json({success:false,error:{code:"LICENSE_EXPIRED"}},{status:403})}
  const rights={...base,bind_mode:"domain",bind_value:input.bind_value,license_token:signed(base),offline_token:signed({...base,type:"offline_cache",offline_until:deadline,license_expired_at:deadline})};
  return Response.json({success:true,data:rights});
 };
 const config={baseUrl:"https://license.example.com",appKey:"yn0123456789abcdef",productId:"yn0123456789abcdef",productCode:"DEMO",stateDir,publicKeys:{1:publicKey.export({type:"spki",format:"pem"})},clock:()=>new Date(now),fetch,highWaterGuard:new HighWaterGuard({store:{load:()=>highWater,save:value=>{highWater=value}}}),...options};
 const runtime=await RuntimeLicense.open(config);await runtime.activate("card","app.example.com");
 return {runtime,config,stateDir,advance:ms=>{now+=ms},fail:value=>{failure=value},wrong:()=>{wrongInstance=true},count:()=>count,close:()=>rm(stateDir,{recursive:true,force:true})};
}

test("positive cache stops at use expiry and never extends update entitlement",async()=>{
 const f=await fixture({cacheSeconds:60});try{
  assert.equal((await f.runtime.requireFeature("export")).can_use,true);assert.equal(f.count(),1);
  f.advance(3500);assert.equal((await f.runtime.verify()).can_update,false);assert.equal(f.count(),1);
  f.advance(1500);await assert.rejects(f.runtime.verify(),e=>e.code==="LICENSE_EXPIRED");assert.equal(f.count(),2);
 }finally{await f.close()}
});

test("offline fallback is opt-in, bound and cannot override explicit server revocation",async()=>{
 const f=await fixture({allowOffline:true});try{
  await f.runtime.verify();f.fail("network");assert.equal((await f.runtime.verify()).offline,true);
  assert.equal((await f.runtime.verify()).can_update,false);
  f.fail("revoked");await assert.rejects(f.runtime.verify(),e=>e.code==="LICENSE_REVOKED");
  f.fail("network");await assert.rejects(f.runtime.verify());
 }finally{await f.close()}
});

test("signed online result for another installation cannot be cached or used offline",async()=>{
 const f=await fixture({allowOffline:true});try{
  f.wrong();await assert.rejects(f.runtime.verify(),e=>e.code==="BINDING_MISMATCH");
  f.fail("network");await assert.rejects(f.runtime.verify());
 }finally{await f.close()}
});

test("first-open concurrency preserves a single private installation identity",async()=>{
 const f=await fixture();try{
  const before=await readFile(join(f.stateDir,"instance.json"),"utf8");
  await Promise.all(Array.from({length:10},()=>RuntimeLicense.open(f.config)));
  assert.equal(await readFile(join(f.stateDir,"instance.json"),"utf8"),before);
  assert.equal((await stat(join(f.stateDir,"instance.json"))).mode&0o777,0o600);
  assert.throws(()=>new RuntimeLicense({...f.config,allowOffline:true,highWaterGuard:undefined}),/guard/);
 }finally{await f.close()}
});

test("web binding follows actual Host and only explicitly trusted proxies",()=>{
 const binding={license_no:"lic",bind_mode:"domain_wildcard",bind_value:"*.example.com"};
 const request={headers:{host:"wrong.example.net:443","x-forwarded-host":"allowed.example.com"},socket:{remoteAddress:"10.0.0.2"}};
 assert.equal(bindingForWebRequest(binding,request).bind_value,"wrong.example.net");
 assert.equal(bindingForWebRequest(binding,request,{trustedProxies:["10.0.0.2"]}).bind_value,"allowed.example.com");
 assert.throws(()=>bindingForWebRequest(binding,undefined),e=>e.code==="HOST_REQUIRED");
});
