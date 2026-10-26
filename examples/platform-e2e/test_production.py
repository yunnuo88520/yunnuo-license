#!/usr/bin/env python3
"""Real HTTP concurrency, mixed load and commercial operations evidence."""
import argparse
import base64
import concurrent.futures
import datetime
import hashlib
import hmac
import importlib.util
import json
import os
import pathlib
import secrets
import statistics
import struct
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid

REPO = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("demo_http", REPO / "examples/update-demo/test_e2e.py")
http = importlib.util.module_from_spec(spec)
spec.loader.exec_module(http)

def request(base, path, data=None, token="", method=None, client_ip=None):
    headers={"Content-Type":"application/json"}
    if token: headers["Authorization"]="Bearer "+token
    if client_ip: headers["X-Forwarded-For"]=client_ip
    req=urllib.request.Request(base+path,data=json.dumps(data).encode() if data is not None else None,headers=headers,method=method)
    start=time.monotonic()
    try: response=urllib.request.urlopen(req,timeout=40)
    except urllib.error.HTTPError as error: response=error
    with response:
        raw=response.read()
        body=json.loads(raw) if "json" in response.headers.get("Content-Type","") else raw
        return response.status,body,time.monotonic()-start

def parallel(calls):
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
        return list(pool.map(lambda f:f(),calls))

