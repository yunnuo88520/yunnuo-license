import {readFile} from "node:fs/promises";
import {createServer} from "node:http";
import {RuntimeLicense} from "../../sdk/node/src/index.js";
const [configPath,stateDir,command="verify",value="",registration="",host="",ip=""]=process.argv.slice(2);
const config=JSON.parse(await readFile(configPath,"utf8"));
const runtime=await RuntimeLicense.open({baseUrl:config.base_url,appKey:config.app_key,productId:config.product_id,productCode:config.product_code,publicKeys:config.public_keys,stateDir,currentVersion:config.current_version||"1.0.0",target:config.target,deviceId:config.device_id,cacheSeconds:0});
const context={...(host?{host}:{}),...(ip?{ip}:{})};const summary=r=>({can_use:r.can_use,can_update:r.can_update,license_no:r.license_no,authorization_terms:r.authorization_terms,offline:r.offline});
try{
 let result;
 if(command==="activate")result=summary(await runtime.activate(value,registration||undefined));
 else if(command==="verify")result=summary(await runtime.verify(context));
 else if(command==="feature")result=summary(await runtime.requireFeature(value,context));
 else if(command==="claim")result=await runtime.claimTicket(context);
 else if(command==="updates")result=await runtime.checkUpdate(context);
 else if(command==="serve"){
  const server=createServer(async(request,response)=>{if(request.url==="/healthz"){response.end("ok");return}try{const rights=await runtime.verify({request});response.writeHead(200,{"Content-Type":"application/json","Cache-Control":"no-store"});response.end(JSON.stringify(summary(rights)))}catch(error){response.writeHead(error.httpStatus||503,{"Content-Type":"application/json"});response.end(JSON.stringify({code:error.code||"SERVICE_UNAVAILABLE",can_use:false}))}});
  server.listen(Number(process.env.DEMO_PORT||3000),"127.0.0.1");
  for(const signal of ["SIGTERM","SIGINT"])process.on(signal,()=>server.close(()=>process.exit(0)));
 }else throw Error("unknown command");
 if(result)console.log(JSON.stringify(result));
}catch(error){console.log(JSON.stringify({success:false,code:error.code||"CLIENT_ERROR",message:error.message}));process.exitCode=1}
