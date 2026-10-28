"""Example customer website using the same generic client.py distribution.
Loopback by default. Put behind your own HTTPS proxy for remote use.
YN_PRODUCT_DIR points to the demo installer's persistent site directory.
Set a unique YN_PRODUCT_ADMIN_PASSWORD to enable the private /activate form;
it is the customer's local administrator password, never a vendor AppSecret.
"""
import argparse,base64,hmac,html,json,os,pathlib,sqlite3,subprocess,sys,urllib.error,urllib.parse,urllib.request
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
APP_ID='@@APP_ID@@'
LICENSE_URL=os.environ.get('YN_LICENSE_URL','@@LICENSE_URL@@').rstrip('/')
PRODUCT=pathlib.Path(os.environ.get('YN_PRODUCT_DIR','./product-data')).resolve()
class Website(BaseHTTPRequestHandler):
 def log_message(self,*args):pass  # Never log form bodies, Basic credentials or cards.
 def respond(self,status,body,kind='text/html; charset=utf-8',extra=None):
  raw=body if isinstance(body,bytes) else body.encode();self.send_response(status);self.send_header('Content-Type',kind);self.send_header('Cache-Control','no-store');self.send_header('Content-Length',str(len(raw)));self.send_header('X-Content-Type-Options','nosniff')
  for name,value in (extra or {}).items():self.send_header(name,value)
  self.end_headers();self.wfile.write(raw)
 def admin(self):
  password=os.environ.get('YN_PRODUCT_ADMIN_PASSWORD','')
  if not password:self.respond(403,'Private activation is disabled. Configure a unique local product administrator password or activate with the CLI.');return False
  try:provided=base64.b64decode(self.headers.get('Authorization','').removeprefix('Basic '),validate=True)
  except ValueError:provided=b''
  expected=(os.environ.get('YN_PRODUCT_ADMIN_USERNAME','owner')+':'+password).encode()
  if not hmac.compare_digest(provided,expected):self.respond(401,'Authentication required',extra={'WWW-Authenticate':'Basic realm="Private product activation", charset="UTF-8"'});return False
  return True
 def license_command(self,operation,extra=None):
  actual={}
  if os.environ.get('YN_BINDING_TYPE','@@DEFAULT_BINDING@@')=='domain':
   # Use the real request Host. Never accept an arbitrary X-Forwarded-Host;
   # configure the product's trusted reverse proxy to preserve Host.
   hostname=urllib.parse.urlsplit('//'+self.headers.get('Host','')).hostname
   if not hostname:raise ValueError('Missing protected hostname')
   actual['YN_BINDING_VALUE']=hostname
  return subprocess.run([sys.executable,str(pathlib.Path(__file__).with_name('client.py')),operation],env={**os.environ,'YN_FORM':'web',**actual,**(extra or {})},capture_output=True,text=True,timeout=30)
 def authorized(self):
  result=self.license_command('verify')
  if result.returncode==0:
   try:
    if json.loads(result.stdout).get('valid') is True:return True
   except ValueError:pass
  # Do not convert an HTTP refusal into offline authorization. client.py has
  # already applied the explicit signed network-grace policy.
  try:
   with urllib.request.urlopen(LICENSE_URL+'/v2/applications/'+APP_ID+'/block-page',timeout=5) as response:body=response.read(128*1024)
  except (OSError,urllib.error.URLError):body=b'<!doctype html><meta charset="utf-8"><h1>Authorization unavailable</h1><p>Contact the software provider to verify your purchase.</p>'
  self.respond(403,body,extra={'Content-Security-Policy':"default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'"});return False
 def do_GET(self):
  path=urllib.parse.urlsplit(self.path).path
  if path=='/activate':
   if self.admin():self.respond(200,'<!doctype html><meta charset="utf-8"><h1>产品激活</h1><form method="post"><label>购买的卡密<input name="card_code" type="password" required autocomplete="off"></label><button>激活此安装</button></form>',extra={'Content-Security-Policy':"default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'"})
   return
  if path=='/healthz':
   try:
    metadata=json.loads((PRODUCT/'current/release.json').read_text())
    with sqlite3.connect((PRODUCT/'site.sqlite').as_uri()+'?mode=ro',uri=True) as db:schema=db.execute('PRAGMA user_version').fetchone()[0]
    valid=metadata.get('healthy',True) and schema==metadata['schema_version'];self.respond(200 if valid else 503,json.dumps({'healthy':bool(valid),'version':metadata['version']}),'application/json')
   except (OSError,ValueError,sqlite3.Error):self.respond(503,'{"healthy":false}','application/json')
   return
  if path not in ('/','/api/status'):self.respond(404,'Not found');return
  if not self.authorized():return
  if path=='/api/status':
   metadata=json.loads((PRODUCT/'current/release.json').read_text())
   with sqlite3.connect((PRODUCT/'site.sqlite').as_uri()+'?mode=ro',uri=True) as db:notes=db.execute('SELECT id,title FROM notes ORDER BY id').fetchall();schema=db.execute('PRAGMA user_version').fetchone()[0]
   self.respond(200,json.dumps({'version':metadata['version'],'schema_version':schema,'notes':[{'id':r[0],'title':r[1]} for r in notes]},ensure_ascii=False),'application/json');return
  index=PRODUCT/'current/index.html'
  self.respond(200,index.read_bytes() if index.exists() else '<!doctype html><meta charset="utf-8"><h1>授权验证通过</h1><p>同一发行包由客户自己的卡密激活。</p>')
 def do_POST(self):
  if urllib.parse.urlsplit(self.path).path!='/activate':self.respond(404,'Not found');return
  if not self.admin():return
  try:length=int(self.headers.get('Content-Length','0'))
  except ValueError:self.respond(400,'Invalid request');return
  if length<1 or length>4096:self.respond(413,'Activation form too large');return
  form=urllib.parse.parse_qs(self.rfile.read(length).decode());card=form.get('card_code',[''])[0].strip()
  if not card:self.respond(400,'Enter the purchased card');return
  result=self.license_command('activate',{'YN_CARD':card})
  # Never echo submitted cards, server tokens or detailed request bodies.
  self.respond(200 if result.returncode==0 else 403,'<!doctype html><meta charset="utf-8"><h1>'+('激活成功' if result.returncode==0 else '激活未通过，请核对套餐、绑定与有效期')+'</h1><a href="/">返回产品</a>')
if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('--bind',default='127.0.0.1');parser.add_argument('--port',type=int,default=8088);args=parser.parse_args();ThreadingHTTPServer((args.bind,args.port),Website).serve_forever()
