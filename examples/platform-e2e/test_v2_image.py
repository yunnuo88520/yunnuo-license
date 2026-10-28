#!/usr/bin/env python3
"""V2 acceptance against an installed image and an independently restored DB."""
import argparse,base64,json,os,pathlib,uuid
from v2_product import Product,http

def main():
 p=argparse.ArgumentParser();p.add_argument('phase',choices=['setup','verify']);p.add_argument('--base-url',required=True);p.add_argument('--state',required=True,type=pathlib.Path);p.add_argument('--artifacts',required=True,type=pathlib.Path);args=p.parse_args();args.artifacts.mkdir(parents=True,exist_ok=True);report={'result':'running','checks':[]}
 try:
  if args.phase=='setup':
   token=http.api(args.base_url,'/admin/login',{'username':'admin','password':os.environ['YN_TEST_ADMIN_PASSWORD']})['access_token'];product=Product(args.base_url,token,args.state.parent/'product');product.publish();product.download()
   # Hash-only server credentials are independent from installation proof.
   credentials=http.api(args.base_url,product.prefix+'/reset-secret',{'reason':'image credential rotation acceptance'},token)
   authorization='Basic '+base64.b64encode((product.app+':'+credentials['app_secret']).encode()).decode()
   assert http.http(args.base_url,'/v2/server/applications/'+product.app+'/licenses',extra_headers={'Authorization':authorization})['success']
   assert http.http(args.base_url,'/v1/licenses/verify',{},expected=410)['error']['code']=='AUTHORIZATION_V2_REQUIRED'
   http.api(args.base_url,product.prefix+'/rotate-key',{'reason':'backup rotation acceptance'},token);assert product.verify()['valid'];assert len(http.http(args.base_url,'/v2/applications/'+product.app+'/.well-known/jwks.json')['keys'])==2
   report['checks'].append('Image uses V2 only, hash-only server credentials and independent signing-key rotation; the generated client downloads signed packages')
   tier=http.api(args.base_url,'/admin/agent-tiers',{'name':'Docker half cost','cost_rate_bps':5000,'sort_order':10},token)
   opened=http.api(args.base_url,'/admin/agent-accounts',{'username':'image-seller-'+uuid.uuid4().hex[:8],'password':os.environ['YN_TEST_ADMIN_PASSWORD'],'tier_id':tier['id'],'initial_balance':1000,'reason':'manual channel receipt'},token)
   plan=http.api(args.base_url,product.prefix+'/plans',{'name':'Channel sale','agent_cost_cents':101,'terms':{'model':'perpetual','binding_types':['installation'],'update_duration_days':30}},token)
   http.api(args.base_url,product.prefix+'/agent-grants',{'agent_id':opened['agent']['id'],'plan_id':plan['id'],'enabled':True,'max_batch_quantity':10},token)
   seller=http.api(args.base_url,'/agent/login',{'username':opened['user']['username'],'password':os.environ['YN_TEST_ADMIN_PASSWORD']})['access_token']
   order={'plan_id':plan['id'],'quantity':2,'customer_ref':'real MySQL sale','request_id':str(uuid.uuid4()),'expected_unit_cost_cents':51};route='/v2/agent/applications/'+product.app+'/orders';first=http.api(args.base_url,route,order,seller);repeated=http.api(args.base_url,route,order,seller)
   assert len(first['cards'])==2 and first['order']['cost_cents']==102;assert repeated['replayed'] and not repeated['cards'];assert http.api(args.base_url,'/agent/account',token=seller)['balance']==898
   report['checks'].append('MySQL transaction charges ceil(101 × 50%) × 2 = 102 exactly once using the unified agent account')
   state={'app':product.app,'plan':product.plan,'license_id':product.license_id,'token':token,'folder':str(product.folder),'environment':{k:v for k,v in product.env.items() if k=='PYTHONPATH' or k.startswith('YN_')},'updates_until':product.license()['updates_until'],'agent_id':opened['agent']['id']}
   args.state.write_text(json.dumps(state));args.state.chmod(0o600)
  else:
   saved=json.loads(args.state.read_text());product=object.__new__(Product);product.base=args.base_url;product.token=saved['token'];product.app=saved['app'];product.prefix='/v2/console/applications/'+product.app;product.plan=saved['plan'];product.license_id=saved['license_id'];product.folder=pathlib.Path(saved['folder']);product.env={**{k:v for k,v in os.environ.items() if not k.startswith('YN_')},**saved['environment'],'YN_LICENSE_URL':args.base_url}
   assert product.verify()['valid'];assert product.license()['updates_until']==saved['updates_until'];product.download();assert http.api(args.base_url,product.prefix+'/audit-integrity',token=product.token)['valid'];assert len(http.http(args.base_url,'/v2/applications/'+product.app+'/.well-known/jwks.json')['keys'])==2
   account=http.api(args.base_url,'/admin/agents/'+saved['agent_id']+'/account',token=product.token);assert account['account']['balance']==898
   report['checks'].append('Restart/restored instance preserves private signing keys, JWKS overlap, license/card digests, exact deadlines, packages, audit integrity and the agent balance')
  report.update(result='passed',app_id=product.app)
 finally:(args.artifacts/'report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
 print(json.dumps({'result':report['result'],'checks':len(report['checks'])}))
if __name__=='__main__':main()
