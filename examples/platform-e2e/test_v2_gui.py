#!/usr/bin/env python3
"""Drive the downloaded Tk window in a real X display; save native screenshots."""
import argparse,importlib.util,json,os,pathlib,secrets,subprocess,tempfile,time,traceback
from v2_product import Product,http,REPO

def main():
 p=argparse.ArgumentParser();p.add_argument('--server',required=True,type=pathlib.Path);p.add_argument('--artifacts',required=True,type=pathlib.Path);p.add_argument('--xvfb',default='Xvfb');args=p.parse_args();args.artifacts.mkdir(parents=True,exist_ok=True);report={'result':'running','cases':[]};server=display_process=None;root=None
 with tempfile.TemporaryDirectory(prefix='v2-native-gui-') as temporary,(args.artifacts/'server.log').open('w') as log,(args.artifacts/'xvfb.log').open('w') as xlog:
  folder=pathlib.Path(temporary);port=http.free_port();base=f'http://127.0.0.1:{port}';password=secrets.token_hex(24);env={k:v for k,v in os.environ.items() if not k.startswith('YN_')};env.update(YN_ADDR=f'127.0.0.1:{port}',YN_DB_DRIVER='sqlite',YN_DB=f'file:{folder}/license.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)',YN_DB_CONFIG_FILE=str(folder/'database.json'),YN_RELEASE_DIR=str(folder/'packages'))
  try:
   if not os.environ.get('DISPLAY'):
    display=':'+str(100+secrets.randbelow(1000));display_process=subprocess.Popen([args.xvfb,display,'-screen','0','1100x900x24','-nolisten','tcp','-ac'],stdout=xlog,stderr=xlog);os.environ['DISPLAY']=display
    for _ in range(100):
     if display_process.poll() is not None:raise RuntimeError('Xvfb failed')
     if pathlib.Path('/tmp/.X11-unix/X'+display[1:]).exists():break
     time.sleep(.05)
   server=subprocess.Popen([str(args.server.resolve())],cwd=REPO/'backend',env=env,stdout=log,stderr=log);http.wait_ready(base,'/healthz',server);http.initialize(base,'admin',password,env['YN_DB']);token=http.api(base,'/admin/login',{'username':'admin','password':password})['access_token'];product=Product(base,token,folder/'product',form='gui',activate=False)
   spec=importlib.util.spec_from_file_location('downloaded_gui',product.folder/'gui.py');gui=importlib.util.module_from_spec(spec);spec.loader.exec_module(gui)
   # The same customer state survives a desktop process restart.
   os.environ.update(product.env)
   import tkinter as tk
   from PIL import ImageGrab
   root=tk.Tk();window=gui.Window(root);window.base.set(base);window.kind.set('installation');window.value.set('');window.card.set(product.activation_card)
   failures=[]
   def capture(name):
    root.update_idletasks();x,y=root.winfo_rootx(),root.winfo_rooty();ImageGrab.grab(bbox=(x,y,x+root.winfo_width(),y+root.winfo_height()),xdisplay=os.environ['DISPLAY']).save(args.artifacts/name)
   steps=[('activate','验证通过','v2-gui-activated.png'),('verify','验证通过','v2-gui-online.png'),('local','验证通过','v2-gui-local.png'),('release','租约已释放','v2-gui-released.png'),('verify','验证通过','v2-gui-resumed.png')]
   started=[0]
   def advance():
    if not steps:report['result']='passed';root.quit();return
    operation,_,_=steps[0];started[0]=time.monotonic();window.run(operation);root.after(50,poll)
   def poll():
    try:
     operation,expected,name=steps[0];text=window.status.get()
     if not text.startswith('正在'):
      assert expected in text,text;assert window.card.get()=='';capture(name);report['cases'].append({'operation':operation,'passed':True,'screenshot':name});steps.pop(0);root.after(50,advance);return
     if time.monotonic()-started[0]>30:raise RuntimeError('GUI operation timed out')
     root.after(50,poll)
    except Exception:failures.append(traceback.format_exc());root.quit()
   root.after(100,advance);root.mainloop();assert not failures,'\n'.join(failures)
  except Exception as e:report.update(result='failed',error=str(e));raise
  finally:
   if root is not None:root.destroy()
   if server is not None:server.terminate();server.wait(timeout=10)
   if display_process is not None:display_process.terminate();display_process.wait(timeout=10)
   (args.artifacts/'report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2))
 print(json.dumps({'result':report['result'],'cases':len(report['cases'])}))
if __name__=='__main__':main()
