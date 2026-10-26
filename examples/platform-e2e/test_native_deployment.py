#!/usr/bin/env python3
"""Extract independent release archives; run real Go + Nginx authorization/update flows."""
import argparse
import datetime
import hashlib
import importlib.util
import io
import json
import os
import pathlib
import secrets
import shutil
import sqlite3
import subprocess
import sys
import tarfile
import tempfile
import urllib.error
import urllib.request
import uuid
import zipfile

REPO=pathlib.Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('demo_http',REPO/'examples/update-demo/test_e2e.py');http=importlib.util.module_from_spec(spec);spec.loader.exec_module(http)
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,*args):return None

def raw(base,path,method='GET',data=None,headers=None):
    req=urllib.request.Request(base+path,data=data,headers=headers or {},method=method)
    try:response=urllib.request.build_opener(NoRedirect).open(req,timeout=20)
    except urllib.error.HTTPError as error:response=error
    with response:return response.status,dict(response.headers),response.read()

def extract(path,destination):
    with tarfile.open(path) as tar:
        for item in tar.getmembers():
            assert not item.issym() and not item.islnk() and '..' not in pathlib.PurePosixPath(item.name).parts and not item.name.startswith('/')
            assert all(part not in {'secrets','backups','node_modules','.git','__pycache__'} for part in pathlib.PurePosixPath(item.name).parts)
            assert not item.name.endswith(('.install-token','.age','.sqlite','.db','.key','.pem'))
        previous_umask=os.umask(0o022)
        try:tar.extractall(destination,filter='data')
        finally:os.umask(previous_umask)
        names={pathlib.PurePosixPath(m.name).parts[0] for m in tar.getmembers()}
    assert len(names)==1;return destination/names.pop()

