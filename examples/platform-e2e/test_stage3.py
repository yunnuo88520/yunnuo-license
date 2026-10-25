#!/usr/bin/env python3
"""Public agent verification and administrator-only ownership transfer evidence."""
import argparse
import importlib.util
import json
import os
import pathlib
import secrets
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
    parser.add_argument("--chromium", default="/usr/bin/chromium")
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    report = {"stage": 3, "result": "running", "checks": []}
    password = os.environ["YN_TEST_ADMIN_PASSWORD"] if args.base_url else secrets.token_hex(24)
    suffix = uuid.uuid4().hex[:6]
    process = browser = playwright = None
    with tempfile.TemporaryDirectory(prefix="stage3-") as temporary, (args.artifacts / "server.log").open("w") as log:
        try:
            base = args.base_url
            if not base:
                port = http.free_port()
                base = f"http://127.0.0.1:{port}"
                env = os.environ.copy()
                env.update(YN_ADDR=f"127.0.0.1:{port}", YN_DB_DRIVER="sqlite", YN_DB=f"file:{temporary}/db.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", YN_DB_CONFIG_FILE=temporary + "/config.json", YN_ADMIN_USERNAME="admin", YN_ADMIN_PASSWORD=password, YN_CARD_PEPPER=secrets.token_hex(32), YN_DATA_KEY=secrets.token_hex(32))
                process = subprocess.Popen([str(args.server.resolve())], cwd=REPO / "backend", env=env, stdout=log, stderr=log)
                http.wait_ready(base, "/healthz", process)
            if process:
                http.initialize(base,"admin",password,env["YN_DB"])
            root = http.api(base, "/admin/login", {"username": "admin", "password": password})["access_token"]
            product = http.api(base, "/admin/products", {"name": "Ebox 企业网站", "code": "V" + suffix.upper(), "bind_mode": "domain", "license_mode": "perpetual"}, root)
            pkg = http.api(base, "/admin/packages", {"product_id": product["id"], "name": "限期更新套餐", "duration_days": 30, "base_cost": 1000}, root)
            agent = http.api(base, "/admin/agents", {"name": "华东认证代理", "phone": "13812345678", "remark": "private-agent-remark"}, root)
            http.api(base, "/admin/agents/" + agent["id"] + "/packages", {"package_id": pkg["id"], "enabled": True, "max_batch_quantity": 10}, root)
            query = {"agent_no": agent["agent_no"], "application_id": product["id"]}
            verified = http.api(base, "/v1/agents/query", query)
            assert verified["is_authorized"] and verified["applications"] == [{"id": product["id"], "name": product["name"], "code": product["code"]}]
            assert not http.api(base, "/v1/agents/query", {**query, "application_id": "prod_ungranted"})["is_authorized"]
            serialized = json.dumps(verified)
            assert all(secret not in serialized for secret in [agent["id"], agent["login_code"], "13812345678", "private-agent-remark", "base_cost", "balance"])
            errors = []
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                playwright = sync_playwright().start()
                browser = playwright.chromium.launch(executable_path=None if args.chromium == "bundled" else args.chromium, headless=True, args=["--no-sandbox"])
                page = browser.new_page(viewport={"width": 1500, "height": 1100}, locale="zh-CN", timezone_id="Asia/Shanghai")
                page.on("pageerror", lambda e: errors.append(str(e)))
                page.goto(base)
                page.get_by_role("button", name="代理验真", exact=True).click()
                page.get_by_label("公开代理编号", exact=True).fill(agent["agent_no"])
                assert page.get_by_label("应用代码（可选）", exact=True).count() == 0
                page.get_by_role("button", name="核验代理身份", exact=True).click()
                page.get_by_text("当前代理权限有效", exact=True).wait_for()
                page.get_by_label("核验应用", exact=True).select_option(product["id"])
                page.screenshot(path=str(args.artifacts / "public-agent-valid.png"), full_page=True)
                for width in [390, 320]:
                    page.set_viewport_size({"width": width, "height": 844})
                    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth + 1"), "Agent application selector overflows on mobile"
                    if width == 390:
                        page.screenshot(path=str(args.artifacts / "public-agent-mobile-selector.png"), full_page=True)
                page.set_viewport_size({"width": 1500, "height": 1100})
            http.api(base, "/admin/agents/" + agent["id"] + "/suspend", {}, root)
            suspended = http.api(base, "/v1/agents/query", query)
            assert suspended["status"] == "suspended" and not suspended["is_authorized"] and suspended["applications"] == []
            if args.screenshots:
                page.get_by_role("button", name="核验代理身份", exact=True).click()
                page.get_by_text("代理权限已暂停或停用", exact=True).wait_for()
                page.screenshot(path=str(args.artifacts / "public-agent-suspended.png"), full_page=True)
            report["checks"].append("Anonymous verification follows current application grants and suspension without exposing login codes, contact details or costs")
            users, tokens = [], []
            for index in range(2):
                username = "owner-" + str(index) + "-" + suffix
                users.append(http.api(base, "/admin/customers", {"username": username, "password": password, "display_name": "原客户" if index == 0 else "受让客户"}, root))
                tokens.append(http.api(base, "/user/login", {"username": username, "password": password})["access_token"])
            card = http.api(base, "/admin/card-batches", {"product_id": product["id"], "name": "转移验收", "quantity": 1, "duration_days": 30}, root)["codes"][0]
            lic = http.api(base, "/user/activate", {"application_code": product["code"], "card_code": card, "bind_value": "customer.example.com"}, tokens[0])
            path = "/admin/licenses/" + lic["license_no"] + "/owner"
            transfer = {"user_id": users[1]["id"], "expected_owner": users[0]["id"], "reason": "企业授权受让已由管理员审核"}
            assert http.http(base, path, transfer, root, expected=403)["error"]["code"] == "OWNER_TRANSFER_DISABLED"
            http.http(base, path, transfer, tokens[0], expected=401)
            http.api(base, "/admin/products/" + product["id"] + "/binding-policy", {"allowed": False, "limit": 0, "allow_owner_transfer": True}, root)
            if args.screenshots:
                page.goto(base + "/admin#customers")
                page.get_by_label("账号", exact=True).fill("admin")
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="安全登录", exact=True).click()
                page.get_by_label("待分配授权编号", exact=True).fill(lic["license_no"])
                page.get_by_role("button", name="查找授权", exact=True).click()
                page.get_by_label("目标用户").select_option(users[1]["id"])
                page.get_by_label("操作原因", exact=True).fill(transfer["reason"])
                with page.expect_response(lambda r: r.url.endswith(path) and r.request.method == "POST") as response:
                    page.get_by_role("button", name="确认分配归属", exact=True).click()
                assert response.value.status == 200
                page.get_by_text("授权归属已更新，操作已审计。", exact=True).wait_for()
                page.get_by_role("button", name="查找授权", exact=True).click()
                page.get_by_text("当前归属：", exact=False).filter(has_text=users[1]["username"]).wait_for()
                page.screenshot(path=str(args.artifacts / "admin-owner-transfer.png"), full_page=True)
            else:
                http.api(base, path, transfer, root)
            http.http(base, "/user/licenses/" + lic["license_no"], token=tokens[0], expected=404)
            owned = http.api(base, "/user/licenses/" + lic["license_no"], token=tokens[1])
            assert owned["can_use"] and owned["can_update"] and owned["license"]["usage_expired_at"] is None
            assert owned["bindings"][0]["value"] == "customer.example.com"
            assert http.http(base, path, transfer, root, expected=409)["error"]["code"] == "OWNER_CHANGED"
            report["checks"].append("Admin-only transfer requires explicit opt-in and reason, removes former-owner access, preserves bindings and rejects stale-owner retries")
            if args.screenshots:
                page.goto(base + "/user-console/")
                page.get_by_label("账号", exact=True).fill(users[1]["username"])
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="登录用户中心", exact=True).click()
                page.get_by_text("应用可用", exact=True).wait_for()
                page.get_by_text("更新服务有效", exact=True).wait_for()
                page.screenshot(path=str(args.artifacts / "new-owner-user-center.png"), full_page=True)
                assert not errors, errors
            report["result"] = "passed"
            print(json.dumps({"stage": 3, "result": "passed", "checks": len(report["checks"])}))
        finally:
            if report["result"] != "passed":
                report["result"] = "failed"
            (args.artifacts / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
            if browser:
                browser.close()
            if playwright:
                playwright.stop()
            if process:
                process.terminate()
                process.wait(timeout=10)


if __name__ == "__main__":
    main()
