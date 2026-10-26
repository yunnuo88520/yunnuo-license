#!/usr/bin/env python3
"""Configure an extracted Linux release without installing Go or Node.js."""
import argparse
import datetime
import getpass
import grp
import ipaddress
import json
import os
import pathlib
import pwd
import re
import shlex
import tempfile

ROOT=pathlib.Path(__file__).resolve().parents[2]

def read_env(path):
    values={}
    if not path.exists():return values
    if path.is_symlink() or path.stat().st_mode&0o077:raise ValueError('runtime.env must be a regular private file (0600)')
    for line in path.read_text().splitlines():
        line=line.strip()
        if not line or line.startswith('#'):continue
        key,sep,value=line.partition('=')
        if not sep or not re.fullmatch('[A-Z][A-Z0-9_]*',key):raise ValueError('invalid runtime environment line')
        parsed=shlex.split(value,comments=True)
        if len(parsed)>1:raise ValueError('quote environment values containing spaces')
        values[key]=parsed[0] if parsed else ''
    return values

def write(path,content,owner,mode=0o600):
    fd,temporary=tempfile.mkstemp(prefix='.configure-',dir=path.parent)
    try:
        with os.fdopen(fd,'w') as f:
            f.write(content);f.flush();os.fsync(f.fileno())
        os.chmod(temporary,mode)
        if os.geteuid()==0:os.chown(temporary,owner.pw_uid,owner.pw_gid)
        os.replace(temporary,path)
    finally:
        if os.path.exists(temporary):os.unlink(temporary)

def nginx_quote(path):
    value=str(path)
    if any(c in value for c in '\r\n\x00$'):raise ValueError('Nginx paths cannot contain newline, NUL or $')
    return '"'+value.replace('\\','\\\\').replace('"','\\"')+'"'

def unit_quote(value):return '"'+str(value).replace('%','%%').replace('\\','\\\\').replace('"','\\"')+'"'