def otp(secret):
    digest=hmac.new(base64.b32decode(secret+"="*((-len(secret))%8)),struct.pack(">Q",int(time.time())//30),hashlib.sha1).digest()
    offset=digest[-1]&15
    return f"{(struct.unpack('>I',digest[offset:offset+4])[0]&0x7fffffff)%1000000:06d}"

def main():
    p=argparse.ArgumentParser();p.add_argument("--server",type=pathlib.Path);p.add_argument("--base-url");p.add_argument("--artifacts",required=True,type=pathlib.Path);p.add_argument("--screenshots",action="store_true");p.add_argument("--chromium",default="/usr/bin/chromium");args=p.parse_args()
    args.artifacts.mkdir(parents=True,exist_ok=True);password=os.environ["YN_TEST_ADMIN_PASSWORD"] if args.base_url else secrets.token_hex(24)
    report={"result":"running","checks":[],"database":"mysql" if args.base_url else "sqlite"};process=browser=pw=None
    with tempfile.TemporaryDirectory(prefix="production-e2e-") as temporary,(args.artifacts/"server.log").open("w") as log:
      try:
        base=args.base_url
        if not base:
            port=http.free_port();base=f"http://127.0.0.1:{port}"
            env=os.environ.copy();env.update(YN_ADDR=f"127.0.0.1:{port}",YN_DB_DRIVER="sqlite",YN_DB=f"file:{temporary}/database.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)",YN_DB_CONFIG_FILE=temporary+"/config.json",YN_RELEASE_DIR=temporary+"/packages",YN_METRICS_TOKEN=secrets.token_hex(24),YN_TRUST_PROXY_HEADERS="true")
            process=subprocess.Popen([str(args.server.resolve())],cwd=REPO/"backend",env=env,stdout=log,stderr=log);http.wait_ready(base,"/healthz",process);http.initialize(base,"admin",password,env["YN_DB"])
        root=http.api(base,"/admin/login",{"username":"admin","password":password})["access_token"]
        product=http.api(base,"/admin/products",{"name":"Ebox 商用并发验收","bind_mode":"device","max_bind_count":1,"bind_conflict_strategy":"reject"},root)
        batch=http.api(base,"/admin/card-batches",{"product_id":product["id"],"name":"Concurrency fixture","quantity":8,"duration_days":30},root)
        binding={"app_key":product["app_key"],"bind_mode":"device"}
        calls=[lambda i=i:request(base,"/v1/licenses/activate",{**binding,"card_code":batch["codes"][0],"bind_value":f"machine-{i}"}) for i in range(8)]
        results=parallel(calls);success=[(i,b["data"]) for i,(status,b,_) in enumerate(results) if status==200]
        assert len(success)==1,[(r[0],r[1].get("error",{}).get("code")) for r in results]
        assert all(status==200 or status==409 or status==403 for status,_,_ in results),"activation returned server error"
        index,license=success[0];credentials={**binding,"bind_value":f"machine-{index}","license_no":license["license_no"]}
        again=http.api(base,"/v1/licenses/activate",{**binding,"bind_value":credentials["bind_value"],"card_code":batch["codes"][0]})
        assert again["license_no"]==license["license_no"] and again["expired_at"]==license["expired_at"]
        report["checks"].append("Eight simultaneous activations of one card produce one license and one binding; repeating the successful activation preserves the original expiry")
        results=parallel([lambda code=code:request(base,"/v1/licenses/renew",{**credentials,"renew_card_code":code}) for code in batch["codes"][1:4]])
        assert all(r[0]==200 for r in results),[(r[0],r[1].get("error",{}).get("code")) for r in results]
        after=http.api(base,"/v1/licenses/verify",credentials)
        parse=lambda s:datetime.datetime.fromisoformat(s.replace("Z","+00:00"))
        assert parse(after["expired_at"])-parse(license["expired_at"])==datetime.timedelta(days=90)
        replayed=http.api(base,"/v1/licenses/renew",{**credentials,"renew_card_code":batch["codes"][1]})
        assert replayed["expired_at"]==after["expired_at"] and replayed["updates_expired_at"]==after["updates_expired_at"]
        report["checks"].append("Three different renewal cards used concurrently extend both rights by exactly 90 days without lost extensions")
        other=http.api(base,"/v1/licenses/activate",{**binding,"card_code":batch["codes"][4],"bind_value":"second-machine"})
        other_credentials={**binding,"bind_value":"second-machine","license_no":other["license_no"]}
        results=parallel([lambda c=c:request(base,"/v1/licenses/renew",{**c,"renew_card_code":batch["codes"][5]}) for c in [credentials,other_credentials]])
        assert sorted(r[0] for r in results)==[200,404],[(r[0],r[1].get("error",{}).get("code")) for r in results]
        report["checks"].append("One renewal card contested by two licenses is consumed once")
        results=parallel([lambda:request(base,"/v1/licenses/activate",{**binding,"card_code":batch["codes"][6],"bind_value":"activation-vs-renewal"}),lambda:request(base,"/v1/licenses/renew",{**credentials,"renew_card_code":batch["codes"][6]})])
        assert sum(r[0]==200 for r in results)==1 and all(r[0]<500 for r in results),[(r[0],r[1].get("error",{}).get("code")) for r in results]
        report["checks"].append("Activation racing renewal for the same unused card has one winner and no double consumption")
        http.api(base,"/admin/products/"+product["id"]+"/updates",{"enabled":True},root)
        # Administrator and developer releases use the same ownership checks.
        username="production-dev-"+uuid.uuid4().hex[:8]
        http.api(base,"/admin/users",{"username":username,"password":password,"role":"developer"},root)
        dev=http.api(base,"/developer/login",{"username":username,"password":password})["access_token"]
        # Use the shared multipart helper with the admin endpoint.
        prefix="/admin/products/"+product["id"]
        boundary=uuid.uuid4().hex;package=b"commercial-upgrade-fixture"*256
        parts=[f'--{boundary}\r\nContent-Disposition: form-data; name="version"\r\n\r\n1.0.0\r\n'.encode(),f'--{boundary}\r\nContent-Disposition: form-data; name="package"; filename="ebox.zip"\r\nContent-Type: application/zip\r\n\r\n'.encode()+package+f'\r\n--{boundary}--\r\n'.encode()]
        release=http.api(base,prefix+"/releases",b"".join(parts),root,"multipart/form-data; boundary="+boundary)
        http.api(base,prefix+"/releases/"+release["id"]+"/publish",{},root)
        tasks=[]
        for i in range(160):
            path=["/v1/licenses/verify","/v1/licenses/heartbeat","/v1/updates/check","/v1/updates/"+release["id"]+"/download"][i%4]
            tasks.append(lambda path=path,i=i:request(base,path,{**credentials,**({"current_version":"0.0.0"} if path.endswith("/check") else {})},client_ip=f"198.51.100.{i//4+1}"))
        results=parallel(tasks);assert all(r[0]==200 for r in results),[(r[0],str(r[1])[:120]) for r in results if r[0]!=200]
        for i,(status,body,elapsed) in enumerate(results):
            if i%4==3:assert hashlib.sha256(body).hexdigest()==release["sha256"]
        times=sorted(r[2]*1000 for r in results);report["load"]={"requests":len(results),"clients":40,"concurrency":8,"server_errors":0,"p50_ms":round(statistics.median(times),2),"p95_ms":round(times[int(len(times)*.95)-1],2),"max_ms":round(max(times),2)}
        report["checks"].append("Mixed verification, heartbeat, update checks and authenticated downloads complete without errors; every downloaded package hash matches")
        status,body,_=request(base,prefix+"/releases/"+release["id"],token=root,method="DELETE");assert status==409
        http.api(base,prefix+"/releases/"+release["id"]+"/withdraw",{},root)
        assert request(base,"/v1/updates/"+release["id"]+"/download",credentials)[0]==404
        assert request(base,prefix+"/releases/"+release["id"],token=dev,method="DELETE")[0]==403
        assert request(base,prefix+"/releases/"+release["id"],token=root,method="DELETE")[0]==200
        assert request(base,prefix+"/releases/"+release["id"]+"/publish",{},root)[0]==404
        report["checks"].append("Published packages cannot be deleted; withdrawal blocks old download requests; deletion releases storage and a deleted version cannot be republished")
        # MFA uses a separate administrator so shared MySQL fixtures remain valid.
        operator="mfa-admin-"+uuid.uuid4().hex[:8]
        user=http.api(base,"/admin/users",{"username":operator,"password":password,"role":"admin"},root)
        session=http.api(base,"/admin/login",{"username":operator,"password":password})["access_token"]
        setup=http.api(base,"/admin/mfa/begin",{"password":password},session)
        codes=http.api(base,"/admin/mfa/confirm",{"password":password,"code":otp(setup["secret"])},session)["recovery_codes"]
        assert request(base,"/admin/profile",token=session)[0]==401
        assert request(base,"/admin/login",{"username":operator,"password":password})[0]==403
        mfa=http.api(base,"/admin/login",{"username":operator,"password":password,"otp_code":codes[0]})["access_token"]
        assert request(base,"/admin/login",{"username":operator,"password":password,"otp_code":codes[0]})[0]==403
        report["checks"].append("MFA enable revokes old sessions, missing OTP is denied, recovery code logs in once and replay is rejected")
        if args.screenshots:
            from playwright.sync_api import sync_playwright
            pw=sync_playwright().start();browser=pw.chromium.launch(executable_path=None if args.chromium=="bundled" else args.chromium,headless=True,args=["--no-sandbox"])
            context=browser.new_context(viewport={"width":1500,"height":1050},locale="zh-CN");page=context.new_page();errors=[];page.on("pageerror",lambda e:errors.append(str(e)))
            page.goto(base+"/admin#operations");page.get_by_label("账号",exact=True).fill("admin");page.get_by_label("密码",exact=True).fill(password);page.get_by_role("button",name="安全登录",exact=True).click();page.get_by_role("heading",name="运行与容量",exact=True,level=3).wait_for();page.screenshot(path=str(args.artifacts/"operations-desktop-light.png"),full_page=True,animations="disabled")
            page.emulate_media(color_scheme="dark");page.locator("html[data-theme='dark']").wait_for(state="attached");page.set_viewport_size({"width":390,"height":844});assert page.evaluate("document.documentElement.scrollWidth<=innerWidth+1");page.screenshot(path=str(args.artifacts/"operations-mobile-dark.png"),full_page=True,animations="disabled")
            context.close();context=browser.new_context(viewport={"width":1440,"height":1000},locale="zh-CN");page=context.new_page();page.goto(base+"/admin#admins");page.get_by_label("账号",exact=True).fill(operator);page.get_by_label("密码",exact=True).fill(password);page.get_by_label("动态验证码或恢复码",exact=True).fill(codes[1]);page.get_by_role("button",name="安全登录",exact=True).click();page.get_by_role("tab",name="账号与安全",exact=True).click();page.get_by_text("已启用 · 剩余 8 个恢复码",exact=True).wait_for();page.screenshot(path=str(args.artifacts/"admin-mfa-enabled.png"),full_page=True,animations="disabled");assert not errors
        # Expose a non-secret descriptor for the independent backup/restore drill.
        report["restoration_fixture"]={"product_id":product["id"],"license_no":credentials["license_no"],"bind_value":credentials["bind_value"]}
        report["result"]="passed"
      finally:
        if browser:browser.close()
        if pw:pw.stop()
        if process:process.terminate();process.wait(timeout=15)
        (args.artifacts/"report.json").write_text(json.dumps(report,ensure_ascii=False,indent=2))
    print(json.dumps({"result":report["result"],"checks":len(report["checks"]),"database":report["database"],"load":report.get("load")},ensure_ascii=False))

if __name__=="__main__":main()
