"""Optional update transport, using the same installation identity as client.py.
This verifies/downloads/authorizes; the product owns backup, migration and restart.
python update.py check | download | authorize
Environment: YN_CURRENT_VERSION, YN_UPDATE_OS/ARCH/RUNTIME/CHANNEL/KIND.
There is deliberately NO offline update authorization.
"""
import hashlib, json, os, pathlib, secrets, sys, urllib.request, urllib.error
import client

def state():
    cache=json.loads((client.STATE/'cache.json').read_text())
    if not cache.get('activation_id'): raise RuntimeError('Activate the product first')
    client.clock(cache)
    return cache

def request_body(cache):
    return {'app_id':client.APP_ID,'activation_id':cache['activation_id'],'binding':client.BINDING,'form':client.FORM,
        'current_version':os.environ['YN_CURRENT_VERSION'],
        'target':{name:os.environ.get('YN_UPDATE_'+name.upper(),default) for name,default in [('os','any'),('arch','any'),('runtime','any'),('channel','stable'),('kind','archive')]}}

def manifest(token,cache,body,expected=None):
    claims=client.local_verify(token,'update_manifest',cache['activation_id'],cache)
    release=claims['release']
    if claims['sub']!=cache['license_id']: raise RuntimeError('Wrong update license')
    for name in ('os','arch','runtime','channel','kind'):
        if release['target'][name] not in (body['target'][name],'any'): raise RuntimeError('Wrong update target')
    if not 0<release['size']<=256*1024*1024: raise RuntimeError('Invalid package size')
    if len(release['sha256'])!=64 or any(c not in '0123456789abcdef' for c in release['sha256']): raise RuntimeError('Invalid package digest')
    if expected is not None and release!=expected: raise RuntimeError('Update changed; check again')
    return release

def check():
    cache=state();body=request_body(cache);result=client.send('/v2/updates/check',body,preserve_update_errors=True)
    if not result['available']:
        (client.STATE/'update.json').unlink(missing_ok=True)
        return {'available':False}
    release=manifest(result['manifest_jwt'],cache,body)
    client.save('update.json',{'body':body,'release':release})
    return {'available':True,'version':release['version'],'sha256':release['sha256'],'size':release['size']}

def selected():
    cache=state();pending=json.loads((client.STATE/'update.json').read_text())
    if pending['body']!=request_body(cache): raise RuntimeError('Product version or target changed; check again')
    return cache,pending

def download():
    cache,pending=selected();release=pending['release'];body=pending['body']
    # The request proof is generated afresh: never reuse check's nonce.
    request=client.signed_request('/v2/updates/'+release['id']+'/download',body)
    target=client.STATE/(release['sha256']+'.package');temp=target.with_suffix('.part-'+secrets.token_hex(8))
    try:
        with urllib.request.urlopen(request,timeout=60) as response:
            manifest(response.headers['X-YN-Update-Manifest'],cache,body,release)
            count=0;digest=hashlib.sha256()
            with temp.open('xb') as output:
                os.chmod(temp,0o600)
                while True:
                    chunk=response.read(65536)
                    if not chunk: break
                    count+=len(chunk)
                    if count>release['size']: raise RuntimeError('Package exceeds signed size')
                    digest.update(chunk);output.write(chunk)
                output.flush();os.fsync(output.fileno())
        if count!=release['size'] or digest.hexdigest()!=release['sha256']: raise RuntimeError('Package digest mismatch')
        os.replace(temp,target)
    except urllib.error.HTTPError as error:
        detail=error.read().decode(errors="replace");client.handle_refusal(detail,preserve_update_errors=True)
        raise RuntimeError("Update server refused request: "+detail) from error
    finally: temp.unlink(missing_ok=True)
    return {'downloaded':True,'version':release['version'],'path':str(target),'sha256':release['sha256']}

def authorize():
    cache,pending=selected();release=pending['release'];target=client.STATE/(release['sha256']+'.package')
    # Recheck the actual file immediately before install; never trust a flag.
    digest=hashlib.sha256();count=0
    with target.open('rb') as source:
        for chunk in iter(lambda:source.read(65536),b''):digest.update(chunk);count+=len(chunk)
    if count!=release['size'] or digest.hexdigest()!=release['sha256']: raise RuntimeError('Package was modified after download')
    result=client.send('/v2/updates/'+release['id']+'/authorize',pending['body'],preserve_update_errors=True)
    manifest(result['manifest_jwt'],cache,pending['body'],release)
    return {'authorized':True,'version':release['version'],'path':str(target),'sha256':release['sha256']}

if __name__=='__main__':
    operation=sys.argv[1] if len(sys.argv)>1 else 'check'
    if operation not in ('check','download','authorize'): raise RuntimeError('Unknown update operation')
    print(json.dumps({'check':check,'download':download,'authorize':authorize}[operation]()))