def main():
    p=argparse.ArgumentParser();p.add_argument('--state-dir',type=pathlib.Path,default=ROOT.parent/'yunnuo-license-data');p.add_argument('--frontend-dir',type=pathlib.Path);p.add_argument('--domain');p.add_argument('--port',type=int);p.add_argument('--bind');p.add_argument('--nginx-listen');p.add_argument('--upstream',help='Go host:port, also supports a backend on another machine');p.add_argument('--user');p.add_argument('--mode',choices=['split','compatible']);p.add_argument('--trusted-proxy-cidrs');g=p.add_mutually_exclusive_group();g.add_argument('--frontend-only',action='store_true');g.add_argument('--backend-only',action='store_true');args=p.parse_args()
    previous={};metadata_path=args.state_dir.resolve()/'deployment.json'
    if metadata_path.exists():
        if metadata_path.is_symlink() or metadata_path.stat().st_mode&0o077:raise ValueError('deployment.json must be private')
        previous=json.loads(metadata_path.read_text())
    args.domain=args.domain or previous.get('domain','localhost');args.nginx_listen=args.nginx_listen or previous.get('nginx_listen','80');args.user=args.user or previous.get('user',getpass.getuser());args.mode=args.mode or previous.get('mode','split')
    args.upstream=args.upstream or previous.get('upstream_override')
    if args.frontend_dir is None and previous.get('frontend_dir_override'):args.frontend_dir=pathlib.Path(previous['frontend_dir_override'])
    if not args.frontend_only and not (ROOT/'backend/yunnuo-server').is_file():raise ValueError('configure an extracted release archive, not a source checkout')
    if not re.fullmatch(r'[A-Za-z0-9_.-]+',args.domain):raise ValueError('domain must be one hostname without scheme or path')
    if not re.fullmatch(r'(?:\[[0-9a-fA-F:]+\]|[A-Za-z0-9_.-]+):[0-9]+|[0-9]+',args.nginx_listen):raise ValueError('invalid Nginx listen address')
    account=pwd.getpwnam(args.user)
    if os.geteuid()!=0 and os.geteuid()!=account.pw_uid:raise ValueError('run as the service user or as root')
    if args.state_dir.is_symlink():raise ValueError('state directory cannot be a symlink')
    if (args.frontend_only or args.backend_only) and args.mode!='split':raise ValueError('separate archives require split mode')
    if args.frontend_only and not args.upstream:raise ValueError('frontend-only deployment requires --upstream backend-host:port')
    state=args.state_dir.resolve();frontend=(args.frontend_dir or ROOT/'frontend').resolve()
    if any(c in str(path) for path in [ROOT,state,frontend] for c in '\r\n\x00'):raise ValueError('deployment paths cannot contain control characters')
    if state==frontend or frontend in state.parents or len(state.parts)<3:raise ValueError('private state must be outside the public frontend directory')
    if not args.backend_only and not frontend.is_dir():raise ValueError('frontend directory missing: supply --frontend-dir when using separate archives')
    state.mkdir(parents=True,exist_ok=True,mode=0o700)
    if state.is_symlink():raise ValueError('state directory cannot be a symlink')
    if os.geteuid()==0:os.chown(state,account.pw_uid,account.pw_gid)
    os.chmod(state,0o700)
    environment=state/'runtime.env';values=read_env(environment)
    old_addr=values.get('YN_ADDR','127.0.0.1:8080');old_host,_,old_port=old_addr.rpartition(':')
    host=args.bind or old_host.strip('[]') or '127.0.0.1';ipaddress.ip_address(host);port=args.port if args.port is not None else int(old_port)
    if not 1<=port<=65535:raise ValueError('Go port must be 1..65535')
    address=('['+host+']' if ':' in host else host)+':'+str(port)
    upstream_host=('::1' if ':' in host else '127.0.0.1') if ipaddress.ip_address(host).is_unspecified else host
    upstream=args.upstream or ('['+upstream_host+']' if ':' in upstream_host else upstream_host)+':'+str(port)
    if not re.fullmatch(r'(?:\[[0-9a-fA-F:]+\]|[A-Za-z0-9_.-]+):[0-9]+',upstream):raise ValueError('upstream must be a host:port without URL or path')
    dsn='file:'+str(state/'bootstrap.sqlite')+'?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)'
    for key,value in {'YN_DB_DRIVER':'sqlite','YN_DB':dsn,'YN_SQLITE_SETUP_DSN':dsn,'YN_DB_CONFIG_FILE':str(state/'database.json'),'YN_RELEASE_DIR':str(state/'packages'),'YN_TRUST_PROXY_HEADERS':'true','YN_TRUSTED_PROXY_CIDRS':'127.0.0.1/32,::1/128','YN_BACKUP_DIR':str(state/'backups')}.items():values.setdefault(key,value)
    # Refresh release paths while preserving database credentials, encryption
    # keys, limits and other operator settings across release replacements.
    values.update(YN_ADDR=address,YN_SERVE_FRONTEND='true' if args.mode=='compatible' else 'false',YN_SDK_DIR=str(ROOT/'backend/sdk'),YN_PUBLIC_STATIC_DIR=str(frontend),YN_ADMIN_STATIC_DIR=str(frontend/'admin-console'),YN_AGENT_STATIC_DIR=str(frontend/'agent-console'),YN_DEVELOPER_STATIC_DIR=str(frontend/'developer-console'),YN_CUSTOMER_STATIC_DIR=str(frontend/'user-console'),YN_BACKUP_OPS=str(ROOT/'backend/yunnuo-ops'))
    if args.trusted_proxy_cidrs is not None:
        for network in args.trusted_proxy_cidrs.split(','):ipaddress.ip_network(network.strip(),strict=False)
        values['YN_TRUSTED_PROXY_CIDRS']=args.trusted_proxy_cidrs
    if not args.frontend_only and environment.exists():
        stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%S%fZ');write(state/('runtime.env.'+stamp+'.bak'),environment.read_text(),account)
    if not args.frontend_only:write(environment,'# Private runtime configuration. Preserve keys and database configuration.\n'+''.join(k+'='+shlex.quote(v)+'\n' for k,v in values.items()),account)
    generated=state/'deploy';generated.mkdir(exist_ok=True,mode=0o700)
    if os.geteuid()==0:os.chown(generated,account.pw_uid,account.pw_gid)
    for name in ['security-headers.conf','proxy-headers.conf']:write(generated/name,(ROOT/'deploy/native'/name).read_text(),account)
    replacements={'__UPSTREAM__':upstream,'__PROXY_HEADERS__':nginx_quote(generated/'proxy-headers.conf'),'__SECURITY_HEADERS__':nginx_quote(generated/'security-headers.conf'),'__LOCATIONS__':nginx_quote(generated/'nginx-locations.conf'),'__FRONTEND__':nginx_quote(frontend),'__DOMAIN__':args.domain,'__LISTEN__':args.nginx_listen}
    locations='nginx-locations.conf.template' if args.mode=='split' else 'nginx-compatible-locations.conf.template'
    for source,target in [(locations,'nginx-locations.conf'),('nginx-server.conf.template','nginx-server.conf')]:
        text=(ROOT/'deploy/native'/source).read_text()
        for key,value in replacements.items():text=text.replace(key,value)
        write(generated/target,text,account)
    metadata={'domain':args.domain,'nginx_listen':args.nginx_listen,'user':account.pw_name,'mode':args.mode,'upstream_override':args.upstream,'frontend_dir_override':str(frontend) if args.frontend_dir else None}
    write(metadata_path,json.dumps(metadata,indent=2)+'\n',account)
    if args.frontend_only:
        print(json.dumps({'frontend_dir':str(frontend),'upstream':upstream,'nginx_server':str(generated/'nginx-server.conf'),'baota_locations':str(generated/'nginx-locations.conf'),'runtime_env':None},ensure_ascii=False));return
    unit='''[Unit]\nDescription=Yunnuo License API\nAfter=network-online.target\nWants=network-online.target\n[Service]\nType=simple\nUser={user}\nGroup={group}\nWorkingDirectory={working}\nEnvironment={environment}\nExecStart=/bin/bash {start}\nRestart=on-failure\nRestartSec=3\nTimeoutStopSec=90\nKillSignal=SIGTERM\nUMask=0077\nNoNewPrivileges=yes\nPrivateTmp=yes\nProtectSystem=strict\nReadWritePaths={state}\n[Install]\nWantedBy=multi-user.target\n'''.format(user=account.pw_name,group=grp.getgrgid(account.pw_gid).gr_name,working=str(ROOT/'backend').replace('%','%%'),environment=unit_quote('YN_RUNTIME_FILE='+str(environment)),start=unit_quote(ROOT/'deploy/native/start.sh'),state=unit_quote(state))
    write(generated/'yunnuo-license.service',unit,account)
    logs=state/'logs';logs.mkdir(exist_ok=True,mode=0o700)
    if os.geteuid()==0:os.chown(logs,account.pw_uid,account.pw_gid)
    # Supervisor uses ConfigParser interpolation; literal percent signs are escaped.
    q=lambda value:shlex.quote(str(value)).replace('%','%%')
    supervisor='''[program:yunnuo-license]\ncommand=/usr/bin/env {environment} /bin/bash {start}\ndirectory={working}\nuser={user}\nautostart=true\nautorestart=true\nstartsecs=3\nstopsignal=TERM\nstopwaitsecs=90\nstopasgroup=true\nkillasgroup=true\nredirect_stderr=true\nstdout_logfile={log}\nstdout_logfile_maxbytes=20MB\nstdout_logfile_backups=5\numask=0077\n'''.format(environment=q('YN_RUNTIME_FILE='+str(environment)),start=q(ROOT/'deploy/native/start.sh'),working=str(ROOT/'backend').replace('%','%%'),user=account.pw_name,log=str(logs/'server.log').replace('%','%%'))
    write(generated/'supervisor.conf',supervisor,account)
    print(json.dumps({'release_dir':str(ROOT),'state_dir':str(state),'runtime_env':str(environment),'mode':args.mode,'go_address':address,'nginx_server':None if args.backend_only else str(generated/'nginx-server.conf'),'baota_locations':None if args.backend_only else str(generated/'nginx-locations.conf'),'systemd_unit':str(generated/'yunnuo-license.service'),'supervisor_config':str(generated/'supervisor.conf'),'credentials_preserved':True},ensure_ascii=False))
if __name__=='__main__':main()
