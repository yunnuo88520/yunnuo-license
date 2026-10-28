"""Reusable V2 product acceptance fixture; no production credential defaults."""
import importlib.util,json,os,pathlib,subprocess,sys,zipfile
REPO=pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0,str(REPO/'examples/update-demo'))
import test_e2e as http
import test_v2 as updates

class Product:
 def __init__(self,base,token,folder,form='web',activate=True):
  self.base=base;self.token=token;self.folder=pathlib.Path(folder);self.folder.mkdir(parents=True)
  self.app=http.api(base,'/v2/console/applications',{'name':'Standard deployment product','allowed_models':['perpetual'],'allowed_bindings':['installation'],'forms':['web','container','gui'],'updates_enabled':True},token)['app_id'];self.prefix='/v2/console/applications/'+self.app
  self.plan=http.api(base,self.prefix+'/plans',{'name':'Permanent use, limited updates','terms':{'model':'perpetual','binding_types':['installation'],'forms':['web','container','gui'],'update_duration_days':30}},token)
  sale=http.api(base,self.prefix+'/orders',{'plan_id':self.plan['id'],'quantity':1,'distribute_cards':True,'confirmed':True,'reason':'deployment acceptance manual receipt'},token);self.license_id=sale['cards'][0]['license_id']
  kit=http.api(base,self.prefix+'/integration?language=python&binding_type=installation&form='+form,token=token)
  for file in kit['files']:(self.folder/file['name']).write_text(file['content'])
  deps=self.folder/'dependencies';package_python=os.environ.get('YN_TEST_PIP_PYTHON',sys.executable);pip_command=[package_python,'-m','pip']+(['--python',sys.executable] if package_python!=sys.executable else []);subprocess.run([*pip_command,'install','--quiet','--target',str(deps),'-r',str(self.folder/'requirements.txt')],check=True,capture_output=True)
  self.env={k:v for k,v in os.environ.items() if not k.startswith('YN_')};self.env.update(PYTHONPATH=str(deps),YN_LICENSE_URL=base,YN_STATE_DIR=str(self.folder/'state'),YN_BINDING_TYPE='installation',YN_FORM=form,YN_CURRENT_VERSION='1.0.0')
  self.activation_card=sale['cards'][0]['code']
  if activate:self.run('client.py','activate',{'YN_CARD':self.activation_card})
 def run(self,file,operation,extra=None,ok=True):
  command=subprocess.run([sys.executable,str(self.folder/file),operation],cwd=self.folder,env={**self.env,**(extra or {})},capture_output=True,text=True)
  if ok:
   assert command.returncode==0,command.stderr[-2000:];return json.loads(command.stdout)
  assert command.returncode!=0,'expected refusal';return command.stderr
 def verify(self):return self.run('client.py','verify')
 def publish(self,version='2.0.0',schema=2):return updates.upload(self.base,self.token,self.app,version,updates.package(version,schema))
 def download(self):
  assert self.run('update.py','check')['available'];download=self.run('update.py','download');assert self.run('update.py','authorize')['authorized'];return download
 def license(self):return next(l for l in http.api(self.base,self.prefix+'/licenses',token=self.token) if l['id']==self.license_id)
 def expire_updates(self,database):
  import datetime,sqlite3
  with sqlite3.connect(database) as db:
   row=json.loads(db.execute('SELECT data_json FROM application_licenses WHERE application_id=? AND id=?',(self.app,self.license_id)).fetchone()[0]);row['record']['updates_until']=(datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(seconds=1)).isoformat().replace('+00:00','Z');db.execute('UPDATE application_licenses SET data_json=? WHERE application_id=? AND id=?',(json.dumps(row),self.app,self.license_id))
 def renew_updates(self):
  sale=http.api(self.base,self.prefix+'/orders',{'plan_id':self.plan['id'],'quantity':1,'distribute_cards':True,'confirmed':True,'purpose':'renew_updates','reason':'manual update renewal receipt'},self.token)
  card={'YN_RENEWAL_CARD':sale['cards'][0]['code']};self.run('client.py','renew',card);first=self.license();self.run('client.py','renew',card);assert self.license()['updates_until']==first['updates_until'];assert first['expires_at'] is None
