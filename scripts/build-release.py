#!/usr/bin/env python3
"""Build static Linux binaries and public tar.gz payloads from the repo root."""
import argparse
import datetime
import gzip
import hashlib
import json
import os
import pathlib
import fnmatch
import re
import shutil
import subprocess
import tarfile
import tempfile

ROOT=pathlib.Path(__file__).resolve().parents[1]
EXCLUDED={'node_modules','target','.git','.m2','.gocache','__pycache__','secrets','backups','textfile','generated','runtime.env','deployment.json','database.json'}

def copy_tree(source,target):
    shutil.copytree(source,target,dirs_exist_ok=True,symlinks=True,ignore=lambda directory,names:[n for n in names if n in EXCLUDED or n.startswith('.env') or any(fnmatch.fnmatch(n,pattern) for pattern in ['*.pyc','*.log','*.tgz','*.install-token','*.db','*.db-*','*.sqlite','*.sqlite-*','*.age','*.pem','*.key'])])

def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for part in iter(lambda:f.read(1024*1024),b''):h.update(part)
    return h.hexdigest()

def archive(stage,output,epoch,selected):
    # Fixed uid/gid, timestamps and gzip headers keep filesystem identity and
    # build-machine paths out of published assets. No symlinks/private data.
    paths=[stage]+[p for p in stage.rglob('*') if not selected or p.relative_to(stage).parts[0] in selected]
    with output.open('wb') as raw,gzip.GzipFile(fileobj=raw,mode='wb',mtime=epoch,filename='') as zipped,tarfile.open(fileobj=zipped,mode='w') as tar:
        for path in sorted(paths):
            if path.is_symlink():raise ValueError('release payload may not contain symlinks')
            relative=path.relative_to(stage);name=stage.name+('/'+relative.as_posix() if path!=stage else '')
            info=tarfile.TarInfo(name);info.uid=info.gid=0;info.uname=info.gname='root';info.mtime=epoch
            if path.is_dir():
                info.type=tarfile.DIRTYPE;info.mode=0o755;tar.addfile(info);continue
            executable=path.name in {'yunnuo-server','yunnuo-ops','yunnuo-healthcheck'} or path.suffix=='.sh'
            info.size=path.stat().st_size;info.mode=0o755 if executable else 0o644
            with path.open('rb') as f:tar.addfile(info,f)

