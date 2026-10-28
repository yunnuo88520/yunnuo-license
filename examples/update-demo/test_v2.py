#!/usr/bin/env python3
"""Real generated Python client, signed updates, data migration and rollback.
Uses an isolated database; direct deadline edits are test fixtures only.
"""
import argparse,base64,datetime,hashlib,io,json,os,pathlib,secrets,sqlite3,subprocess,sys,tempfile,zipfile
import test_e2e as http
REPO=pathlib.Path(__file__).resolve().parents[2]

def package(version,schema,healthy=True):
 out=io.BytesIO()
 with zipfile.ZipFile(out,'w') as z:
  z.writestr('index.html',f'<h1>Ebox {version}</h1>');z.writestr('release.json',json.dumps({'version':version,'schema_version':schema,'healthy':healthy}))
 return out.getvalue()

def upload(base,token,app,version,raw,target=None):
 boundary='yn-'+secrets.token_hex(16)
 parts=[]
 for name,value in [('version',version),('target',json.dumps(target or {'os':'any','arch':'any','runtime':'any','channel':'stable','kind':'archive'}))]:
  parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode())
 parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="product.zip"\r\nContent-Type: application/zip\r\n\r\n'.encode()+raw+f'\r\n--{boundary}--\r\n'.encode())
 result=http.http(base,'/v2/console/applications/'+app+'/releases/upload',b''.join(parts),token=token,content_type='multipart/form-data; boundary='+boundary)
 if 'data' in result:result=result['data']
 http.api(base,f'/v2/console/applications/{app}/releases/{result["id"]}/status',{'status':'published','reason':'verified update fixture'},token)
 return result

