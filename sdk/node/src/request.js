const denied=(code,message)=>Object.assign(new Error(message),{code,httpStatus:403});
export function requestHostname(request,{trustedProxies=[]}={}){
  if(!request?.headers)throw denied("HOST_REQUIRED","实际 HTTP 请求缺失，不能使用配置域名代替访问域名");
  let host=request.headers.host;const remote=String(request.socket?.remoteAddress||"").replace(/^::ffff:/,"");
  if(trustedProxies.includes(remote)&&request.headers["x-forwarded-host"])host=request.headers["x-forwarded-host"];
  if(typeof host!=="string"||/[\s,/@?#\\]/.test(host))throw denied("HOST_INVALID","访问域名不正确");
  let url;try{url=new URL("http://"+host)}catch{throw denied("HOST_INVALID","访问域名不正确")}
  if(url.username||url.password||url.pathname!=="/")throw denied("HOST_INVALID","访问域名不正确");
  return url.hostname.toLowerCase().replace(/\.$/,"");
}
