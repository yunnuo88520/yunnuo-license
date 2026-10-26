#!/usr/bin/env python3
"""Unified customer identity, atomic agency onboarding and scoped business acceptance."""
import argparse
import importlib.util
import json
import os
import pathlib
import secrets
import subprocess
import tempfile
import uuid
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor

REPO = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("demo_http", REPO / "examples/update-demo/test_e2e.py")
http = importlib.util.module_from_spec(spec)
spec.loader.exec_module(http)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server", type=pathlib.Path)
    parser.add_argument("--base-url")
    parser.add_argument("--artifacts", required=True, type=pathlib.Path)
    parser.add_argument("--screenshots", action="store_true")
    parser.add_argument("--chromium", default="/usr/bin/chromium")
    parser.add_argument("--verify-state", type=pathlib.Path)
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    report = {"result": "running", "checks": []}
    password = os.environ["YN_TEST_ADMIN_PASSWORD"] if args.base_url else secrets.token_hex(24)
    suffix = uuid.uuid4().hex[:8]
    process = browser = pw = None
    with tempfile.TemporaryDirectory(prefix="agent-identity-") as temporary, (args.artifacts / "server.log").open("w") as log:
        try:
            base = args.base_url
            if not base:
                port = http.free_port()
                base = f"http://127.0.0.1:{port}"
                env = os.environ.copy()
                env.update(YN_ADDR=f"127.0.0.1:{port}", YN_DB_DRIVER="sqlite", YN_DB=f"file:{temporary}/db.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", YN_DB_CONFIG_FILE=temporary + "/config.json", YN_RELEASE_DIR=temporary + "/packages")
                process = subprocess.Popen([str(args.server.resolve())], cwd=REPO / "backend", env=env, stdout=log, stderr=log)
                http.wait_ready(base, "/healthz", process)
                http.initialize(base, "admin", password, env["YN_DB"])
            root = http.api(base, "/admin/login", {"username": "admin", "password": password})["access_token"]
            if args.verify_state:
                saved = json.loads(args.verify_state.read_text())
                login = http.api(base, "/agent/login", {"username": saved["username"], "password": password})
                assert http.api(base, "/user/profile", token=login["access_token"])["id"] == saved["user_id"]
                account = http.api(base, "/agent/account", token=login["access_token"])
                assert account["agent_id"] == saved["agent_id"] and account["balance"] == 1300
                assert len(http.api(base, "/agent/money-ledgers", token=login["access_token"])) == 2
                print(json.dumps({"result": "passed", "check": "Unified identity, balance and ledger survive MySQL restart"}))
                report = saved
                return
            product = http.api(base, "/admin/products", {"name": "Ebox 企业网站", "bind_mode": "domain"}, root)
            pkg = http.api(base, "/admin/packages", {"product_id": product["id"], "name": "月度授权", "duration_days": 30, "base_cost": 1000}, root)
            tier = http.api(base, "/admin/agent-tiers", {"name": "金牌代理"}, root)
            http.api(base, "/admin/agent-tiers/" + tier["id"] + "/prices", {"package_id": pkg["id"], "unit_cost": 700}, root)
            username = "seller-" + suffix
            customer = http.api(base, "/admin/customers", {"username": username, "password": password, "display_name": "小王", "email": username + "@example.com"}, root)
            old_token = http.api(base, "/user/login", {"username": username, "password": password})["access_token"]
            card = http.api(base, "/admin/card-batches", {"product_id": product["id"], "quantity": 1, "duration_days": 30}, root)["codes"][0]
            owned = http.api(base, "/user/activate", {"card_code": card, "bind_value": "owner.example.com"}, old_token)
            http.http(base, "/agent/profile", token=old_token, expected=403)
            opening = {"user_id": customer["id"], "name": "小王软件服务", "tier_id": tier["id"], "package_ids": [pkg["id"]], "initial_balance": 2000, "reason": "代理预存款", "max_batch_quantity": 10}
            errors = []
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                pw = sync_playwright().start()
                browser = pw.chromium.launch(executable_path=None if args.chromium == "bundled" else args.chromium, headless=True, args=["--no-sandbox"])
                context = browser.new_context(viewport={"width": 1500, "height": 1100}, locale="zh-CN", timezone_id="Asia/Shanghai")
                page = context.new_page()
                page.on("pageerror", lambda e: errors.append(str(e)))
                page.goto(base + "/admin#agents")
                page.get_by_label("账号", exact=True).fill("admin")
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="安全登录", exact=True).click()
                page.get_by_role("button", name="开通代理账号", exact=True).click()
                dialog = page.get_by_role("dialog", name="开通代理账号", exact=True)
                dialog.get_by_label("账号来源").select_option("existing")
                dialog.get_by_label("选择已有用户").select_option(customer["id"])
                dialog.get_by_label("代理显示名称").fill(opening["name"])
                dialog.get_by_label("代理等级").select_option(tier["id"])
                dialog.get_by_role("checkbox", name="Ebox 企业网站 / 月度授权").check()
                dialog.get_by_label("初始预存余额（分）").fill("2000")
                dialog.get_by_label("余额登记原因").fill(opening["reason"])
                dialog.get_by_label("单批发卡上限").fill("10")
                page.screenshot(path=str(args.artifacts / "admin-one-step-onboarding.png"), full_page=True)
                for width in [390, 320]:
                    page.set_viewport_size({"width": width, "height": 844})
                    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth+1")
                    if width == 390:
                        page.screenshot(path=str(args.artifacts / "admin-onboarding-mobile.png"), full_page=True)
                page.set_viewport_size({"width": 1500, "height": 1100})
                with page.expect_response(lambda r: r.url.endswith("/admin/agent-accounts") and r.request.method == "POST") as response:
                    dialog.get_by_role("button", name="确认开通代理", exact=True).click()
                assert response.value.status == 200, response.value.text()
                opened = response.value.json()["data"]
                dialog.wait_for(state="hidden")
                page.get_by_text("可用余额", exact=False).wait_for()
                page.screenshot(path=str(args.artifacts / "admin-unified-agent-detail.png"), full_page=True)
            else:
                opened = http.api(base, "/admin/agent-accounts", opening, root)
            agency = opened["agent"]
            assert opened["user"]["id"] == customer["id"]
            assert http.api(base, "/agent/profile", token=old_token)["agent_id"] == agency["id"]
            assert http.api(base, "/user/licenses/" + owned["license_no"], token=old_token)["can_use"]
            http.http(base, "/admin/agent-accounts", opening, root, expected=409)
            assert http.api(base, "/agent/account", token=old_token)["balance"] == 2000
            report["checks"].append("One-step existing-user onboarding preserves identity, original session and personal licenses; duplicate requests cannot grant twice")
            token = http.api(base, "/agent/login", {"username": username, "password": password})["access_token"]
            assert http.api(base, "/agent/packages", token=token)[0]["unit_cost"] == 700
            if args.screenshots:
                seller = browser.new_context(viewport={"width": 1440, "height": 1000}, color_scheme="dark", locale="zh-CN")
                user = seller.new_page()
                user.on("pageerror", lambda e: errors.append(str(e)))
                user.goto(base + "/agent-console/")
                assert user.get_by_label("代理代码", exact=True).count() == 0
                user.get_by_label("登录名", exact=True).fill(username)
                user.get_by_label("密码", exact=True).fill(password)
                user.get_by_role("button", name="进入工作台", exact=True).click()
                user.get_by_role("link", name="管理我购买的授权", exact=True).click()
                user.get_by_text("Ebox 企业网站", exact=False).first.wait_for()
                assert user.get_by_role("button", name="登录用户中心", exact=True).count() == 0
                user.screenshot(path=str(args.artifacts / "same-account-personal-licenses-dark.png"), full_page=True)
                user.get_by_role("link", name="代理工作台", exact=True).click()
                user.locator(".console .workspace").wait_for()
                user.locator(".metric").filter(has_text="可用余额").get_by_text("¥20.00", exact=True).wait_for()
                user.get_by_role("cell", name="¥7.00", exact=True).wait_for()
                user.set_viewport_size({"width": 390, "height": 844})
                assert user.evaluate("document.documentElement.scrollWidth <= innerWidth+1")
                user.screenshot(path=str(args.artifacts / "same-account-agent-mobile-dark.png"), full_page=True)
            report["checks"].append("No agency-code login; one token works in both portals and browser navigation needs no second login on desktop/mobile")
            issue = {"package_id": pkg["id"], "quantity": 1, "request_id": uuid.uuid4().hex, "expected_unit_cost": 700}
            cards = http.api(base, "/agent/card-batches", issue, token)
            assert http.api(base, "/agent/card-batches", issue, token)["batch"]["id"] == cards["batch"]["id"]
            assert http.api(base, "/agent/account", token=token)["balance"] == 1300
            sold = http.api(base, "/v1/licenses/activate", {"app_key": product["app_key"], "card_code": cards["codes"][0], "bind_mode": "domain", "bind_value": "buyer.example.com"})
            if args.screenshots:
                page.get_by_role("button", name="开通代理账号", exact=True).click()
                dialog.get_by_label("登录账号", exact=True).fill("other-"+suffix)
                dialog.get_by_label("初始密码", exact=True).fill(password)
                dialog.get_by_label("用户显示名称").fill("独立代理用户")
                page.emulate_media(color_scheme="dark")
                page.set_viewport_size({"width": 390, "height": 844})
                assert page.evaluate("document.documentElement.scrollWidth <= innerWidth+1")
                page.screenshot(path=str(args.artifacts / "admin-new-agent-mobile-dark.png"), full_page=True)
                with page.expect_response(lambda r: r.url.endswith("/admin/agent-accounts") and r.request.method == "POST") as response:
                    dialog.get_by_role("button", name="确认开通代理", exact=True).click()
                assert response.value.status == 200
                other = response.value.json()["data"]
            else:
                other = http.api(base, "/admin/agent-accounts", {"username": "other-"+suffix, "password": password}, root)
            other_token = http.api(base, "/user/login", {"username": other["user"]["username"], "password": password})["access_token"]
            assert http.api(base, "/agent/account?agent_id="+agency["id"], token=other_token)["balance"] == 0
            http.http(base, "/agent/card-batches/"+cards["batch"]["id"]+"/cards", token=other_token, expected=404)
            http.http(base, "/admin/agent-accounts", opening, token, expected=401)
            report["checks"].append("Tier price is 700 cents; retry issues no extra cards or debits; a different agent cannot read the wallet or batch, and neither gains administrator rights")
            prefix = "/admin/agents/" + agency["id"]
            http.api(base, prefix + "/suspend", {}, root)
            http.http(base, "/agent/profile", token=token, expected=403)
            http.http(base, "/agent/card-batches", {**issue, "request_id": uuid.uuid4().hex}, token, expected=403)
            assert http.api(base, "/user/licenses/" + owned["license_no"], token=token)["can_use"]
            assert http.api(base, "/v1/licenses/verify", {"app_key": product["app_key"], "license_no": sold["license_no"], "bind_mode": "domain", "bind_value": "buyer.example.com"})["can_use"]
            http.api(base, prefix + "/enable", {}, root)
            report["checks"].append("Pausing the agency denies new business immediately while personal and already-sold licenses remain usable")
            new_password = secrets.token_hex(24)
            http.api(base, "/user/password", {"current_password": password, "new_password": new_password}, token)
            for path in ["/user/profile", "/agent/profile"]:
                http.http(base, path, token=token, expected=401)
            fresh = http.api(base, "/agent/login", {"username": username, "password": new_password})["access_token"]
            http.api(base, "/user/profile", token=fresh)
            http.api(base, "/user/password", {"current_password": new_password, "new_password": password}, fresh)
            report["checks"].append("Changing the personal password revokes both portal sessions and the same new password works for agent login")
            # Repeat against real MySQL in Docker, using parallel HTTP requests.
            candidate = http.api(base, "/admin/customers", {"username":"race-"+suffix,"password":password}, root)
            def parallel_open(_):
                payload = json.dumps({"user_id":candidate["id"],"initial_balance":1000,"reason":"Concurrent opening"}).encode()
                request = urllib.request.Request(base+"/admin/agent-accounts",data=payload,headers={"Content-Type":"application/json","Authorization":"Bearer "+root})
                try:
                    response = urllib.request.urlopen(request,timeout=30)
                except urllib.error.HTTPError as error:
                    response = error
                with response:
                    response.read()
                    return response.status
            with ThreadPoolExecutor(max_workers=2) as pool:
                assert sorted(pool.map(parallel_open,range(2))) == [200,409]
            race_token = http.api(base, "/user/login", {"username":candidate["username"],"password":password})["access_token"]
            assert len(http.api(base,"/agent/money-ledgers",token=race_token)) == 1
            report["checks"].append("Concurrent HTTP onboarding creates one qualification and one initial credit")
            assert not errors, errors
            report.update(result="passed", username=username, user_id=customer["id"], agent_id=agency["id"])
            (args.artifacts / "failure.png").unlink(missing_ok=True)
            print(json.dumps({"result": "passed", "checks": len(report["checks"])}))
        except Exception:
            if args.screenshots and "page" in locals():
                page.screenshot(path=str(args.artifacts / "failure.png"), full_page=True)
            raise
        finally:
            if report["result"] != "passed":
                report["result"] = "failed"
            (args.artifacts / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
            if browser:
                browser.close()
            if pw:
                pw.stop()
            if process:
                process.terminate()
                process.wait(timeout=10)


if __name__ == "__main__":
    main()
