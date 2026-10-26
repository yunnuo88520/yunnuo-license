#!/usr/bin/env python3
"""Create/reuse private Compose credentials; never print passwords or replace keys."""
import argparse
import datetime
import json
import os
import pathlib
import re
import secrets
import shlex
import subprocess
import tempfile

REPO=pathlib.Path(__file__).resolve().parents[1]
def read_env(path):
    values={}
    if not path.exists():return values
    if path.is_symlink() or path.stat().st_mode&0o077:raise ValueError('existing environment file must be a regular private file (0600)')
    for line in path.read_text().splitlines():
        line=line.strip()
        if not line or line.startswith('#'):continue
        key,sep,value=line.partition('=')
        if not sep or not re.fullmatch(r'[A-Z][A-Z0-9_]*',key):raise ValueError('invalid environment variable line')
        parsed=shlex.split(value,comments=True)
        if len(parsed)>1:raise ValueError('quote environment values containing spaces')
        values[key]=parsed[0] if parsed else ''
    return values

def write_env(path,values):
    path.parent.mkdir(parents=True,exist_ok=True)
    fd,tmp=tempfile.mkstemp(prefix='.runtime-',dir=path.parent)
    try:
        with os.fdopen(fd,'w') as f:
            f.write('# Private deployment environment. Preserve database credentials and encryption keys.\n')
            for key,value in values.items():f.write(key+'='+shlex.quote(str(value))+'\n')
            f.flush();os.fsync(f.fileno())
        os.replace(tmp,path)
    finally:
        if os.path.exists(tmp):os.unlink(tmp)

def main():
    p=argparse.ArgumentParser();p.add_argument('--output',type=pathlib.Path,default=REPO/'.env');p.add_argument('--domain');p.add_argument('--config-dir',type=pathlib.Path);p.add_argument('--bind');p.add_argument('--port',type=int);p.add_argument('--image');args=p.parse_args()
    values=read_env(args.output)
    defaults={'YN_IMAGE':'yunnuo-license:local','YN_BIND_ADDR':'127.0.0.1','YN_HTTP_PORT':'8080','YN_DATA_VOLUME':'yunnuo-license-data','MYSQL_DATABASE':'yunnuo_license','MYSQL_USER':'yunnuo','YN_TRUST_PROXY_HEADERS':'false','YN_TRUSTED_PROXY_CIDRS':'127.0.0.1/32,::1/128'}
    for k,v in defaults.items():values.setdefault(k,v)
    for key in ['MYSQL_PASSWORD','MYSQL_ROOT_PASSWORD','YN_METRICS_TOKEN']:
        if not values.get(key):values[key]=secrets.token_hex(32)
        if values[key].startswith(('replace-','请替换','替换')):raise ValueError('placeholder credentials must be replaced explicitly; existing nonempty values are never regenerated')
    if args.domain:
        if not re.fullmatch(r'(?=.{1,253}$)[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?',args.domain):raise ValueError('domain must be a hostname without scheme, path or port')
        values['YN_DOMAIN']=args.domain
    if args.bind:
        import ipaddress
        ipaddress.ip_address(args.bind);values['YN_BIND_ADDR']=('['+args.bind+']') if ':' in args.bind else args.bind
    if args.port is not None:
        if not 1<=args.port<=65535:raise ValueError('port must be 1..65535')
        values['YN_HTTP_PORT']=str(args.port)
    if args.config_dir:values['YN_DEPLOY_CONFIG_DIR']=str(args.config_dir.resolve())
    if args.image:values['YN_IMAGE']=args.image
    values['YN_BUILD_COMMIT']=subprocess.check_output(['git','rev-parse','HEAD'],cwd=REPO,text=True).strip()
    values['YN_BUILD_TIME']=datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    write_env(args.output,values)
    if values.get('YN_DOMAIN'):
        subprocess.run(['bash',str(REPO/'deploy/configure.sh')],cwd=REPO,env={**os.environ,**values},check=True,stdout=subprocess.DEVNULL)
    print(json.dumps({'environment':str(args.output.resolve()),'existing_credentials_preserved':True,'https_configured':bool(values.get('YN_DOMAIN'))},ensure_ascii=False))
if __name__=='__main__':main()
