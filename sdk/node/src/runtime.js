import {requestHostname} from "./request.js";
export {requestHostname} from "./request.js";
import {randomBytes,randomUUID} from "node:crypto";
import {constants} from "node:fs";
import {mkdir,open,rename,link,unlink,lstat} from "node:fs/promises";
import {join} from "node:path";
import {LicenseClient} from "./client.js";
import {OfflineVerifier} from "./offline.js";
import {APIError} from "./errors.js";

const denied=(code,message)=>Object.assign(new Error(message),{code,httpStatus:403});
async function readJSON(path){
  let file;try{if((await lstat(path)).isSymbolicLink())throw Error("ynlicense: private state must not be a symlink");file=await open(path,constants.O_RDONLY|(constants.O_NOFOLLOW||0));const stat=await file.stat();if(!stat.isFile()||stat.size>2*1024*1024)throw Error("ynlicense: invalid private state");return JSON.parse(await file.readFile("utf8"))}catch(e){if(e.code==="ENOENT")return null;throw e}finally{await file?.close()}
}
async function writeJSON(path,value,create=false){
  const temp=path+"."+randomUUID()+".tmp";const file=await open(temp,"wx",0o600);
  try{await file.writeFile(JSON.stringify(value)+"\n");await file.sync()}finally{await file.close()}
  try{if(create)await link(temp,path);else await rename(temp,path)}finally{await unlink(temp).catch(e=>{if(e.code!=="ENOENT")throw e})}
}
function canonicalJSON(value){return JSON.stringify(Object.fromEntries(Object.entries(value).sort(([a],[b])=>a<b?-1:a>b?1:0))).replace(/[<>&\u2028\u2029]/g,c=>"\\u"+c.charCodeAt(0).toString(16).padStart(4,"0"))}
function actualBinding(mode,registration,context,identity,device){
  if(mode==="composite"){
    const entries=typeof registration==="string"?JSON.parse(registration):registration;
    return canonicalJSON(Object.fromEntries(Object.entries(entries).map(([kind,value])=>[kind,actualBinding(kind,value,context,identity,device)])));
  }
  if(mode==="domain"||mode==="domain_wildcard"){
    let host=context?.request?requestHostname(context.request,context):context?.host;
    if(!host){if(String(registration).startsWith("*."))throw denied("HOST_REQUIRED","泛域名校验需要实际访问域名");host=registration}
    return new URL("http://"+host).hostname.toLowerCase().replace(/\.$/,"");
  }
  if(mode==="instance")return identity.instance_id;
  if(mode==="device")return String(registration||device);
  if(mode==="ip_cidr")return context?.ip||(/\//.test(registration)?(()=>{throw denied("IP_REQUIRED","网段校验需要实际受保护的 IP 地址")})():registration);
  return String(registration||"");
}

/** One public product configuration, one private state directory per installation. */
export class RuntimeLicense {
  #config;#client;#identity;#state;#stateFile;#verifier;#session=randomUUID().replaceAll("-","");#cache=new Map();#clock;
  static async open(config){const runtime=new RuntimeLicense(config);await runtime.#load();return runtime}
  constructor(config){
    if(!config?.stateDir||!config.productId||!config.productCode||!config.publicKeys)throw TypeError("ynlicense: stateDir, productId, productCode and pinned publicKeys are required");
    if(config.allowOffline&&!config.highWaterGuard)throw TypeError("ynlicense: offline use requires a persistent high-water guard");
    this.#config={cacheSeconds:0,allowOffline:false,target:{os:process.platform,arch:process.arch==="x64"?"amd64":process.arch,runtime:"node",channel:"stable",kind:"archive"},...config};this.#clock=config.clock||(()=>new Date());
    if(!Number.isFinite(this.#config.cacheSeconds)||this.#config.cacheSeconds<0||this.#config.cacheSeconds>60)throw TypeError("ynlicense: cacheSeconds must be between zero and 60");
    this.#client=new LicenseClient(config);this.#verifier=new OfflineVerifier({productCode:config.productCode,publicKeys:config.publicKeys,clock:this.#clock});
  }
  async #load(){
    await mkdir(this.#config.stateDir,{recursive:true,mode:0o700});
    const path=join(this.#config.stateDir,"instance.json");let identity=await readJSON(path);
    if(!identity){const created={instance_id:randomUUID().replaceAll("-",""),instance_secret:randomBytes(32).toString("hex"),device_id:randomUUID()};try{await writeJSON(path,created,true);identity=created}catch(e){if(e.code!=="EEXIST")throw e;identity=await readJSON(path)}}
    if(!/^[a-z0-9_-]{16,64}$/.test(identity?.instance_id)||! /^[a-f0-9]{64}$/.test(identity?.instance_secret))throw Error("ynlicense: invalid persisted installation identity");
    this.#identity=identity;this.#stateFile=join(this.#config.stateDir,"license.json");this.#state=await readJSON(this.#stateFile)||{};
    if(this.#state.seat?.borrowed)this.#session=this.#state.seat.session_id;
    if(this.#state.appid&&this.#state.appid!==this.#config.appKey)throw Error("ynlicense: private state belongs to another application");
  }
  async #save(){this.#state.appid=this.#config.appKey;await writeJSON(this.#stateFile,this.#state)}
  async #device(){return typeof this.#config.deviceId==="function"?String(await this.#config.deviceId()):String(this.#config.deviceId||this.#identity.device_id)}
  async #input(context={}){
    if(!this.#state.license_no)throw denied("LICENSE_REQUIRED","请先激活产品");
    const mode=this.#state.bind_mode;const identity=this.#state.legacy_attach?{}:this.#identity;return {license_no:this.#state.license_no,bind_mode:mode,bind_value:actualBinding(mode,this.#state.registration,context,this.#identity,await this.#device()),instance_id:identity.instance_id,instance_secret:identity.instance_secret,device_id:this.#state.legacy_attach?undefined:await this.#device(),seat_id:this.#state.seat?.id,session_id:this.#session,target:this.#config.target,current_version:this.#config.currentVersion};
  }
  #expected(input){return {licenseNo:input.license_no,bindMode:input.bind_mode,bindValue:input.bind_value,instanceId:input.instance_id,...(this.#state.seat?.borrowed?{seatId:this.#state.seat.id,sessionId:this.#state.seat.session_id}:{})}}
  async activate(cardCode,registration){
    const requirements=await this.#client.cardRequirements(cardCode);const terms=requirements.authorization_terms;
    const mode=terms.bind_mode;
    registration??=this.#config.registration;
    if(mode==="instance")registration=this.#identity.instance_id;
    if(mode==="device"&&!registration)registration=await this.#device();
    if(mode==="none")registration="none";
    const value=typeof registration==="object"?canonicalJSON(registration):String(registration??"");
    const result=await this.#client.activate({card_code:cardCode,bind_mode:mode,bind_value:value,instance_id:this.#identity.instance_id,instance_secret:this.#identity.instance_secret,device_id:await this.#device(),device_name:this.#config.label||""});
    if(!result.can_use)throw denied("USAGE_DENIED","当前授权不允许使用");
    this.#state={license_no:result.license_no,bind_mode:mode,registration:result.bind_value??value,rights:result};this.#cache.clear();await this.#save();return result;
  }
  async attach(licenseNo,bindMode,registration){
    const result=await this.#client.verify({license_no:licenseNo,bind_mode:bindMode,bind_value:registration});
    if(!result.can_use)throw denied("USAGE_DENIED","当前授权不允许使用");
    this.#state={legacy_attach:true,license_no:licenseNo,bind_mode:bindMode,registration,rights:result};this.#cache.clear();await this.#save();return result;
  }
  async securityDecision(context={}){
    let input=await this.#input(context);
    try{return await this.#client.securityDecision(input)}catch(e){
      if(e.code!=="SEAT_REQUIRED")throw e;
      await this.verify(context);input=await this.#input(context);return this.#client.securityDecision(input);
    }
  }
  async verify(context={}){
    const input=await this.#input(context);const key=input.bind_mode+"\x00"+input.bind_value;const now=this.#clock().getTime();const cached=this.#cache.get(key);
    if(cached&&now<cached.until&&performance.now()<cached.monotonicUntil){if(this.#config.highWaterGuard)await this.#config.highWaterGuard.checkAndUpdate(this.#clock());return {...cached.rights,can_update:cached.rights.can_update&&(!cached.rights.updates_expired_at||now<Date.parse(cached.rights.updates_expired_at))}};
    try{
      let result;
      try{result=await this.#client.verify(input)}catch(e){if(e.code!=="SEAT_REQUIRED")throw e;const seat=await this.#client.acquireSeat({...input,session_id:this.#session});this.#state.seat=seat;input.seat_id=seat.id;result=await this.#client.verify(input)}
      if(result.can_use!==true)throw denied("USAGE_DENIED","当前授权不允许使用");
      const claims=this.#verifier.verifyLicenseToken(result.license_token,this.#expected(input));
      // Rights and local cache deadlines come from verified claims, never from
      // unsigned envelope fields accompanying an otherwise valid token.
      result={...result,license_no:claims.license_no,can_use:claims.can_use,can_update:claims.can_update,license_mode:claims.license_mode,usage_expired_at:claims.usage_expired_at,updates_expired_at:claims.updates_expired_at,grace_until:claims.grace_until,authorization_terms:claims.authorization_terms};
      if(this.#config.highWaterGuard)await this.#config.highWaterGuard.checkAndUpdate(this.#clock());
      this.#state.rights=result;this.#state.offline_token=result.offline_token;await this.#save();
      const deadline=result.grace_until||result.usage_expired_at;let until=now+this.#config.cacheSeconds*1000;if(deadline)until=Math.min(until,Date.parse(deadline));if(this.#state.seat)until=Math.min(until,Date.parse(this.#state.seat.expires_at));
      this.#cache.set(key,{until,monotonicUntil:performance.now()+Math.max(0,until-now),rights:result});if(this.#cache.size>128)this.#cache.delete(this.#cache.keys().next().value);return result;
    }catch(e){
      this.#cache.clear();
      if(e instanceof APIError&&e.httpStatus<500||e.httpStatus===403){if(["LICENSE_REVOKED","LICENSE_EXPIRED","PRODUCT_DISABLED","INSTANCE_INVALID","SECURITY_USE_DENIED","USAGE_DENIED"].includes(e.code)){this.#state.offline_token=null;this.#state.seat=null;await this.#save()}throw e}
      if(!this.#config.allowOffline||!(e instanceof APIError&&e.httpStatus>=500||String(e.message).startsWith("ynlicense: request failed:")))throw e;
      await this.#config.highWaterGuard.checkAndUpdate(this.#clock());
      const token=this.#state.seat?.borrowed?this.#state.seat.token:this.#state.offline_token;
      const claims=this.#verifier.verifyOfflineToken(token,this.#expected(input));
      if(claims.can_use!==true)throw denied("USAGE_DENIED","离线凭证不允许使用");
      if(claims.authorization_terms?.max_concurrent_seats>0&&(!claims.seat_id||claims.seat_id!==this.#state.seat?.id))throw denied("SEAT_REQUIRED","离线使用需要有效的席位凭证");
      return {license_no:claims.license_no,license_mode:claims.license_mode||"subscription",can_use:true,can_update:false,usage_expired_at:claims.usage_expired_at,updates_expired_at:claims.updates_expired_at,authorization_terms:claims.authorization_terms,offline:true};
    }
  }
  async requireFeature(feature,context={}){const rights=await this.verify(context);if(!rights.authorization_terms?.features?.includes(feature))throw denied("FEATURE_DENIED","此授权未包含所需功能");return rights}
  async heartbeat(context={}){return this.#client.heartbeat(await this.#input(context))}
  async renew(cardCode,context={}){const result=await this.#client.renew({...await this.#input(context),renew_card_code:cardCode});this.#cache.clear();this.#state.rights=result;await this.#save();return result}
  async claimTicket(context={}){return this.#client.createClaimTicket(await this.#input(context))}
  async borrowSeat(days,context={}){const seat=await this.#client.acquireSeat({...await this.#input(context),session_id:this.#session,borrow_days:days});this.#state.seat=seat;this.#cache.clear();await this.#save();return seat}
  async checkUpdate(context={}){return this.#client.checkUpdate(await this.#input(context))}
  async downloadUpdate(release,context={}){const key=this.#config.publicKeys[String(release.key_version)];if(!key)throw Error("ynlicense: unknown signing key version");return this.#client.downloadUpdate(release,await this.#input(context),{publicKeyPem:key,productId:this.#config.productId})}
  async authorizeUpdate(releaseID,context={}){return this.#client.authorizeUpdate(releaseID,await this.#input(context))}
  async releaseSeat(context={}){if(this.#state.seat&&!this.#state.seat.borrowed){await this.#client.releaseSeat(await this.#input(context));this.#state.seat=null;this.#cache.clear();await this.#save()}}
}
