#!/usr/bin/env python3
"""Use the root Compose file and generated env with a disposable MySQL volume."""
import argparse
import importlib.util
import json
import os
import pathlib
import secrets
import subprocess
import tempfile
import time
import uuid
from v2_product import Product

REPO=pathlib.Path(__file__).resolve().parents[2]
def load(name,path):
    spec=importlib.util.spec_from_file_location(name,path);module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module
http=load('http_helpers',REPO/'examples/update-demo/test_e2e.py');configure=load('configure_compose',REPO/'scripts/configure-compose.py');native=load('native_http',REPO/'examples/platform-e2e/test_native_deployment.py')

def main():
    p=argparse.ArgumentParser();p.add_argument('--image',required=True);p.add_argument('--artifacts',required=True,type=pathlib.Path);args=p.parse_args();args.artifacts.mkdir(parents=True,exist_ok=True);report={'result':'running','checks':[]};name='yunnuo-compose-'+uuid.uuid4().hex[:10];volume=name+'-data'
    with tempfile.TemporaryDirectory(prefix='compose-deployment-') as temporary:
        envfile=pathlib.Path(temporary)/'.env';port=http.free_port();base=f'http://127.0.0.1:{port}';command=['docker','compose','--project-name',name,'--env-file',str(envfile),'-f',str(REPO/'compose.yml')]
        try:
            generated=['python3',str(REPO/'scripts/configure-compose.py'),'--output',str(envfile),'--port',str(port),'--image',args.image,'--domain','license.example.com','--config-dir',str(pathlib.Path(temporary)/'config')]
            subprocess.run(generated,cwd=REPO,check=True,capture_output=True,text=True);values=configure.read_env(envfile);values['YN_DATA_VOLUME']=volume;configure.write_env(envfile,values)
            secrets_before={key:values[key] for key in ['MYSQL_PASSWORD','MYSQL_ROOT_PASSWORD','YN_METRICS_TOKEN']}
            subprocess.run(generated,cwd=REPO,check=True,capture_output=True,text=True);values=configure.read_env(envfile);assert all(values[k]==v for k,v in secrets_before.items()) and values['YN_DATA_VOLUME']==volume and envfile.stat().st_mode&0o077==0
            rendered=json.loads(subprocess.check_output([*command,'config','--format','json'],text=True));assert rendered['volumes']['data']['name']==volume;assert rendered['services']['license']['ports'][0]['host_ip']=='127.0.0.1'
            production=json.loads(subprocess.check_output(['docker','compose','--env-file',str(envfile),'-f',str(REPO/'deploy/compose.production.yml'),'config','--format','json'],text=True,env={**os.environ,'YN_DOMAIN':'license.example.com'}));assert production['volumes']['data']['name']==volume
            report['checks'].append('Root and HTTPS Compose files validate, use the same explicitly selected persistent volume, and repeated configuration preserves private credentials')
            subprocess.run([*command,'up','-d','--no-build','--pull','never'],check=True,capture_output=True,text=True)
            container=subprocess.check_output([*command,'ps','-q','license'],text=True).strip();assert container
            for _ in range(120):
                state=json.loads(subprocess.check_output(['docker','inspect',container],text=True))[0]
                if state['State'].get('Health',{}).get('Status')=='healthy':break
                assert state['State']['Running'],'container stopped during installation startup';time.sleep(1)
            else:raise AssertionError('Compose service did not become healthy')
            ports=state['NetworkSettings']['Ports'];assert not ports.get('3306/tcp') and not ports.get('33060/tcp');assert native.raw(base,'/')[0]==307
            private=subprocess.check_output([*command,'exec','-T','license','cat','/var/lib/mysql/.yunnuo/database.json.install-token'],text=True).strip();assert len(private)>=32
            assert http.http(base,'/v1/setup/initialize',{},expected=403)['error']['code']=='SETUP_TOKEN_REQUIRED'
            password=secrets.token_hex(24);http.http(base,'/v1/setup/initialize',{'database_driver':'mysql','database_host':'127.0.0.1','database_port':3306,'database_name':values['MYSQL_DATABASE'],'database_user':values['MYSQL_USER'],'database_password':values['MYSQL_PASSWORD'],'admin_username':'admin','admin_password':password},extra_headers={'X-Setup-Token':private})
            token=http.api(base,'/admin/login',{'username':'admin','password':password})['access_token'];product=Product(base,token,pathlib.Path(temporary)/'product',form='container');license=product.license();product.publish();product.download()
            report['checks'].append('Compose starts embedded MySQL without publishing database ports; first-run token installation, admin login and actual license activation work')
            subprocess.run([*command,'restart','license'],check=True,capture_output=True,text=True)
            for _ in range(120):
                try:
                    if http.api(base,'/readyz')['ready']:break
                except (OSError,AssertionError):pass
                time.sleep(1)
            else:raise AssertionError('Compose restart did not become ready')
            assert http.api(base,'/admin/profile',token=token)['username']=='admin';assert product.verify()['valid'];assert product.license()['updates_until']==license['updates_until'];product.download();assert http.api(base,'/v1/setup/status')['initialized']
            report['checks'].append('Compose restart preserves database, encryption keys, administrator session, issued license and original deadline')
            report.update(result='passed',database='mysql',published_database_ports=False)
        finally:
            with (args.artifacts/'compose.log').open('w') as log:subprocess.run([*command,'logs','--no-color'],stdout=log,stderr=subprocess.STDOUT)
            subprocess.run([*command,'down','--volumes','--remove-orphans'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            (args.artifacts/'report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
    print(json.dumps({'result':report['result'],'checks':len(report['checks'])}))
if __name__=='__main__':main()
