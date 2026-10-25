#!/usr/bin/env python3
"""Compile and run the actual per-product projects downloaded from the developer page."""
import argparse
import datetime
import hashlib
import importlib.util
import io
import json
import os
import pathlib
import secrets
import shlex
import signal
import sqlite3
import subprocess
import tempfile
import time
import urllib.parse
import uuid
import zipfile

REPO = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("integration_http", REPO / "examples/update-demo/test_e2e.py")
http = importlib.util.module_from_spec(spec)
spec.loader.exec_module(http)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server", required=True, type=pathlib.Path)
    parser.add_argument("--artifacts", required=True, type=pathlib.Path)
    parser.add_argument("--screenshots", action="store_true")
    parser.add_argument("--chromium", default="/usr/bin/chromium")
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    report = {"result": "running", "checks": [], "languages": []}
    password = secrets.token_hex(24)
    server = browser = playwright = None
    sites = []
    logs = []
    with tempfile.TemporaryDirectory(prefix="integration-") as temporary, (args.artifacts / "server.log").open("w") as server_log:
        try:
            folder = pathlib.Path(temporary)
            port = http.free_port()
            base = f"http://127.0.0.1:{port}"
            database = folder / "license.sqlite"
            env = os.environ.copy()
            env.update(YN_ADDR=f"127.0.0.1:{port}", YN_DB_DRIVER="sqlite", YN_DB=f"file:{database}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", YN_DB_CONFIG_FILE=str(folder / "config.json"), YN_RELEASE_DIR=str(folder / "releases"), YN_SDK_DIR=str(REPO / "sdk"))
            server = subprocess.Popen([str(args.server.resolve())], cwd=REPO / "backend", env=env, stdout=server_log, stderr=server_log)
            http.wait_ready(base, "/healthz", server)
            http.initialize(base, "admin", password, env["YN_DB"])
            root = http.api(base, "/admin/login", {"username": "admin", "password": password})["access_token"]
            for name in ["developer", "other-developer"]:
                http.api(base, "/admin/users", {"username": name, "password": password, "role": "developer"}, root)
            developer = http.api(base, "/developer/login", {"username": "developer", "password": password})["access_token"]
            other = http.api(base, "/developer/login", {"username": "other-developer", "password": password})["access_token"]
            page = None
            http_example = http_command = None
            errors = []
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                playwright = sync_playwright().start()
                browser = playwright.chromium.launch(executable_path=None if args.chromium == "bundled" else args.chromium, headless=True, args=["--no-sandbox"])
                page = browser.new_page(viewport={"width": 1440, "height": 1080}, locale="zh-CN")
                page.on("pageerror", lambda error: errors.append(str(error)))
                page.goto(base + "/developer-console/")
                page.get_by_label("账号", exact=True).fill("developer")
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="登录", exact=True).click()
                page.get_by_label("产品名称", exact=True).fill("Ebox 接入验收")
                assert page.get_by_label("产品代码", exact=True).count() == 0
                with page.expect_response(lambda r: r.url.endswith("/developer/products") and r.request.method == "POST") as response:
                    page.get_by_role("button", name="创建产品", exact=True).click()
                assert response.value.status == 200
                node_product = response.value.json()["data"]
                page.get_by_role("heading", name="接入授权 · Ebox 接入验收", exact=True).wait_for()
                page.context.grant_permissions(["clipboard-read", "clipboard-write"], origin=base)
                page.get_by_role("button", name="复制应用 ID", exact=True).click()
                assert page.evaluate("navigator.clipboard.readText()") == node_product["id"]
            else:
                node_product = http.api(base, "/developer/products", {"name": "Ebox 接入验收", "bind_mode": "domain"}, developer)
            assert node_product["id"].startswith("prod_") and node_product["app_key"] == node_product["id"]

            def kit_url(product, language, token=developer):
                query = urllib.parse.urlencode({"base_url": base, "language": language, "bind_value": "ebox.example.com"})
                return "/developer/products/" + product["id"] + "/integration-kit?" + query

            http.http(base, kit_url(node_product, "node"), token=other, expected=404)
            http.http(base, kit_url(node_product, "../../outside"), token=developer, expected=400)
            http.http(base, kit_url(node_product, "node"), expected=401)
            report["checks"].append("Developer product isolation, authentication and archive language validation are enforced")

            def cli(language, project, *arguments, expected=0, expected_code=None):
                command = {"node": ["node", "app.mjs", *arguments], "go": ["go", "run", ".", *arguments], "java": ["mvn", "-q", "compile", "exec:java", "-Dexec.args=" + " ".join(arguments)]}[language]
                result = subprocess.run(command, cwd=project, env=os.environ.copy(), capture_output=True, text=True, timeout=180)
                assert (result.returncode == 0) == (expected == 0), f"{language} {arguments[0]}: {(result.stderr + result.stdout)[-2200:]}"
                if expected:
                    if expected_code:
                        assert expected_code in result.stdout + result.stderr, f"{language}: missing expected error {expected_code}"
                    return None
                for line in reversed(result.stdout.splitlines()):
                    try:
                        return json.loads(line)
                    except json.JSONDecodeError:
                        pass
                raise AssertionError(f"{language} returned no result for {arguments[0]}")

            def start_site(language, project):
                demo_port = http.free_port()
                site_env = os.environ.copy()
                site_env["DEMO_PORT"] = str(demo_port)
                command = {"node": ["node", "app.mjs", "serve"], "go": ["go", "run", ".", "serve"], "java": ["mvn", "-q", "compile", "exec:java", "-Dexec.args=serve"]}[language]
                log = (args.artifacts / f"{language}-site-{len(sites)}.log").open("w")
                logs.append(log)
                process = subprocess.Popen(command, cwd=project, env=site_env, stdout=log, stderr=log, start_new_session=True)
                site = f"http://127.0.0.1:{demo_port}"
                sites.append((process, site))
                http.wait_ready(site, "/protected", process)
                return site

            for language in ["node", "go", "java"]:
                product = node_product if language == "node" else http.api(base, "/developer/products", {"name": "接入验收 " + language, "bind_mode": "domain"}, developer)
                project = folder / language
                project.mkdir()
                if page:
                    page.reload()
                    page.get_by_label("选择产品", exact=False).select_option(product["id"])
                    guide = page.locator(".integration-guide")
                    guide.get_by_label("接入语言", exact=False).select_option(language)
                    guide.get_by_label("客户端实际绑定值", exact=True).fill("ebox.example.com")
                    with page.expect_download() as download:
                        guide.get_by_role("button", name=f"下载 {dict(node='Node.js',go='Go',java='Java')[language]} 接入包", exact=True).click()
                    content = pathlib.Path(download.value.path()).read_bytes()
                    if language == "node":
                        guide.screenshot(path=str(args.artifacts / "developer-application-integration.png"))
                        guide.get_by_role("button", name="HTTP 接口", exact=True).click()
                        example = json.loads(guide.locator('[data-example="activate"]').inner_text())
                        assert example["app_key"] == product["id"] and example["bind_mode"] == "domain"
                        http_example=example
                        http_command=shlex.split(guide.locator('[data-example="activate-command"]').inner_text())
                        for width in [390, 320]:
                            page.set_viewport_size({"width": width, "height": 844})
                            assert page.evaluate("document.documentElement.scrollWidth<=innerWidth+1")
                        page.screenshot(path=str(args.artifacts / "mobile-integration-http.png"), full_page=True, animations="disabled")
                        page.set_viewport_size({"width": 1440, "height": 1080})
                else:
                    content = http.http(base, kit_url(product, language), token=developer)
                with zipfile.ZipFile(io.BytesIO(content)) as archive:
                    assert all(not name.startswith("/") and ".." not in pathlib.PurePosixPath(name).parts for name in archive.namelist())
                    assert all(secret.encode() not in archive.read(name) for name in archive.namelist() for secret in [root, developer, other, password, "-----BEGIN PRIVATE KEY-----"])
                    archive.extractall(project)
                config = json.loads((project / "config.json").read_text())
                assert config["app_key"] == product["id"] and config["product_code"] == product["code"] and config["bind_mode"] == "domain"
                codes = http.api(base, "/developer/products/" + product["id"] + "/card-batches", {"name": "接入测试卡", "quantity": 2, "duration_days": 30}, developer)["codes"]
                if language=="node" and http_command:
                    http_example["card_code"]=codes[0]
                    (project/"activate.json").write_text(json.dumps(http_example))
                    result=subprocess.run(http_command,cwd=project,capture_output=True,text=True,timeout=30)
                    assert result.returncode==0 and json.loads(result.stdout)["data"]["can_use"]
                    (project/"activate.json").unlink()
                    report["checks"].append("The exact personalized curl command and request JSON copied from the page activate a real card successfully")
                license = cli(language, project, "activate", codes[0])
                assert license["can_use"] and license["can_update"]
                assert cli(language, project, "verify")["can_use"]
                assert cli(language, project, "heartbeat")["accepted"]
                assert cli(language, project, "renew", codes[1])["can_use"]
                site = start_site(language, project)
                assert http.http(site, "/protected")["can_use"]
                changed = {**config, "bind_value": "wrong.example.com"}
                (project / "config.json").write_text(json.dumps(changed))
                cli(language, project, "verify", expected=1)
                (project / "config.json").write_text(json.dumps(config))
                http.api(base, "/admin/licenses/revoke", {"license_no":license["license_no"], "reason": "真实接入包吊销验证"}, root)
                http.http(site, "/protected", expected=403)
                cli(language, project, "verify", expected=1)
                http.api(base,"/admin/licenses/restore",{"license_no":license["license_no"],"reason":"恢复后单独验证到期边界"},root)
                assert http.http(site,"/protected")["can_use"]
                past = (datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=1)).isoformat().replace("+00:00", "Z")
                with sqlite3.connect(database) as db:
                    db.execute("UPDATE licenses SET status='active',expired_at=? WHERE license_no=?", (past, license["license_no"]))
                    db.execute("UPDATE license_entitlements SET updates_expired_at=? WHERE license_id=(SELECT id FROM licenses WHERE license_no=?)", (past, license["license_no"]))
                http.http(site, "/protected", expected=403)
                cli(language, project, "verify", expected=1, expected_code="LICENSE_EXPIRED")
                report["languages"].append(language)
                report["checks"].append(f"{language}: downloaded project builds, activates, verifies, heartbeats, renews and rejects wrong bindings, revocation and usage expiry at its real business endpoint")
                permanent = http.api(base, "/developer/products", {"name":"永久使用验收 "+language,"bind_mode":"domain","license_mode":"perpetual"},developer)
                with zipfile.ZipFile(io.BytesIO(http.http(base,kit_url(permanent,language),token=developer))) as archive:
                    (project/"config.json").write_bytes(archive.read("config.json"))
                permanent_card=http.api(base,"/developer/products/"+permanent["id"]+"/card-batches",{"name":"永久使用限期更新","quantity":1,"duration_days":30},developer)["codes"][0]
                permanent_license=cli(language,project,"activate",permanent_card)
                assert permanent_license["can_use"] and permanent_license["usage_expired_at"] is None
                if language=="node":
                    http.api(base,"/developer/products/"+permanent["id"]+"/updates",{"enabled":True},developer)
                    package=http.archive("2.0.0",2)
                    http.publish(base,permanent,developer,"2.0.0",package)
                    assert cli(language,project,"updates","1.0.0")["update_available"]
                    assert cli(language,project,"download","1.0.0")["downloaded"]
                    assert hashlib.sha256((project/"update.zip").read_bytes()).digest()==hashlib.sha256(package).digest()
                    (project/"update.zip").unlink()
                with sqlite3.connect(database) as db:
                    db.execute("UPDATE license_entitlements SET updates_expired_at=? WHERE license_id=(SELECT id FROM licenses WHERE license_no=?)",(past,permanent_license["license_no"]))
                current=cli(language,project,"verify")
                assert current["can_use"] and current["can_update"] is False
                permanent_site=start_site(language,project)
                rights=http.http(permanent_site,"/protected")
                assert rights["can_use"] and rights["can_update"] is False
                if language=="node":
                    cli(language,project,"download","1.0.0",expected=1,expected_code="UPDATES_EXPIRED")
                    assert not (project/"update.zip").exists()
                report["checks"].append(f"{language}: perpetual usage stays available after update entitlement expires; Node upgrade download additionally verifies signed bytes and refuses expired updates")

            if page:
                page.goto(base + "/admin#products")
                page.get_by_label("账号", exact=True).fill("admin")
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="安全登录", exact=True).click()
                row = page.get_by_role("row").filter(has=page.get_by_role("cell", name=node_product["name"], exact=True))
                row.get_by_role("button", name="接入授权", exact=True).click()
                page.get_by_role("heading", name="接入授权 · Ebox 接入验收", exact=True).wait_for()
                page.locator(".integration-guide").screenshot(path=str(args.artifacts / "admin-integration-entry.png"))
                page.get_by_role("button", name="新增产品", exact=True).click()
                dialog = page.get_by_role("dialog", name="新增产品", exact=True)
                assert dialog.locator('input[name="code"]').count() == 0
                dialog.get_by_label("产品名称", exact=True).fill("管理员创建应用")
                with page.expect_response(lambda r: r.url.endswith("/admin/products") and r.request.method == "POST") as response:
                    dialog.get_by_role("button", name="确认新增", exact=True).click()
                assert response.value.status == 200
                admin_product = response.value.json()["data"]
                assert admin_product["id"] != node_product["id"] and admin_product["app_key"] == admin_product["id"]
                page.get_by_role("heading", name="接入授权 · 管理员创建应用", exact=True).wait_for()
                page.locator(".integration-guide").evaluate("e => e.scrollIntoView({behavior:'instant',block:'start'})")
                page.locator(".integration-guide").screenshot(path=str(args.artifacts / "admin-generated-application-id.png"))
                report["checks"].append("Both developer and administrator forms create applications without a manual code and automatically show the unique generated application ID")
                assert not errors, errors
            server.terminate()
            server.wait(timeout=10)
            for _, site in sites:
                http.http(site, "/protected", expected=503)
            report["checks"].append("All three running product demos refuse protected business with HTTP 503 when the authorization platform is unavailable")
            report["result"] = "passed"
            print(json.dumps({"result": "passed", "languages": report["languages"], "checks": len(report["checks"])}))
        finally:
            if report["result"] != "passed":
                report["result"] = "failed"
            (args.artifacts / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
            if browser:
                browser.close()
            if playwright:
                playwright.stop()
            for process, _ in sites:
                if process.poll() is None:
                    os.killpg(process.pid, signal.SIGTERM)
                    process.wait(timeout=10)
            for log in logs:
                log.close()
            if server and server.poll() is None:
                server.terminate()
                server.wait(timeout=10)


if __name__ == "__main__":
    main()
