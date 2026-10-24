#!/usr/bin/env python3
"""Real-service ownership, renewal, rebind and operation-audit evidence."""
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
    p = argparse.ArgumentParser()
    p.add_argument("--server", type=pathlib.Path)
    p.add_argument("--base-url")
    p.add_argument("--artifacts", type=pathlib.Path, required=True)
    p.add_argument("--screenshots", action="store_true")
    args = p.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    report = {"stage": 1, "result": "running", "checks": []}
    report_file = args.artifacts / "report.json"
    report_file.write_text(json.dumps(report))
    password = os.environ["YN_TEST_ADMIN_PASSWORD"] if args.base_url else secrets.token_hex(24)
    suffix = uuid.uuid4().hex[:6]
    alice_name, bob_name = "alice-" + suffix, "bob-" + suffix
    process = browser = playwright = None
    with tempfile.TemporaryDirectory(prefix="stage1-") as temporary, (args.artifacts / "server.log").open("w") as log:
        try:
            base = args.base_url
            if not base:
                if not args.server:
                    raise ValueError("--server or --base-url required")
                port = http.free_port()
                base = f"http://127.0.0.1:{port}"
                env = os.environ.copy()
                env.update(YN_ADDR=f"127.0.0.1:{port}", YN_DB_DRIVER="sqlite", YN_DB=f"file:{temporary}/db.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", YN_DB_CONFIG_FILE=temporary + "/config.json", YN_RELEASE_DIR=temporary + "/packages", YN_ADMIN_USERNAME="admin", YN_ADMIN_PASSWORD=password, YN_CARD_PEPPER=secrets.token_hex(32), YN_DATA_KEY=secrets.token_hex(32))
                process = subprocess.Popen([str(args.server.resolve())], cwd=REPO / "backend", env=env, stdout=log, stderr=log)
                http.wait_ready(base, "/healthz", process)
            root = http.api(base, "/admin/login", {"username": "admin", "password": password})["access_token"]
            alice = http.api(base, "/admin/customers", {"username": alice_name, "password": password, "display_name": "小王 · 正版客户"}, root)
            bob = http.api(base, "/admin/customers", {"username": bob_name, "password": password, "display_name": "小李 · 独立用户"}, root)
            alice_token = http.api(base, "/user/login", {"username": alice_name, "password": password})["access_token"]
            bob_token = http.api(base, "/user/login", {"username": bob_name, "password": password})["access_token"]
            product = http.api(base, "/admin/products", {"name": "Ebox 企业网站授权", "code": "U" + suffix.upper(), "bind_mode": "domain", "license_mode": "perpetual"}, root)
            http.api(base, "/admin/products/" + product["id"] + "/binding-policy", {"allowed": True, "limit": 2, "cooldown_hours": 24}, root)
            batch = http.api(base, "/admin/card-batches", {"product_id": product["id"], "name": "客户验证套餐", "quantity": 2, "duration_days": 30}, root)
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                playwright = sync_playwright().start()
                browser = playwright.chromium.launch(executable_path="/usr/bin/chromium", headless=True, args=["--no-sandbox"])
                page = browser.new_page(viewport={"width": 1500, "height": 1100}, locale="zh-CN", timezone_id="Asia/Shanghai")
                errors = []
                page.on("pageerror", lambda e: errors.append(str(e)))
                page.goto(base + "/user-console/")
                page.get_by_label("账号", exact=True).fill(alice_name)
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="登录用户中心", exact=True).click()
                page.get_by_label("应用代码", exact=True).fill(product["code"])
                page.get_by_label("未激活卡密", exact=True).fill(batch["codes"][0])
                page.get_by_label("绑定信息", exact=True).fill("old.example.com")
                page.get_by_role("button", name="激活并绑定到我的账号", exact=True).click()
                page.get_by_text("授权已激活并归属当前账号。", exact=True).wait_for()
            else:
                http.api(base, "/user/activate", {"application_code": product["code"], "card_code": batch["codes"][0], "bind_value": "old.example.com"}, alice_token)
            owned = http.api(base, "/user/licenses", token=alice_token)
            assert len(owned) == 1 and owned[0]["license"]["owner_user_id"] == alice["id"]
            number, binding = owned[0]["license"]["license_no"], owned[0]["bindings"][0]["id"]
            path = "/user/licenses/" + number
            assert http.api(base, "/user/licenses?user_id=" + alice["id"], token=bob_token) == []
            assert http.http(base, path, token=bob_token, expected=404)["error"]["code"] == "LICENSE_NOT_FOUND"
            http.http(base, path + "/renew", {"binding_id": binding, "card_code": batch["codes"][1]}, bob_token, expected=404)
            http.http(base, "/admin/customers", token=alice_token, expected=401)
            http.http(base, "/admin/licenses/" + number + "/owner", {"user_id": bob["id"], "reason": "self transfer"}, alice_token, expected=401)
            assert http.http(base, "/user/activate", {"application_code": product["code"], "card_code": batch["codes"][0], "bind_value": "other.example.com"}, bob_token, expected=403)["error"]["code"] == "OWNER_REQUIRED"
            report["checks"].append("Ownership isolation, user-session privilege boundary and activated-card claim protection")
            if args.screenshots:
                page.get_by_role("button", name="卡密续费", exact=True).click()
                page.get_by_label("续费卡密", exact=True).fill(batch["codes"][1])
                page.get_by_role("button", name="确认续费", exact=True).click()
                page.get_by_text("续费成功，期限已更新。", exact=True).wait_for()
            else:
                http.api(base, path + "/renew", {"binding_id": binding, "card_code": batch["codes"][1]}, alice_token)
            updated = http.api(base, path, token=alice_token)
            assert updated["license"]["usage_expired_at"] is None and updated["license"]["updates_expired_at"] > owned[0]["license"]["updates_expired_at"]
            report["checks"].append("User-owned card renewal extends only the intended rights")
            change = {"binding_id": binding, "new_value": "new.example.com", "reason": "网站迁移到新域名", "request_id": uuid.uuid4().hex}
            if args.screenshots:
                requests = []
                page.on("request", lambda r: requests.append(r.post_data_json) if r.url.endswith("/rebind") else None)
                page.get_by_role("button", name="更换绑定", exact=True).click()
                page.get_by_label("新的绑定信息", exact=True).fill(change["new_value"])
                page.get_by_label("更换原因", exact=True).fill(change["reason"])
                page.get_by_role("button", name="确认换绑", exact=True).click()
                page.get_by_text("换绑成功，旧绑定已失效。", exact=True).wait_for()
                change = requests[-1]
                page.screenshot(path=str(args.artifacts / "user-owned-licenses.png"), full_page=True)
            else:
                http.api(base, path + "/rebind", change, alice_token)
            assert http.api(base, path + "/rebind", change, alice_token)["changes_used"] == 1
            credentials = {"app_key": product["app_key"], "license_no": number, "bind_mode": "domain"}
            assert http.http(base, "/v1/licenses/verify", {**credentials, "bind_value": "old.example.com"}, expected=403)["error"]["code"] == "BINDING_MISMATCH"
            assert http.api(base, "/v1/licenses/verify", {**credentials, "bind_value": "new.example.com"})["can_use"]
            next_change = {**change, "request_id": uuid.uuid4().hex, "new_value": "next.example.com"}
            assert http.http(base, path + "/rebind", next_change, alice_token, expected=403)["error"]["code"] == "UNBIND_COOLDOWN"
            if args.screenshots:
                page.get_by_role("button", name="更换绑定", exact=True).click()
                page.get_by_label("新的绑定信息", exact=True).fill("next.example.com")
                page.get_by_label("更换原因", exact=True).fill("重复迁移尝试")
                page.get_by_role("button", name="确认换绑", exact=True).click()
                page.get_by_role("alert").filter(has_text="UNBIND_COOLDOWN").wait_for()
                page.screenshot(path=str(args.artifacts / "rebind-cooldown-denied.png"), full_page=True)
            report["checks"].append("Atomic rebind invalidates old binding, charges retries once and enforces cooldown")
            assert http.http(base, "/admin/licenses/" + number + "/owner", {"user_id": bob["id"], "expected_owner": alice["id"], "reason": "管理员转移验证"}, root, expected=403)["error"]["code"] == "OWNER_TRANSFER_DISABLED"
            report["checks"].append("Owner transfers disabled by default and unavailable to ordinary users")
            logs = http.api(base, "/admin/operation-logs?category=binding&result=failed", token=root)
            assert any(r["error_code"] == "UNBIND_COOLDOWN" and r["actor_id"] == alice["id"] for r in logs["items"])
            all_logs = http.api(base, "/admin/operation-logs?page_size=5000", token=root)
            assert any(r["actor_id"] == bob["id"] and r["http_status"] == 404 for r in all_logs["items"])
            serialized = json.dumps(all_logs)
            assert all(secret not in serialized for secret in [password, alice_token, bob_token, root, *batch["codes"]])
            report["checks"].append("Categorized, attributed operation logs exclude passwords, card codes and tokens")
            if args.screenshots:
                denied = browser.new_page()
                denied.set_extra_http_headers({"Authorization": "Bearer " + bob_token})
                denied.goto(base + path)
                denied.screenshot(path=str(args.artifacts / "cross-user-access-denied.png"), full_page=True)
                page.goto(base + "/admin-console/#customers")
                page.get_by_label("账号", exact=True).fill("admin")
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="安全登录", exact=True).click()
                page.get_by_role("heading", name="普通用户管理", exact=True).wait_for()
                page.get_by_text("小李 · 独立用户", exact=True).wait_for()
                page.screenshot(path=str(args.artifacts / "admin-customers.png"), full_page=True)
                page.get_by_role("link", name="审计", exact=True).click()
                page.get_by_role("heading", name="全操作记录", exact=True).wait_for()
                page.get_by_label("分类标签").select_option("binding")
                page.get_by_label("结果").first.select_option("failed")
                page.get_by_role("button", name="查询操作日志", exact=True).click()
                page.get_by_role("cell", name="UNBIND_COOLDOWN", exact=True).first.wait_for()
                page.screenshot(path=str(args.artifacts / "categorized-denial-audit.png"), full_page=True)
                assert not errors, errors
                report["checks"].append("Browser activation, renewal, rebind, refusal, account management and audit search")
            report["result"] = "passed"
            report_file.write_text(json.dumps(report, ensure_ascii=False, indent=2))
            print(json.dumps({"stage": 1, "result": "passed", "checks": len(report["checks"]), "artifacts": str(args.artifacts)}, ensure_ascii=False))
        finally:
            if report["result"] != "passed":
                report["result"] = "failed"
                report_file.write_text(json.dumps(report, ensure_ascii=False, indent=2))
            if browser:
                browser.close()
            if playwright:
                playwright.stop()
            if process:
                process.terminate()
                process.wait(timeout=10)


if __name__ == "__main__":
    main()
