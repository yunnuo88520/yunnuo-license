#!/usr/bin/env python3
"""Mandatory first-run installer and management entry change across restart."""
import argparse
import importlib.util
import json
import os
import pathlib
import secrets
import subprocess
import tempfile
import urllib.request

REPO=pathlib.Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location("http_helpers",REPO/"examples/update-demo/test_e2e.py")
http=importlib.util.module_from_spec(spec);spec.loader.exec_module(http)
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,*args): return None

def main():
    parser=argparse.ArgumentParser();parser.add_argument("--server",required=True,type=pathlib.Path);parser.add_argument("--artifacts",required=True,type=pathlib.Path);parser.add_argument("--screenshots",action="store_true");args=parser.parse_args();args.artifacts.mkdir(parents=True,exist_ok=True)
    report={"result":"running","checks":[]};browser=playwright=process=None;password=secrets.token_hex(24)
    with tempfile.TemporaryDirectory(prefix="install-") as temporary,(args.artifacts/"server.log").open("w") as log:
        try:
            folder=pathlib.Path(temporary);port=http.free_port();base=f"http://127.0.0.1:{port}";config=folder/"config.json";env=os.environ.copy();env.pop("YN_DATA_KEY",None);env.pop("YN_CARD_PEPPER",None)
            env.update(YN_ADDR=f"127.0.0.1:{port}",YN_DB_DRIVER="sqlite",YN_DB=f"file:{folder}/bootstrap.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)",YN_DB_CONFIG_FILE=str(config),YN_ADMIN_USERNAME="must-not-bootstrap",YN_ADMIN_PASSWORD=password)
            def start():
                p=subprocess.Popen([str(args.server.resolve())],cwd=REPO/"backend",env=env,stdout=log,stderr=log);http.wait_ready(base,"/healthz",p);return p
            process=start();assert not http.api(base,"/v1/setup/status")["initialized"]
            opener=urllib.request.build_opener(NoRedirect)
            for path in ["/","/admin","/admin/","/agent-console/","/user-console/","/developer-console/"]:
                try:opener.open(base+path);raise AssertionError("first-run page was accessible")
                except urllib.error.HTTPError as e:assert e.code==307 and e.headers["Location"]=="/install"
            assert http.http(base,"/admin/login",{"username":"must-not-bootstrap","password":password},expected=503)["error"]["code"]=="INSTALL_REQUIRED"
            report["checks"].append("First-run pages redirect to /install and environment administrator credentials cannot bypass installation")
            db=folder/"installed.sqlite";setup={"database_driver":"sqlite","database_path":str(db),"admin_username":"platform-owner","admin_password":password,"admin_name":"平台管理员"}
            setup_token=pathlib.Path(str(config)+".install-token").read_text().strip()
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                playwright=sync_playwright().start();browser=playwright.chromium.launch(executable_path="/usr/bin/chromium",headless=True,args=["--no-sandbox"]);page=browser.new_page(viewport={"width":1440,"height":1180},locale="zh-CN")
                errors=[];page.on("pageerror",lambda e:errors.append(str(e)));page.goto(base);assert page.url==base+"/install"
                page.get_by_role("button",name="SQLite",exact=False).click();page.get_by_label("安装令牌",exact=True).fill(setup_token);page.get_by_label("数据库文件路径",exact=True).fill(str(db));page.get_by_label("管理员用户名",exact=True).fill(setup["admin_username"]);page.get_by_label("管理员密码",exact=True).fill(password);page.screenshot(path=str(args.artifacts/"first-run-installer.png"),full_page=True)
                page.get_by_role("button",name="完成安装",exact=True).click();page.get_by_role("heading",name="您的授权平台已就绪",exact=True).wait_for();assert page.get_by_role("link",name="管理后台",exact=False).get_attribute("href")=="/admin";page.screenshot(path=str(args.artifacts/"installation-complete-entries.png"),full_page=True)
            else:http.http(base,"/v1/setup/initialize",setup,extra_headers={"X-Setup-Token":setup_token})
            settings=json.loads(config.read_text());assert settings["admin_path"]=="/admin" and len(settings["data_key"])==64 and len(settings["card_pepper"])==64 and password not in config.read_text();assert config.stat().st_mode&0o077==0
            token=http.api(base,"/admin/login",{"username":setup["admin_username"],"password":password})["access_token"]
            http.http(base,"/v1/setup/initialize",setup,expected=403);http.http(base,"/admin-console/",expected=404)
            report["checks"].append("Database and administrator are installed, private configuration persists keys and completion links, reinstallation is rejected")
            process.terminate();process.wait(timeout=10);settings["admin_path"]="/company-panel";config.write_text(json.dumps(settings));process=start()
            http.http(base,"/admin",expected=404);assert b"\xe7\xae\xa1\xe7\x90\x86\xe6\x8e\xa7\xe5\x88\xb6\xe5\x8f\xb0" in http.http(base,"/company-panel")
            assert http.api(base,"/admin/profile",token=token)["username"]==setup["admin_username"]
            if args.screenshots:
                page.goto(base+"/company-panel");page.get_by_label("账号",exact=True).fill(setup["admin_username"]);page.get_by_label("密码",exact=True).fill(password);page.get_by_role("button",name="安全登录",exact=True).click();page.get_by_role("link",name="仪表盘",exact=True).wait_for();page.screenshot(path=str(args.artifacts/"custom-management-entry.png"),full_page=True);assert not errors,errors
            report["checks"].append("Editing admin_path takes effect after restart, old entry stops serving management and the installed database and session remain valid")
            report["result"]="passed";print(json.dumps(report))
        finally:
            if report["result"]!="passed":report["result"]="failed"
            (args.artifacts/"report.json").write_text(json.dumps(report,ensure_ascii=False,indent=2))
            if browser:browser.close()
            if playwright:playwright.stop()
            if process and process.poll() is None:process.terminate();process.wait(timeout=10)

if __name__=="__main__":main()
