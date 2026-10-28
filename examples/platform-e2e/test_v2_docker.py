#!/usr/bin/env python3
"""Compile one generated Go product, run and upgrade actual Docker images."""
import argparse,gzip,hashlib,json,os,pathlib,secrets,subprocess,tempfile,uuid
from v2_product import Product,http,updates,REPO

def main():
 p=argparse.ArgumentParser();p.add_argument('--server',required=True,type=pathlib.Path);p.add_argument('--artifacts',required=True,type=pathlib.Path);args=p.parse_args();args.artifacts.mkdir(parents=True,exist_ok=True);report={'result':'running','cases':[]};process=None;images=[]
 def passed(name):report['cases'].append(name);print(name,flush=True)
 def docker(*parts,ok=True):
  result=subprocess.run(['docker',*map(str,parts)],capture_output=True,text=True)
  if ok:assert result.returncode==0,result.stderr[-3000:]
  return result
 with tempfile.TemporaryDirectory(prefix='v2-docker-') as temporary,(args.artifacts/'server.log').open('w') as log:
  folder=pathlib.Path(temporary);port=http.free_port();base=f'http://127.0.0.1:{port}';database=folder/'license.sqlite';password=secrets.token_hex(24);env={k:v for k,v in os.environ.items() if not k.startswith('YN_')};env.update(YN_ADDR=f'127.0.0.1:{port}',YN_DB_DRIVER='sqlite',YN_DB=f'file:{database}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)',YN_DB_CONFIG_FILE=str(folder/'database.json'),YN_RELEASE_DIR=str(folder/'packages'),YN_CLIENT_READ_RATE_LIMIT='10000')
  try:
   process=subprocess.Popen([str(args.server.resolve())],cwd=REPO/'backend',env=env,stdout=log,stderr=log);http.wait_ready(base,'/healthz',process);http.initialize(base,'admin',password,env['YN_DB']);token=http.api(base,'/admin/login',{'username':'admin','password':password})['access_token'];product=Product(base,token,folder/'python-controller',form='container',activate=False)
   kit=http.api(base,product.prefix+'/integration?language=go&binding_type=installation&form=container',token=token);source=folder/'go-product';source.mkdir()
   for file in kit['files']:(source/file['name']).write_text(file['content'])
   build_env={**os.environ,'CGO_ENABLED':'0','GOFLAGS':'-mod=mod'};subprocess.run(['go','mod','tidy'],cwd=source,env=build_env,check=True,capture_output=True)
   data=folder/'persistent-product-data';data.mkdir();(data/'customer.json').write_text(json.dumps({'customer_articles':['Original customer content'],'configuration':'preserved'}));original=(data/'customer.json').read_bytes()
   def build(version,bad=False):
    stage=folder/('image-'+version);stage.mkdir();tag='yunnuo-v2-product-'+uuid.uuid4().hex[:12]+':'+version;images.append(tag)
    flags='-X main.productVersion='+version+(' -X main.productHealth=fail' if bad else '')
    subprocess.run(['go','build','-buildvcs=false','-ldflags',flags,'-o',str(stage/'product'),'.'],cwd=source,env=build_env,check=True,capture_output=True)
    (stage/'Dockerfile').write_text('FROM scratch\nCOPY product /product\nENTRYPOINT ["/product"]\n');docker('build','-q','-t',tag,stage);digest=docker('image','inspect','--format','{{.Id}}',tag).stdout.strip();return tag,digest
   def run(tag,operation,extra=None,volume=None,ok=True):
    environment={'YN_LICENSE_URL':base,'YN_STATE_DIR':'/state','YN_BINDING_TYPE':'installation','YN_FORM':'container','YN_UPDATE_KIND':'container_archive',**(extra or {})};command=['run','--rm','--network','host','--user',str(os.getuid())+':'+str(os.getgid()),'-v',str(volume or data)+':/state']
    for name,value in environment.items():command+=['-e',name+'='+value]
    result=docker(*command,tag,operation,ok=ok)
    if not ok:assert result.returncode!=0;return result.stderr
    return result.stdout.strip()
   first,_=build('1.0.0');run(first,'activate',{'YN_CARD':product.activation_card});identity=data/product.app/'installation.key';key_digest=hashlib.sha256(identity.read_bytes()).hexdigest();assert json.loads(run(first,'verify'))['valid'];passed('Docker: generated standard product activates once; container recreation retains private installation identity')
   assert 'container machine fingerprint forbidden' in run(first,'verify',{'YN_BINDING_TYPE':'machine_id','YN_BINDING_VALUE':'0'*64},ok=False);passed('Docker: container machine binding is rejected without a mounted host fingerprint')
   other=folder/'missing-volume';other.mkdir();assert 'ACTIVATION_LIMIT_EXCEEDED' in run(first,'activate',{'YN_CARD':product.activation_card},volume=other,ok=False);passed('Docker: losing the persistent state creates a new installation and cannot bypass activation quota')
   def publish(version,bad=False):
    tag,digest=build(version,bad);archive=folder/(version+'.tar.gz')
    tar=folder/(version+'.tar');export=subprocess.run(['docker','save','-o',str(tar),tag],capture_output=True);assert export.returncode==0
    with tar.open('rb') as incoming,gzip.open(archive,'wb') as outgoing:
     import shutil;shutil.copyfileobj(incoming,outgoing)
    tar.unlink();target={'os':'linux','arch':'amd64','runtime':'go','channel':'stable','kind':'container_archive','image_tag':tag,'image_digest':digest};release=updates.upload(base,token,product.app,version,archive.read_bytes(),target=target);docker('image','rm',tag);return tag,digest,release
   second,digest,release=publish('2.0.0');assert json.loads(run(first,'update-check'))['available'];download=json.loads(run(first,'update-download'));package=data/pathlib.PurePosixPath(download['path']).relative_to('/state');assert hashlib.sha256(package.read_bytes()).hexdigest()==release['sha256'];passed('Docker: actual immutable image archive is selected, signed, downloaded and checked by generated Go code')
   product.expire_updates(database);assert 'UPDATES_EXPIRED' in run(first,'update-authorize',ok=False);assert json.loads(run(first,'verify'))['valid'];assert run(first,'version')=='1.0.0';passed('Docker: expiry after download refuses installation while permanent use remains valid')
   renewal=http.api(base,product.prefix+'/orders',{'plan_id':product.plan['id'],'quantity':1,'distribute_cards':True,'confirmed':True,'purpose':'renew_updates','reason':'manual update renewal'},token);run(first,'renew',{'YN_RENEWAL_CARD':renewal['cards'][0]['code']});permit=json.loads(run(first,'update-authorize'));assert permit['authorized'];docker('load','-i',package);assert docker('image','inspect','--format','{{.Id}}',second).stdout.strip()==digest;assert run(second,'version')=='2.0.0';assert json.loads(run(second,'verify'))['valid'];assert hashlib.sha256(identity.read_bytes()).hexdigest()==key_digest and (data/'customer.json').read_bytes()==original;passed('Docker: renewed update imports the actual v2 image and preserves customer configuration, data and activation')
   third,_,_=publish('3.0.0',bad=True);run(second,'update-check');download=json.loads(run(second,'update-download'));run(second,'update-authorize');docker('load','-i',data/pathlib.PurePosixPath(download['path']).relative_to('/state'));assert 'health check failed' in run(third,'health',ok=False);assert run(second,'health')=='ok' and run(second,'version')=='2.0.0';assert json.loads(run(second,'verify'))['valid'];assert (data/'customer.json').read_bytes()==original;passed('Docker: candidate health command fails; restarting the previous image retains the same volume and authorized installation')
   report.update(result='passed',app_id=product.app)
  except Exception as e:report.update(result='failed',error=str(e));raise
  finally:
   for image in images:docker('image','rm','-f',image,ok=False)
   if process is not None:process.terminate();process.wait(timeout=10)
   (args.artifacts/'report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
if __name__=='__main__':main()