def main():
    p=argparse.ArgumentParser();p.add_argument('--backend',required=True,type=pathlib.Path);p.add_argument('--frontend',required=True,type=pathlib.Path);p.add_argument('--artifacts',required=True,type=pathlib.Path);p.add_argument('--screenshots',action='store_true');p.add_argument('--chromium',default='/usr/bin/chromium');p.add_argument('--nginx-image',default='nginx:stable-alpine');p.add_argument('--supervisor',action='store_true');args=p.parse_args();args.artifacts.mkdir(parents=True,exist_ok=True)
    report={'result':'running','checks':[]};process=browser=pw=None;container='yunnuo-native-'+uuid.uuid4().hex[:10]
    with tempfile.TemporaryDirectory(prefix='native-deployment-') as temporary,(args.artifacts/'server.log').open('w') as log:
        temp=pathlib.Path(temporary);temp.chmod(0o755)
        try:
            backend=extract(args.backend,temp);web=extract(args.frontend,temp)
            info=json.loads((backend/'BUILD_INFO.json').read_text());assert info['arch']=='amd64' and not (backend/'frontend').exists() and not (web/'backend/yunnuo-server').exists()
            for name in ['yunnuo-server','yunnuo-ops','yunnuo-healthcheck']:
                data=(backend/'backend'/name).read_bytes();assert data[:4]==b'\x7fELF' and (backend/'backend'/name).stat().st_mode&0o111
            assert (web/'frontend').stat().st_mode&0o755==0o755
            report['checks'].append('Independent backend/frontend archives extract with public static permissions, executables, migrations, SDK sources and no private state')
            subprocess.run(['sha256sum','-c','FILES.sha256'],cwd=backend,check=True,capture_output=True,text=True)
            go_port,nginx_port=http.free_port(),http.free_port();direct=f'http://127.0.0.1:{go_port}';base=f'http://127.0.0.1:{nginx_port}';state=temp/'private-state';frontstate=temp/'nginx-state'
            backend_config=[str(backend/'deploy/native/configure.py'),'--state-dir',str(state),'--backend-only','--port',str(go_port)]
            subprocess.run(['python3',*backend_config],check=True,capture_output=True,text=True)
            front_config=[str(web/'deploy/native/configure.py'),'--state-dir',str(frontstate),'--frontend-only','--upstream',f'127.0.0.1:{go_port}','--domain','license.example.com','--nginx-listen',f'127.0.0.1:{nginx_port}']
            subprocess.run(['python3',*front_config],check=True,capture_output=True,text=True)
            env={k:v for k,v in os.environ.items() if not k.startswith('YN_')};env['YN_RUNTIME_FILE']=str(state/'runtime.env')
            start=lambda:subprocess.Popen(['bash',str(backend/'deploy/native/start.sh')],env=env,stdout=log,stderr=log)
            process=start();http.wait_ready(direct,'/readyz',process)
            assert raw(direct,'/install')[0]==404 and raw(direct,'/assets/theme-init.js')[0]==404
            nginx_conf=temp/'nginx.conf';nginx_conf.write_text('pid /tmp/yunnuo-nginx.pid;\nevents {}\nhttp { include /etc/nginx/mime.types; access_log /dev/stdout; error_log /dev/stderr; include "'+str(frontstate/'deploy/nginx-server.conf')+'"; }\n')
            subprocess.run(['docker','run','-d','--name',container,'--network','host','-v',str(temp)+':'+str(temp)+':ro',args.nginx_image,'nginx','-g','daemon off;','-c',str(nginx_conf)],check=True,capture_output=True,text=True)
            http.wait_ready(base,'/healthz',process)
            for path in ['/','/admin','/agent-console/','/user-console/','/developer-console/','/recover']:
                status,headers,_=raw(base,path);assert status==307 and headers['Location']=='/install',(path,status)
            status,headers,body=raw(base,'/install');assert status==200 and '安装授权平台'.encode() in body
            assert 'Content-Security-Policy' in headers and raw(base,'/assets/theme-init.js')[0]==200
            assert raw(base,'/v1/setup/initialize','POST',b'{}',{'Content-Type':'application/json'})[0]==403
            report['checks'].append('Nginx serves HTML/assets while Go static serving is disabled; every first-run business page redirects to /install and ownership token is required')
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                pw=sync_playwright().start();browser=pw.chromium.launch(executable_path=None if args.chromium=='bundled' else args.chromium,headless=True,args=['--no-sandbox']);context=browser.new_context(viewport={'width':1440,'height':1000},locale='zh-CN');page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.goto(base);page.get_by_label('安装令牌',exact=True).wait_for();page.screenshot(path=str(args.artifacts/'nginx-first-install.png'),full_page=True,animations='disabled')
            password=secrets.token_hex(24);dsn='file:'+str(state/'bootstrap.sqlite')+'?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)';http.initialize(base,'admin',password,dsn)
            token=http.api(base,'/admin/login',{'username':'admin','password':password})['access_token']
            for path in ['/','/admin','/agent-console/','/user-console/','/developer-console/','/recover']:
                assert raw(base,path)[0]==200,path
            for path in ['/backend/yunnuo-server','/runtime.env','/.env','/admin-console/index.html']:
                assert raw(base,path)[0]==404,path
            report['checks'].append('Installation completes through the same-origin Nginx API, all role entries work, and private/backend files and raw admin HTML are inaccessible')
            product=http.api(base,'/admin/products',{'name':'Nginx Ebox','bind_mode':'domain','license_mode':'perpetual','update_duration_days':7},token)
            batch=http.api(base,'/admin/card-batches',{'product_id':product['id'],'name':'Release package acceptance','quantity':2,'duration_days':30},token)
            credentials={'app_key':product['id'],'bind_mode':'domain','bind_value':'ebox.example.com'};license=http.api(base,'/v1/licenses/activate',{**credentials,'card_code':batch['codes'][0]});credentials['license_no']=license['license_no']
            prefix='/admin/products/'+product['id'];http.api(base,prefix+'/updates',{'enabled':True},token)
            archive=io.BytesIO()
            with zipfile.ZipFile(archive,'w') as package:package.writestr('version.txt','Ebox 1.0.0')
            package=archive.getvalue();boundary=uuid.uuid4().hex;parts=[f'--{boundary}\r\nContent-Disposition: form-data; name="version"\r\n\r\n1.0.0\r\n'.encode(),f'--{boundary}\r\nContent-Disposition: form-data; name="package"; filename="ebox.zip"\r\nContent-Type: application/zip\r\n\r\n'.encode()+package+f'\r\n--{boundary}--\r\n'.encode()]
            release=http.api(base,prefix+'/releases',b''.join(parts),token,'multipart/form-data; boundary='+boundary);http.api(base,prefix+'/releases/'+release['id']+'/publish',{},token)
            assert http.api(base,'/v1/updates/check',{**credentials,'current_version':'0.0.0'})['update_available']
            download='/v1/updates/'+release['id']+'/download';authorize='/v1/updates/'+release['id']+'/authorize'
            assert hashlib.sha256(http.http(base,download,credentials)).hexdigest()==release['sha256'];assert http.api(base,authorize,credentials)['authorized']
            assert raw(base,'/admin/products','POST',headers={'Content-Length':'300000000','Content-Type':'application/json'})[0]==413
            report['checks'].append('Packaged SDK/migrations support real activation, multipart upload, publish, authenticated checksum download and install authorization through Nginx; oversized uploads are rejected')
            with sqlite3.connect(state/'bootstrap.sqlite') as db:
                expiry=(datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(seconds=1)).isoformat().replace('+00:00','Z');db.execute('UPDATE license_entitlements SET updates_expired_at=? WHERE license_id=(SELECT id FROM licenses WHERE license_no=?)',(expiry,license['license_no']))
            verified=http.api(base,'/v1/licenses/verify',credentials);assert verified['can_use'] and not verified['can_update']
            assert http.http(base,download,credentials,expected=403)['error']['code']=='UPDATES_EXPIRED';http.http(base,authorize,credentials,expected=403)
            renewed=http.api(base,'/v1/licenses/renew',{**credentials,'renew_card_code':batch['codes'][1]});replayed=http.api(base,'/v1/licenses/renew',{**credentials,'renew_card_code':batch['codes'][1]});assert renewed['updates_expired_at']==replayed['updates_expired_at'] and renewed['usage_expired_at'] is None
            report['checks'].append('Expired updates remain denied through the proxy while perpetual usage continues; renewing restores updates and a retry does not add another term')
            config_path=state/'database.json';config=json.loads(config_path.read_text());keys={k:config[k] for k in ['data_key','card_pepper']};config['admin_path']='/private/control';config_path.write_text(json.dumps(config));config_path.chmod(0o600)
            process.terminate();process.wait(timeout=30);subprocess.run(['python3',*backend_config],check=True,capture_output=True,text=True);subprocess.run(['python3',str(web/'deploy/native/configure.py'),'--state-dir',str(frontstate),'--frontend-only'],check=True,capture_output=True,text=True)
            assert all(json.loads(config_path.read_text())[k]==v for k,v in keys.items());assert f'127.0.0.1:{nginx_port}' in (frontstate/'deploy/nginx-server.conf').read_text()
            process=start();http.wait_ready(direct,'/readyz',process)
            assert raw(base,'/admin')[0]==404 and raw(base,'/private/control')[0]==200;assert http.api(base,'/admin/profile',token=token)['username']=='admin';assert hashlib.sha256(http.http(base,download,credentials)).hexdigest()==release['sha256']
            report['checks'].append('Reconfiguration preserves keys, ports and hostnames; restarting Go alone changes the admin page path immediately, preserving data, sessions and packages')
            if args.screenshots:
                page.goto(base+'/private/control#operations');page.get_by_label('账号',exact=True).fill('admin');page.get_by_label('密码',exact=True).fill(password);page.get_by_role('button',name='安全登录',exact=True).click();page.get_by_role('heading',name='运行与容量',level=3,exact=True).wait_for();page.screenshot(path=str(args.artifacts/'nginx-admin-desktop.png'),full_page=True,animations='disabled');page.emulate_media(color_scheme='dark');page.locator("html[data-theme='dark']").wait_for(state='attached');page.set_viewport_size({'width':390,'height':844});assert page.evaluate('document.documentElement.scrollWidth<=innerWidth+1');page.screenshot(path=str(args.artifacts/'nginx-admin-mobile-dark.png'),animations='disabled');assert not errors
            if shutil.which('systemd-analyze'):
                verified_unit=subprocess.run(['systemd-analyze','verify',str(state/'deploy/yunnuo-license.service')],capture_output=True,text=True)
                assert verified_unit.returncode==0,verified_unit.stderr
            # Exercise the template without auth_request for BaoTa Nginx builds
            # where that optional module is absent. Existing state stays intact.
            process.terminate();process.wait(timeout=30)
            subprocess.run(['python3',str(backend/'deploy/native/configure.py'),'--state-dir',str(state),'--mode','compatible','--frontend-dir',str(web/'frontend'),'--nginx-listen',f'127.0.0.1:{nginx_port}'],check=True,capture_output=True,text=True)
            nginx_conf.write_text('pid /tmp/yunnuo-nginx.pid;\nevents {}\nhttp { include /etc/nginx/mime.types; access_log /dev/stdout; error_log /dev/stderr; include "'+str(state/'deploy/nginx-server.conf')+'"; }\n')
            process=start();http.wait_ready(direct,'/readyz',process)
            subprocess.run(['docker','exec',container,'nginx','-s','reload','-c',str(nginx_conf)],check=True,capture_output=True,text=True)
            assert raw(direct,'/install')[0]==200 and raw(base,'/private/control')[0]==200 and raw(base,'/admin')[0]==404
            assert hashlib.sha256(http.http(base,download,credentials)).hexdigest()==release['sha256']
            report['checks'].append('Compatibility mode runs without auth_request directives, preserving the custom admin entry and existing authorization/update data')
            if args.supervisor:
                import supervisor
                process.terminate();process.wait(timeout=30)
                supervisor_conf=state/'supervisord.conf';supervisor_conf.write_text('[supervisord]\nnodaemon=true\nlogfile='+str(state/'logs/supervisord.log')+'\npidfile='+str(state/'supervisord.pid')+'\nchildlogdir='+str(state/'logs')+'\n'+(state/'deploy/supervisor.conf').read_text())
                process=subprocess.Popen([sys.executable,'-m','supervisor.supervisord','-n','-c',str(supervisor_conf)],env=env,stdout=log,stderr=log)
                http.wait_ready(direct,'/readyz',process)
                assert http.api(base,'/v1/licenses/verify',credentials)['can_use'] and raw(base,'/private/control')[0]==200
                report['checks'].append('The generated Supervisor configuration starts and supervises the packaged Go binary with private runtime.env, persistent database and custom page routing')
            report.update(result='passed',split_backend_static_serving=False,compatibility_verified=True,supervisor_verified=args.supervisor,frontend='nginx',database='sqlite',build_info=info)
        finally:
            if browser:browser.close()
            if pw:pw.stop()
            subprocess.run(['docker','logs',container],stdout=(args.artifacts/'nginx.log').open('w'),stderr=subprocess.STDOUT)
            subprocess.run(['docker','rm','-f',container],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            if process and process.poll() is None:process.terminate();process.wait(timeout=30)
            (args.artifacts/'report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
    print(json.dumps({'result':report['result'],'checks':len(report['checks'])}))
if __name__=='__main__':main()