def main():
 parser=argparse.ArgumentParser();parser.add_argument('--server',required=True,type=pathlib.Path);parser.add_argument('--artifacts',required=True,type=pathlib.Path);args=parser.parse_args();args.artifacts.mkdir(parents=True,exist_ok=True)
 report={'result':'running','cases':[]};process=None;sites=[]
 def passed(name):report['cases'].append(name);print(name,flush=True)
 with tempfile.TemporaryDirectory(prefix='v2-update-') as temp,(args.artifacts/'server.log').open('w') as log:
  folder=pathlib.Path(temp);database=folder/'license.sqlite';port=http.free_port();base=f'http://127.0.0.1:{port}';password=secrets.token_hex(24)
  env=os.environ.copy();env.update(YN_ADDR=f'127.0.0.1:{port}',YN_DB_DRIVER='sqlite',YN_DB=f'file:{database}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)',YN_DB_CONFIG_FILE=str(folder/'database.json'),YN_RELEASE_DIR=str(folder/'packages'),YN_CLIENT_WRITE_RATE_LIMIT='2000',YN_CLIENT_READ_RATE_LIMIT='10000')
  try:
   process=subprocess.Popen([str(args.server.resolve())],cwd=REPO/'backend',env=env,stdout=log,stderr=log);http.wait_ready(base,'/healthz',process);http.initialize(base,'admin',password,env['YN_DB']);token=http.api(base,'/admin/login',{'username':'admin','password':password})['access_token']
   for model in ('subscription','perpetual'):
    app=http.api(base,'/v2/console/applications',{'name':'Ebox '+model,'allowed_models':[model],'allowed_bindings':['installation'],'forms':['web','container','gui'],'updates_enabled':True},token)['app_id'];prefix='/v2/console/applications/'+app
    plan=http.api(base,prefix+'/plans',{'name':'Standard','terms':{'model':model,'binding_types':['installation'],'forms':['web','container','gui'],'duration_days':30 if model=='subscription' else 0,'update_duration_days':30}},token)
    sale=http.api(base,prefix+'/orders',{'plan_id':plan['id'],'quantity':1,'distribute_cards':True,'confirmed':True,'reason':'manual receipt verified'},token)
    kit=http.api(base,prefix+'/integration?language=python&binding_type=installation&form=web',token=token);project=folder/model;project.mkdir()
    for file in kit['files']:(project/file['name']).write_text(file['content'])
    deps=folder/'dependencies'
    if not deps.exists():subprocess.run([sys.executable,'-m','pip','install','--quiet','--target',str(deps),'-r',str(project/'requirements.txt')],check=True,capture_output=True)
    customer_env=os.environ.copy();customer_env.update(PYTHONPATH=str(deps),YN_LICENSE_URL=base,YN_STATE_DIR=str(project/'state'),YN_CARD=sale['cards'][0]['code'],YN_FORM='web',YN_BINDING_TYPE='installation',YN_CURRENT_VERSION='1.0.0')
    def client(file,operation,ok=True):
     result=subprocess.run([sys.executable,str(project/file),operation],cwd=project,env=customer_env,capture_output=True,text=True)
     if ok and result.returncode:raise AssertionError(result.stderr[-2000:])
     if not ok:
      assert result.returncode!=0,'expected update refusal';return result.stderr
     return json.loads(result.stdout)
    def installer(action,*rest):
     result=subprocess.run([sys.executable,str(REPO/'examples/update-demo/installer.py'),action,str(project/'site'),*map(str,rest)],capture_output=True,text=True,check=True);return json.loads(result.stdout)
    initial=project/'initial.zip';initial.write_bytes(package('1.0.0',1));installer('init',initial);before=installer('status');assert before['schema_version']==1
    website_port=http.free_port();website=f'http://127.0.0.1:{website_port}';local_password=secrets.token_hex(24);web_env={k:v for k,v in customer_env.items() if k!='YN_CARD'};web_env.update(YN_PRODUCT_DIR=str(project/'site'),YN_PRODUCT_ADMIN_PASSWORD=local_password)
    site=subprocess.Popen([sys.executable,str(project/'web.py'),'--port',str(website_port)],cwd=project,env=web_env,stdout=log,stderr=log);sites.append(site);http.wait_ready(website,'/healthz',site)
    http.http(website,'/activate',expected=401)
    authorization='Basic '+base64.b64encode(('owner:'+local_password).encode()).decode()
    form=('card_code='+customer_env['YN_CARD']).encode();activated=http.http(website,'/activate',form,content_type='application/x-www-form-urlencoded',extra_headers={'Authorization':authorization});assert '激活成功' in activated.decode();customer_env.pop('YN_CARD');assert client('client.py','verify')['valid'];assert b'1.0.0' in http.http(website,'/');passed(model+': real website form activates with its own administrator password; refresh needs no card')
    upload(base,token,app,'2.0.0',package('2.0.0',2));assert client('update.py','check')['version']=='2.0.0';download=client('update.py','download');path=pathlib.Path(download['path']);assert hashlib.sha256(path.read_bytes()).hexdigest()==download['sha256'];passed(model+': actual HTTP download, JWT target and SHA-256 verified')
    license_id=sale['cards'][0]['license_id']
    with sqlite3.connect(database) as db:original=json.loads(db.execute('SELECT data_json FROM application_licenses WHERE application_id=? AND id=?',(app,license_id)).fetchone()[0])
    def deadline(expired):
     value=json.loads(json.dumps(original));record=value['record']
     if expired:
      record['updates_until']=(datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(seconds=1)).isoformat().replace('+00:00','Z')
      if model=='subscription':record['expires_at']=record['updates_until']
     with sqlite3.connect(database) as db:db.execute('UPDATE application_licenses SET data_json=?,expires_at=? WHERE application_id=? AND id=?',(json.dumps(value),record['expires_at'],app,license_id))
    deadline(True)
    assert 'EXPIRED' in client('update.py','check',False);assert 'EXPIRED' in client('update.py','download',False);assert 'EXPIRED' in client('update.py','authorize',False);assert installer('status')==before;passed(model+': expired check, download and preinstall authorization denied; data unchanged')
    if model=='perpetual':
     assert client('client.py','local')['valid'];assert b'1.0.0' in http.http(website,'/');passed('perpetual: update-only refusal preserves both cached local usage and the real website')
    else:
     assert 'LICENSE_EXPIRED' in client('client.py','verify',False);assert '授权安全拦截' in http.http(website,'/',expected=403).decode();passed('subscription: real website returns the configured block page after usage expiry')
    deadline(False);assert client('client.py','verify')['valid'];passed(model+': restored rights resume without card or reinstallation')
    # Corrupt a downloaded package before installation; no API permit can fix it.
    original_package=path.read_bytes();path.write_bytes(original_package+b'changed');assert 'modified' in client('update.py','authorize',False);path.write_bytes(original_package);passed(model+': tampered downloaded package refused before install')
    permit=client('update.py','authorize');assert permit['authorized'];installer('apply',permit['path'],permit['version']);after=installer('status');assert after['schema_version']==2 and after['notes']==before['notes'];installer('commit');customer_env['YN_CURRENT_VERSION']='2.0.0';assert http.http(website,'/api/status')['version']=='2.0.0';passed(model+': authorized upgrade migrates SQLite and preserves customer data')
    upload(base,token,app,'3.0.0',package('3.0.0',3,healthy=False));client('update.py','check');client('update.py','download');permit=client('update.py','authorize');installer('apply',permit['path'],permit['version']);assert installer('status')['schema_version']==3
    # The candidate website itself fails its health endpoint after migration.
    assert http.http(website,'/healthz',expected=503)['healthy'] is False
    installer('rollback');assert http.http(website,'/api/status')['version']=='2.0.0';assert installer('status')==after;assert not (project/'site/pending.json').exists();passed(model+': failed health check rolls back files, schema and data')
   report['result']='passed'
  except Exception as e:report.update(result='failed',error=str(e));raise
  finally:
   for site in sites:
    site.terminate();site.wait(timeout=10)
   if process is not None:process.terminate();process.wait(timeout=10)
   (args.artifacts/'report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
if __name__=='__main__':main()