def main():
    p=argparse.ArgumentParser();p.add_argument('--arch',action='append',choices=['amd64','arm64']);p.add_argument('--output',type=pathlib.Path,default=ROOT/'dist');p.add_argument('--skip-frontend-build',action='store_true',help='reuse verified frontend/dist and installed dependencies for license notices');p.add_argument('--allow-dirty',action='store_true',help='preview only: mark uncommitted sources explicitly');p.add_argument('--split-only',action='store_true',help='omit the combined bundle');args=p.parse_args()
    arches=list(dict.fromkeys(args.arch or ['amd64','arm64']));version=(ROOT/'VERSION').read_text().strip()
    if not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?',version):raise ValueError('VERSION must be a semantic version without paths or whitespace')
    commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip();dirty=bool(subprocess.check_output(['git','status','--porcelain','--untracked-files=normal'],cwd=ROOT,text=True))
    if dirty and not args.allow_dirty:raise ValueError('commit source changes before packaging; --allow-dirty is for marked preview artifacts only')
    epoch=int(os.environ.get('SOURCE_DATE_EPOCH') or subprocess.check_output(['git','show','-s','--format=%ct','HEAD'],cwd=ROOT,text=True).strip());built=datetime.datetime.fromtimestamp(epoch,datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    if not args.skip_frontend_build:
        subprocess.run(['npm','ci'],cwd=ROOT/'frontend',check=True);subprocess.run(['npm','run','build'],cwd=ROOT/'frontend',check=True)
    if not (ROOT/'frontend/dist/index.html').is_file():raise ValueError('build frontend/dist first')
    output=args.output.resolve();output.mkdir(parents=True,exist_ok=True);outputs=[]
    subprocess.run(['go','mod','download'],cwd=ROOT/'backend',check=True)
    modules=json.loads(subprocess.check_output(['go','list','-m','-json'],cwd=ROOT/'backend',text=True));assert modules['Path']=='github.com/yunnuo88520/yunnuo-license/backend'
    # Collect redistribution notices for the linked Go modules, including Go
    # itself. No module source, caches, private runtime files or environment.
    module_stream=subprocess.check_output(['go','list','-m','-json','all'],cwd=ROOT/'backend',text=True);decoder=json.JSONDecoder();packages=[];offset=0
    while offset<len(module_stream):
        while offset<len(module_stream) and module_stream[offset].isspace():offset+=1
        if offset==len(module_stream):break
        item,offset=decoder.raw_decode(module_stream,offset);packages.append(item)
    with tempfile.TemporaryDirectory(prefix='yunnuo-release-') as temporary:
        base=pathlib.Path(temporary)
        common=base/'common';common.mkdir();copy_tree(ROOT/'deploy',common/'deploy');copy_tree(ROOT/'docs',common/'docs');shutil.copy2(ROOT/'LICENSE',common/'LICENSE');shutil.copy2(ROOT/'VERSION',common/'VERSION');shutil.copy2(ROOT/'README.md',common/'PROJECT-README.md');shutil.copy2(ROOT/'deploy/native/ARCHIVE-README.md',common/'README.md');shutil.copy2(ROOT/'deploy/native/ARCHIVE-README.md',common/'DEPLOY.md');shutil.copy2(ROOT/'CHANGELOG.md',common/'CHANGELOG.md')
        notices=common/'licenses';notices.mkdir();goroot=pathlib.Path(subprocess.check_output(['go','env','GOROOT'],text=True).strip());shutil.copy2(goroot/'LICENSE',notices/'Go-LICENSE')
        frontend_notices=notices/'frontend';frontend_notices.mkdir();frontend_modules=[]
        installed=subprocess.check_output(['npm','ls','--omit=dev','--all','--parseable'],cwd=ROOT/'frontend',text=True).splitlines()
        for directory in installed:
            source=pathlib.Path(directory).resolve()
            if source==ROOT/'frontend':continue
            source.relative_to(ROOT/'frontend/node_modules')
            package=json.loads((source/'package.json').read_text());name=package['name'].replace('/','_')+'@'+package['version'];target=frontend_notices/name;target.mkdir()
            for candidate in source.iterdir():
                if candidate.is_file() and candidate.name.upper().startswith(('LICENSE','COPYING','NOTICE')):shutil.copy2(candidate,target/candidate.name)
            frontend_modules.append({'name':package['name'],'version':package['version'],'license':package.get('license'),'notices':[f.name for f in target.iterdir()]})
        (frontend_notices/'INDEX.json').write_text(json.dumps(frontend_modules,indent=2))
        for module in packages:
            if module.get('Main') or not module.get('Dir'):continue
            source=pathlib.Path(module['Dir']);name=module['Path'].replace('/','_')+'@'+module.get('Version','unknown')
            for candidate in source.iterdir():
                if candidate.is_file() and candidate.name.upper().startswith(('LICENSE','COPYING','NOTICE')):
                    (notices/name).mkdir(exist_ok=True);shutil.copy2(candidate,notices/name/candidate.name)
        web=base/('yunnuo-license-v'+version+'-frontend');copy_tree(common,web);copy_tree(ROOT/'frontend/dist',web/'frontend');(web/'BUILD_INFO.json').write_text(json.dumps({'version':version,'commit':commit,'source_dirty':dirty,'component':'frontend','build_time':built},indent=2))
        path=output/(web.name+'.tar.gz');archive(web,path,epoch,None);outputs.append(path)
        for arch in arches:
            name='yunnuo-license-v'+version+'-linux-'+arch;stage=base/name;copy_tree(common,stage);copy_tree(ROOT/'frontend/dist',stage/'frontend');copy_tree(ROOT/'backend/migrations',stage/'backend/migrations');copy_tree(ROOT/'sdk',stage/'backend/sdk')
            env={**os.environ,'GOOS':'linux','GOARCH':arch,'CGO_ENABLED':'0'}
            for command,target in [('server','yunnuo-server'),('maintenance','yunnuo-ops'),('healthcheck','yunnuo-healthcheck')]:
                ldflags='-s -w -X github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo.Version='+version+' -X github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo.Commit='+commit+' -X github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo.BuildTime='+built
                subprocess.run(['go','build','-trimpath','-buildvcs=false','-ldflags',ldflags,'-o',str(stage/'backend'/target),'./cmd/'+command],cwd=ROOT/'backend',env=env,check=True)
            info={'version':version,'commit':commit,'source_dirty':dirty,'os':'linux','arch':arch,'build_time':built,'go_version':subprocess.check_output(['go','env','GOVERSION'],text=True).strip(),'frontend_serving':'nginx with API page gate; compatible mode optional'};(stage/'BUILD_INFO.json').write_text(json.dumps(info,indent=2))
            internal=[str(p.relative_to(stage)) for p in stage.rglob('*') if p.is_file()];(stage/'FILES.sha256').write_text(''.join(digest(stage/name)+'  '+name+'\n' for name in sorted(internal)))
            if not args.split_only:
                path=output/(name+'.tar.gz');archive(stage,path,epoch,None);outputs.append(path)
            backend=base/(name+'-backend');copy_tree(stage,backend);shutil.rmtree(backend/'frontend');(backend/'FILES.sha256').unlink();internal=[str(p.relative_to(backend)) for p in backend.rglob('*') if p.is_file()];(backend/'FILES.sha256').write_text(''.join(digest(backend/name)+'  '+name+'\n' for name in sorted(internal)));path=output/(backend.name+'.tar.gz');archive(backend,path,epoch,None);outputs.append(path)
    # Only checksums for this invocation are included, never unrelated old files.
    (output/'SHA256SUMS').write_text(''.join(digest(path)+'  '+path.name+'\n' for path in outputs))
    print(json.dumps({'version':version,'commit':commit,'source_dirty':dirty,'archives':[str(p) for p in outputs],'checksums':str(output/'SHA256SUMS')},ensure_ascii=False))
if __name__=='__main__':main()
