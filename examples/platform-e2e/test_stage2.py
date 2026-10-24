#!/usr/bin/env python3
"""Real-service package pricing, atomic issuance and browser evidence."""
import argparse
import concurrent.futures
import importlib.util
import json
import os
import pathlib
import secrets
import subprocess
import tempfile
import urllib.error
import urllib.request
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
    if args.verify_state:
        saved = json.loads(args.verify_state.read_text())["persistence"]
        root = http.api(args.base_url, "/admin/login", {"username": "admin", "password": os.environ["YN_TEST_ADMIN_PASSWORD"]})["access_token"]
        result = http.api(args.base_url, "/admin/agents/" + saved["agent_id"] + "/account", token=root)
        assert result["account"]["balance"] == saved["balance"]
        assert len(result["ledgers"]) == saved["ledger_count"]
        assert result["packages"] == []
        batches = http.api(args.base_url, "/admin/card-batches", token=root)
        assert len([b for b in batches if b.get("agent_id") == saved["agent_id"]]) == saved["batch_count"]
        (args.artifacts / "restart-report.json").write_text(json.dumps({"result": "passed", "checks": "balance, ledger snapshots, issued batches and revoked grants survive restart"}))
        print("Package balances, ledgers, batches and revoked grants persisted after restart")
        return
    report = {"stage": 2, "result": "running", "checks": []}
    report_file = args.artifacts / "report.json"
    report_file.write_text(json.dumps(report))
    password = os.environ["YN_TEST_ADMIN_PASSWORD"] if args.base_url else secrets.token_hex(24)
    suffix = uuid.uuid4().hex[:7]
    process = browser = playwright = None
    with tempfile.TemporaryDirectory(prefix="stage2-") as temporary, (args.artifacts / "server.log").open("w") as log:
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
            if process:
                http.initialize(base,"admin",password,env["YN_DB"])
            root = http.api(base, "/admin/login", {"username": "admin", "password": password})["access_token"]
            product = http.api(base, "/admin/products", {"name": "Ebox 企业网站", "code": "P" + suffix.upper(), "bind_mode": "domain"}, root)
            pkg = http.api(base, "/admin/packages", {"product_id": product["id"], "name": "Ebox 月度授权", "duration_days": 30, "base_cost": 1000}, root)
            group = http.api(base, "/admin/product-groups", {"name": "企业网站组", "product_ids": [product["id"]]}, root)
            tier = http.api(base, "/admin/agent-tiers", {"name": "金牌代理"}, root)
            http.api(base, "/admin/agent-tiers/" + tier["id"] + "/prices", {"package_id": pkg["id"], "unit_cost": 700}, root)
            agent = http.api(base, "/admin/agents", {"name": "华东正版代理"}, root)
            prefix = "/admin/agents/" + agent["id"]
            http.api(base, prefix + "/users", {"username": "owner-" + suffix, "password": password}, root)
            http.api(base, prefix + "/tier", {"tier_id": tier["id"]}, root)
            grant = {"group_id": group["id"], "enabled": True, "max_batch_quantity": 10, "can_export": True}
            credit = {"amount": 1400, "reason": "验证预付余额：两张金牌月卡", "request_id": uuid.uuid4().hex}
            errors = []
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                playwright = sync_playwright().start()
                browser = playwright.chromium.launch(executable_path="/usr/bin/chromium", headless=True, args=["--no-sandbox"])
                page = browser.new_page(viewport={"width": 1540, "height": 1100}, locale="zh-CN", timezone_id="Asia/Shanghai")
                page.on("pageerror", lambda e: errors.append(str(e)))
                page.goto(base + "/admin#commerce")
                page.get_by_label("账号", exact=True).fill("admin")
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="安全登录", exact=True).click()
                page.get_by_role("heading", name="代理授权与资金", exact=True).wait_for()
                page.get_by_label("当前代理").select_option(agent["id"])
                page.get_by_label("或按分组批量授权").select_option(group["id"])
                page.get_by_label("单批上限", exact=True).fill("10")
                page.get_by_label("允许明文导出", exact=True).check()
                with page.expect_response(lambda r: r.url.endswith(prefix + "/packages") and r.request.method == "POST") as response:
                    page.get_by_role("button", name="保存套餐授权", exact=True).click()
                assert response.value.status == 200
                page.get_by_role("cell", name="等级价", exact=True).wait_for()
                captured = []
                page.on("request", lambda r: captured.append(r.post_data_json) if r.url.endswith(prefix + "/balance") else None)
                page.get_by_label("余额调整（分，可为负数）", exact=True).fill("1400")
                page.get_by_label("调整原因", exact=True).fill(credit["reason"])
                with page.expect_response(lambda r: r.url.endswith(prefix + "/balance") and r.request.method == "POST") as response:
                    page.get_by_role("button", name="登记余额调整", exact=True).click()
                assert response.value.status == 200
                page.get_by_text("¥14.00", exact=True).first.wait_for()
                credit = captured[-1]
                page.get_by_role("heading", name="代理授权与资金", exact=True).locator("..").screenshot(path=str(args.artifacts / "admin-package-grants.png"))
            else:
                http.api(base, prefix + "/packages", grant, root)
                http.api(base, prefix + "/balance", credit, root)
            assert http.api(base, prefix + "/balance", credit, root)["balance_after"] == 1400
            token = http.api(base, "/agent/login", {"login_code": agent["login_code"], "username": "owner-" + suffix, "password": password})["access_token"]
            quotes = http.api(base, "/agent/packages", token=token)
            assert len(quotes) == 1 and quotes[0]["unit_cost"] == 700 and quotes[0]["price_source"] == "tier"
            other = http.api(base, "/admin/agents", {"name": "未授权独立代理"}, root)
            http.api(base, "/admin/agents/" + other["id"] + "/users", {"username": "other-" + suffix, "password": password}, root)
            other_token = http.api(base, "/agent/login", {"login_code": other["login_code"], "username": "other-" + suffix, "password": password})["access_token"]
            other_account = http.api(base, "/agent/account?agent_id=" + agent["id"], token=other_token)
            assert other_account["agent_id"] == other["id"] and other_account["balance"] == 0
            assert http.api(base, "/agent/packages?agent_id=" + agent["id"], token=other_token) == []
            assert http.api(base, "/agent/money-ledgers?agent_id=" + agent["id"], token=other_token) == []
            http.http(base, prefix + "/account", token=other_token, expected=401)
            report["checks"].append("Independent agent accounts cannot read another agent's balance, packages or ledgers")
            http.http(base, "/admin/packages", {"product_id": product["id"], "name": "代理伪造套餐", "duration_days": 999}, token, expected=401)
            denied = {"product_id": product["id"], "quantity": 1, "duration_days": 999}
            assert http.http(base, "/agent/card-batches", denied, token, expected=403)["error"]["code"] == "AGENT_PACKAGE_NOT_ALLOWED"
            http.api(base, "/admin/packages", {"product_id": product["id"], "name": "新增年卡", "duration_days": 365, "base_cost": 9999}, root)
            assert len(http.api(base, "/agent/packages", token=token)) == 1
            report["checks"].append("Administrator-only packages, group-grant snapshot and authoritative tier pricing")
            issue = {"package_id": pkg["id"], "quantity": 2, "name": "真实月卡发放", "request_id": uuid.uuid4().hex, "expected_unit_cost": 700}
            assert http.http(base, "/agent/card-batches", {**issue, "expected_unit_cost": 1}, token, expected=400)["error"]["code"] == "PACKAGE_PRICE_CHANGED"
            assert http.http(base, "/agent/card-batches", {**issue, "duration_days": 999}, token, expected=400)["error"]["code"] == "PACKAGE_TERMS_MISMATCH"
            if args.screenshots:
                user = browser.new_page(viewport={"width": 1500, "height": 1100}, locale="zh-CN", timezone_id="Asia/Shanghai")
                user.on("pageerror", lambda e: errors.append(str(e)))
                user.goto(base + "/agent-console/#generate")
                user.get_by_label("代理代码", exact=True).fill(agent["login_code"])
                user.get_by_label("登录名", exact=True).fill("owner-" + suffix)
                user.get_by_label("密码", exact=True).fill(password)
                user.get_by_role("button", name="进入工作台", exact=True).click()
                user.get_by_label("授权套餐").select_option(pkg["id"])
                user.get_by_label("批次名称", exact=True).fill(issue["name"])
                user.get_by_label("数量", exact=True).fill("2")
                user.screenshot(path=str(args.artifacts / "agent-tier-price.png"), full_page=True)
                captured = []
                user.on("request", lambda r: captured.append(r.post_data_json) if r.url.endswith("/agent/card-batches") and r.method == "POST" else None)
                with user.expect_response(lambda r: r.url.endswith("/agent/card-batches") and r.request.method == "POST") as response:
                    user.get_by_role("button", name="生成卡密", exact=True).click()
                assert response.value.status == 200
                generated = response.value.json()["data"]
                issue = captured[-1]
                user.get_by_role("link", name="额度流水", exact=True).click()
                user.get_by_role("heading", name="资金流水", exact=True).wait_for()
                user.get_by_role("cell", name="套餐发卡", exact=True).wait_for()
                user.screenshot(path=str(args.artifacts / "agent-atomic-ledger.png"), full_page=True)
            else:
                generated = http.api(base, "/agent/card-batches", issue, token)
            retried = http.api(base, "/agent/card-batches", issue, token)
            assert retried["batch"]["id"] == generated["batch"]["id"]
            assert sorted(retried["codes"]) == sorted(generated["codes"])
            assert http.api(base, "/agent/account", token=token)["balance"] == 0
            ledgers = http.api(base, "/agent/money-ledgers", token=token)
            assert len(ledgers) == 2 and sum(l["amount"] for l in ledgers) == 0
            assert http.http(base, "/agent/card-batches", {**issue, "quantity": 1}, token, expected=400)["error"]["code"] == "IDEMPOTENCY_CONFLICT"
            assert http.http(base, "/agent/card-batches", {**issue, "quantity": 1, "request_id": uuid.uuid4().hex}, token, expected=403)["error"]["code"] == "AGENT_BALANCE_NOT_ENOUGH"
            report["checks"].append("Actual browser issuance, immutable terms, price checks, one debit and replay of the same batch")
            license = http.api(base, "/v1/licenses/activate", {"app_key": product["app_key"], "card_code": generated["codes"][0], "bind_mode": "domain", "bind_value": "ebox.example.com"})
            assert license["can_use"]
            http.api(base, prefix + "/balance", {"amount": 700, "reason": "并发验证：仅够一张卡", "request_id": uuid.uuid4().hex}, root)

            def concurrent_issue(_):
                payload = json.dumps({**issue, "quantity": 1, "request_id": uuid.uuid4().hex}).encode()
                request = urllib.request.Request(base + "/agent/card-batches", data=payload, headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
                try:
                    response = urllib.request.urlopen(request, timeout=40)
                except urllib.error.HTTPError as e:
                    response = e
                with response:
                    return response.status, json.loads(response.read())

            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                results = list(pool.map(concurrent_issue, range(2)))
            assert sorted(r[0] for r in results) == [200, 403], results
            assert next(r[1]["error"]["code"] for r in results if r[0] == 403) == "AGENT_BALANCE_NOT_ENOUGH"
            assert http.api(base, "/agent/account", token=token)["balance"] == 0
            assert len(http.api(base, "/agent/card-batches", token=token)) == 2
            report["checks"].append("Concurrent HTTP requests cannot overspend; issued cards activate successfully")
            http.api(base, prefix + "/packages", {"package_id": pkg["id"], "enabled": False, "max_batch_quantity": 10}, root)
            assert http.api(base, "/agent/packages", token=token) == []
            assert http.http(base, "/agent/card-batches", {**issue, "quantity": 1, "request_id": uuid.uuid4().hex}, token, expected=403)["error"]["code"] == "AGENT_PACKAGE_NOT_ALLOWED"
            logs = http.api(base, "/admin/operation-logs?page_size=5000", token=root)
            assert any(l["error_code"] == "AGENT_BALANCE_NOT_ENOUGH" and l["actor_type"] == "agent" for l in logs["items"])
            assert all(secret not in json.dumps(logs) for secret in [root, token, password, *generated["codes"]])
            report["checks"].append("Revoked package grants and complete, attributable refusal logs without secrets")
            report["persistence"] = {"agent_id": agent["id"], "balance": 0, "ledger_count": 4, "batch_count": 2}
            if args.screenshots:
                assert not errors, errors
            report["result"] = "passed"
            report_file.write_text(json.dumps(report, ensure_ascii=False, indent=2))
            print(json.dumps({"stage": 2, "result": "passed", "checks": len(report["checks"]), "artifacts": str(args.artifacts)}, ensure_ascii=False))
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
