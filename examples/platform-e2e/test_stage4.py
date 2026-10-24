#!/usr/bin/env python3
"""Real app: tampered package detection, manual review, web blocking, appeal and recovery."""
import argparse
import importlib.util
import json
import os
import pathlib
import secrets
import shutil
import subprocess
import tempfile
import uuid

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
    parser.add_argument("--verify-state", type=pathlib.Path)
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    password = os.environ["YN_TEST_ADMIN_PASSWORD"] if args.base_url else secrets.token_hex(24)
    if args.verify_state:
        state = json.loads(args.verify_state.read_text())["persistence"]
        root = http.api(args.base_url, "/admin/login", {"username": "admin", "password": password})["access_token"]
        cases = http.api(args.base_url, "/admin/security/cases", token=root)
        assert next(c for c in cases if c["id"] == state["case_id"])["status"] == "resolved"
        appeals = http.api(args.base_url, "/admin/security/appeals", token=root)
        assert next(a for a in appeals if a["id"] == state["appeal_id"])["status"] == "closed"
        assert http.api(args.base_url, "/admin/products/" + state["product_id"] + "/security-policy", token=root)["enabled"]
        (args.artifacts / "restart-report.json").write_text(json.dumps({"result": "passed", "checks": "security policy, reviewed case and closed appeal persist after restart"}))
        print("Security policy, case review and appeal persisted after restart")
        return
    report = {"stage": 4, "result": "running", "checks": []}
    suffix = uuid.uuid4().hex[:6]
    processes = []
    browser = playwright = None
    with tempfile.TemporaryDirectory(prefix="stage4-") as temporary, (args.artifacts / "server.log").open("w") as log:
        temp = pathlib.Path(temporary)
        try:
            base = args.base_url
            if not base:
                port = http.free_port()
                base = f"http://127.0.0.1:{port}"
                env = os.environ.copy()
                env.update(YN_ADDR=f"127.0.0.1:{port}", YN_DB_DRIVER="sqlite", YN_DB=f"file:{temporary}/db.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", YN_DB_CONFIG_FILE=temporary + "/config.json", YN_RELEASE_DIR=temporary + "/packages", YN_ADMIN_USERNAME="admin", YN_ADMIN_PASSWORD=password, YN_CARD_PEPPER=secrets.token_hex(32), YN_DATA_KEY=secrets.token_hex(32))
                p = subprocess.Popen([str(args.server.resolve())], cwd=REPO / "backend", env=env, stdout=log, stderr=log)
                processes.append(p)
                http.wait_ready(base, "/healthz", p)
            if not args.base_url:
                http.initialize(base,"admin",password,env["YN_DB"])
            root = http.api(base, "/admin/login", {"username": "admin", "password": password})["access_token"]
            http.api(base, "/admin/users", {"username": "developer-" + suffix, "password": password, "role": "developer"}, root)
            developer = http.api(base, "/developer/login", {"username": "developer-" + suffix, "password": password})["access_token"]
            product = http.api(base, "/developer/products", {"name": "Ebox 安全处置演示", "code": "S" + suffix.upper(), "bind_mode": "domain", "license_mode": "perpetual"}, developer)
            http.api(base, "/developer/products/" + product["id"] + "/updates", {"enabled": True}, developer)
            user = http.api(base, "/admin/customers", {"username": "customer-" + suffix, "password": password, "display_name": "Ebox 正版用户"}, root)
            token = http.api(base, "/user/login", {"username": user["username"], "password": password})["access_token"]
            card = http.api(base, "/admin/card-batches", {"product_id": product["id"], "quantity": 1, "duration_days": 30}, root)["codes"][0]
            license = http.api(base, "/user/activate", {"application_code": product["code"], "card_code": card, "bind_value": "ebox.example.com"}, token)
            binding = {"license_no": license["license_no"], "bind_mode": "domain", "bind_value": "ebox.example.com"}
            credentials = {**binding, "app_key": product["app_key"]}
            release = http.publish(base, product, developer, "2.0.0", http.archive("2.0.0", 2))
            policy = {"enabled": True, "detect_mismatch": True, "accept_reports": True, "action": "block_page", "message": "站点未授权，联系 QQ 531023977\n请支持正版软件，完成授权核实后恢复访问。", "support_url": base + "/user-console/"}
            errors = []
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                playwright = sync_playwright().start()
                browser = playwright.chromium.launch(executable_path="/usr/bin/chromium", headless=True, args=["--no-sandbox"])
                page = browser.new_page(viewport={"width": 1500, "height": 1100}, locale="zh-CN", timezone_id="Asia/Shanghai")
                page.on("pageerror", lambda e: errors.append(str(e)))
                page.goto(base + "/admin#security")
                page.get_by_label("账号", exact=True).fill("admin")
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="安全登录", exact=True).click()
                page.get_by_label("安全策略应用").select_option(product["id"])
                page.get_by_label("启用检测与已确认处置", exact=True).check()
                page.get_by_label("记录服务端绑定不匹配", exact=True).check()
                page.get_by_label("接收客户端升级包完整性报告", exact=True).check()
                page.get_by_label("确认后的建议动作").select_option("block_page")
                page.get_by_label("打击盗版公告", exact=True).fill(policy["message"])
                page.get_by_label("服务联系链接", exact=True).fill(policy["support_url"])
                page.get_by_role("button", name="保存安全策略", exact=True).click()
                page.get_by_text("应用安全策略已保存。", exact=True).wait_for()
                page.get_by_role("button", name="预览拦截页", exact=True).click()
                frame = page.frame_locator('iframe[title="拦截页公告预览"]')
                assert frame.locator(".announcement-text").inner_text() == policy["message"]
                page.locator(".announcement-preview").screenshot(path=str(args.artifacts / "admin-announcement-preview.png"))
                page.get_by_role("button", name="收起拦截页预览", exact=True).click()
            else:
                http.api(base, "/admin/products/" + product["id"] + "/security-policy", policy, root)
            saved = http.api(base, "/admin/products/" + product["id"] + "/security-policy", token=root)
            assert saved["message"] == policy["message"]
            report["checks"].append("Administrator's multiline anti-piracy notice and QQ contact persist and share the real block-page preview")
            config = temp / "private.json"
            config.write_text(json.dumps({"baseUrl": base, "appKey": product["app_key"], "binding": binding, "release": release, "productId": product["id"], "publicKeyPem": product["public_key_pem"]}))
            config.chmod(0o600)
            harness = temp / "tamper.mjs"
            harness.write_text('''import {readFile} from 'node:fs/promises';import assert from 'node:assert/strict';
import {LicenseClient} from ''' + json.dumps((REPO / "sdk/node/src/index.js").as_uri()) + ''';
const c=JSON.parse(await readFile(process.argv[2],'utf8'));const client=new LicenseClient({...c,reportIntegrityFailures:true,fetch:async(url,options)=>{const response=await fetch(url,options);if(url.endsWith('/download')&&response.ok){const bytes=new Uint8Array(await response.arrayBuffer());bytes[0]^=1;return new Response(bytes)}return response}});
await assert.rejects(client.downloadUpdate(c.release,c.binding,{publicKeyPem:c.publicKeyPem,productId:c.productId}),/checksum/);
''')
            subprocess.run([shutil.which("node"), str(harness), str(config)], check=True, capture_output=True)
            cases = http.api(base, "/admin/security/cases", token=root)
            case = next(c for c in cases if c["product_id"] == product["id"] and c["signal"] == "integrity_mismatch")
            assert case["status"] == "open" and case["action"] == "observe" and "未经独立证实" in case["evidence"]
            assert http.api(base, "/v1/licenses/verify", credentials)["can_use"]
            http.http(base, "/v1/licenses/verify", {**credentials, "bind_value": "unlicensed.example.com"}, expected=403)
            report["checks"].append("Real SDK refuses a modified signed package and reports the observed checksum; observations require manual review")
            site_port = http.free_port()
            site = f"http://127.0.0.1:{site_port}"
            env = os.environ.copy()
            env.update(YN_SECURITY_DEMO_CONFIG=str(config), YN_SECURITY_DEMO_PORT=str(site_port))
            demo = subprocess.Popen([shutil.which("node"), str(REPO / "examples/security-demo/server.mjs")], env=env, stdout=log, stderr=log)
            processes.append(demo)
            http.wait_ready(site, "/healthz", demo)
            review_path = "/admin/security/cases/" + case["id"] + "/review"
            http.http(base, review_path, {"status": "confirmed", "action": "block_page", "reason": "user override"}, token, expected=401)
            http.api(base, review_path, {"status": "confirmed", "action": "deny_updates", "reason": "核实后仅限制更新，保留运行"}, root)
            assert "Ebox 网站正常运行" in http.http(site, "/").decode()
            assert http.http(site, "/api/update", {}, expected=403)["code"] == "SECURITY_UPDATES_DENIED"
            assert http.http(base, "/v1/updates/" + release["id"] + "/download", credentials, expected=403)["error"]["code"] == "SECURITY_UPDATES_DENIED"
            if args.screenshots:
                app = browser.new_page(viewport={"width": 1500, "height": 1100}, locale="zh-CN")
                app.on("pageerror", lambda e: errors.append(str(e)))
                app.goto(site)
                app.get_by_role("button", name="检查在线更新", exact=True).click()
                app.get_by_text("仅更新服务被限制，当前网站仍可使用。", exact=True).wait_for()
                app.screenshot(path=str(args.artifacts / "updates-denied-website-usable.png"), full_page=True)
                page.get_by_role("button", name="刷新安全记录", exact=True).click()
                row = page.get_by_role("row").filter(has_text="客户端完整性报告").first
                row.get_by_role("button", name="查看与核实", exact=True).click()
                page.get_by_label("本次处置动作").select_option("block_page")
                page.get_by_label("核实与处置原因", exact=True).fill("已核实异常部署，先拦截访问并等待客户申诉。")
                page.get_by_role("button", name="提交核实结论", exact=True).click()
                page.get_by_text("安全处置已更新，操作已审计。", exact=True).wait_for()
                page.screenshot(path=str(args.artifacts / "admin-confirmed-security-case.png"), full_page=True)
            else:
                http.api(base, review_path, {"status": "confirmed", "action": "block_page", "reason": "核实异常部署，等待申诉"}, root)
            assert "授权安全拦截" in http.http(site, "/", expected=403).decode()
            for endpoint in ["/v1/licenses/verify", "/v1/licenses/heartbeat", "/v1/updates/" + release["id"] + "/download"]:
                assert http.http(base, endpoint, credentials, expected=403)["error"]["code"] == "SECURITY_USE_DENIED"
            if args.screenshots:
                response = app.goto(site)
                assert response.status == 403
                app.get_by_role("heading", name="授权安全拦截", exact=True).wait_for()
                assert app.locator(".announcement-text").inner_text() == policy["message"]
                app.screenshot(path=str(args.artifacts / "real-application-block-page.png"), full_page=True)
                app.set_viewport_size({"width": 390, "height": 844})
                assert app.evaluate("document.documentElement.scrollWidth <= innerWidth")
                app.screenshot(path=str(args.artifacts / "real-application-block-page-mobile.png"), full_page=True)
                app.set_viewport_size({"width": 1500, "height": 1100})
            revised_notice = "站点未授权，联系 QQ 531023977\n公告已更新，请联系处理授权事宜。"
            if args.screenshots:
                page.get_by_label("打击盗版公告", exact=True).fill(revised_notice)
                page.get_by_role("button", name="保存安全策略", exact=True).click()
                page.get_by_text("应用安全策略已保存。", exact=True).wait_for()
            else:
                http.api(base, "/admin/products/" + product["id"] + "/security-policy", {**policy, "message": revised_notice}, root)
            updated_html = http.http(site, "/", expected=403).decode()
            assert revised_notice in updated_html and policy["message"] not in updated_html
            http.api(base, "/admin/products/" + product["id"] + "/security-policy", policy, root)
            report["checks"].append("Editing the announcement updates the blocked application immediately without restarting it")
            report["checks"].append("Configured update-only restriction preserves the website; confirmed blocking affects real app access, heartbeat and package download")
            if args.screenshots:
                customer = browser.new_page(viewport={"width": 1500, "height": 1100}, locale="zh-CN")
                customer.on("pageerror", lambda e: errors.append(str(e)))
                customer.goto(base + "/user-console/")
                customer.get_by_label("账号", exact=True).fill(user["username"])
                customer.get_by_label("密码", exact=True).fill(password)
                customer.get_by_role("button", name="登录用户中心", exact=True).click()
                customer.locator("article").filter(has_text=case["id"]).get_by_role("button", name="提交申诉", exact=True).click()
                customer.get_by_label("申诉说明", exact=True).fill("本人持有正版购买凭证，请核实部署迁移记录并解除误报。")
                customer.get_by_role("button", name="发送申诉给管理员", exact=True).click()
                customer.get_by_text("申诉已提交，管理员将核实处理。", exact=True).wait_for()
                customer.screenshot(path=str(args.artifacts / "owner-security-appeal.png"), full_page=True)
            else:
                http.api(base, "/user/security/cases/" + case["id"] + "/appeal", {"reason": "正版购买，请核实部署迁移"}, token)
            appeals = http.api(base, "/admin/security/appeals", token=root)
            appeal = next(a for a in appeals if a["case_id"] == case["id"])
            http.api(base, review_path, {"status": "resolved", "reason": "核实购买与迁移记录，排除误报并解除"}, root)
            assert "Ebox 网站正常运行" in http.http(site, "/").decode()
            assert http.api(base, "/v1/licenses/verify", credentials)["can_update"]
            http.http(base, "/v1/updates/" + release["id"] + "/download", credentials)
            assert next(a for a in http.api(base, "/user/security/appeals", token=token) if a["id"] == appeal["id"])["status"] == "closed"
            if args.screenshots:
                app.goto(site)
                app.get_by_role("heading", name="Ebox 网站正常运行", exact=True).wait_for()
                app.screenshot(path=str(args.artifacts / "real-application-restored.png"), full_page=True)
                assert not errors, errors
            report["checks"].append("Owner appeals remain available while app is blocked; administrator resolution restores access and downloads and closes the appeal")
            report["persistence"] = {"case_id": case["id"], "appeal_id": appeal["id"], "product_id": product["id"]}
            report["result"] = "passed"
            print(json.dumps({"stage": 4, "result": "passed", "checks": len(report["checks"])}))
        finally:
            if report["result"] != "passed":
                report["result"] = "failed"
            (args.artifacts / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
            if browser:
                browser.close()
            if playwright:
                playwright.stop()
            for p in reversed(processes):
                p.terminate()
                p.wait(timeout=10)


if __name__ == "__main__":
    main()
