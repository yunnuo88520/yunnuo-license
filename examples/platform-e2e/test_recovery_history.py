#!/usr/bin/env python3
"""Real HTTP account recovery, session revocation, isolated full history and browser evidence."""
import argparse
import csv
import importlib.util
import io
import json
import os
import pathlib
import secrets
import subprocess
import tempfile
import uuid

REPO = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("http_helpers", REPO / "examples/update-demo/test_e2e.py")
http = importlib.util.module_from_spec(spec)
spec.loader.exec_module(http)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server", type=pathlib.Path)
    parser.add_argument("--base-url")
    parser.add_argument("--artifacts", required=True, type=pathlib.Path)
    parser.add_argument("--screenshots", action="store_true")
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    report = {"result": "running", "checks": []}
    process = browser = playwright = None
    password = os.environ["YN_TEST_ADMIN_PASSWORD"] if args.base_url else secrets.token_hex(24)
    new_password = secrets.token_hex(24)
    suffix = uuid.uuid4().hex[:8]
    with tempfile.TemporaryDirectory(prefix="recovery-history-") as temporary, (args.artifacts / "server.log").open("w") as log:
        try:
            base = args.base_url
            if not base:
                if not args.server:
                    raise ValueError("--server or --base-url is required")
                port = http.free_port()
                base = f"http://127.0.0.1:{port}"
                env = os.environ.copy()
                env.update(YN_ADDR=f"127.0.0.1:{port}", YN_DB_DRIVER="sqlite", YN_DB=f"file:{temporary}/db.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", YN_DB_CONFIG_FILE=temporary + "/config.json", YN_RELEASE_DIR=temporary + "/packages")
                process = subprocess.Popen([str(args.server.resolve())], cwd=REPO / "backend", env=env, stdout=log, stderr=log)
                http.wait_ready(base, "/healthz", process)
                http.initialize(base, "admin", password, env["YN_DB"])
            root = http.api(base, "/admin/login", {"username": "admin", "password": password})["access_token"]
            customer = http.api(base, "/admin/customers", {"username": "customer-" + suffix, "password": password, "display_name": "小王 · 账号恢复验证"}, root)
            old_customer = http.api(base, "/user/login", {"username": customer["username"], "password": password})["access_token"]
            http.http(base, "/admin/recovery/accounts?kind=customer", token=old_customer, expected=401)
            reset = None
            errors = []
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                playwright = sync_playwright().start()
                browser = playwright.chromium.launch(executable_path="/usr/bin/chromium", headless=True, args=["--no-sandbox"])
                page = browser.new_page(viewport={"width": 1540, "height": 1100}, locale="zh-CN")
                page.on("pageerror", lambda e: errors.append(str(e)))
                page.goto(base + "/admin#recovery")
                page.get_by_label("账号", exact=True).fill("admin")
                page.get_by_label("密码", exact=True).fill(password)
                page.get_by_role("button", name="安全登录", exact=True).click()
                page.get_by_role("heading", name="账号密码恢复", exact=True).wait_for()
                page.get_by_label("待恢复账号", exact=True).select_option(customer["id"])
                page.get_by_label("身份核实与操作原因", exact=True).fill("已核实购买记录与授权绑定信息")
                with page.expect_response(lambda r: r.url.endswith("/admin/recovery/customer/" + customer["id"]) and r.request.method == "POST") as response:
                    page.get_by_role("button", name="创建一次性恢复码", exact=True).click()
                assert response.value.status == 200
                reset = response.value.json()["data"]
                page.get_by_text("一次性恢复码已创建", exact=False).wait_for()
                page.screenshot(path=str(args.artifacts / "admin-account-recovery.png"), full_page=True)
                recover = browser.new_page(viewport={"width": 1440, "height": 1000}, locale="zh-CN")
                recover.on("pageerror", lambda e: errors.append(str(e)))
                recover.goto(base + "/recover")
                recover.get_by_label("一次性恢复码", exact=True).fill(reset["code"])
                recover.get_by_label("新密码", exact=True).fill(new_password)
                recover.get_by_label("再次输入新密码", exact=True).fill(new_password)
                recover.get_by_role("button", name="确认重置密码", exact=True).click()
                recover.get_by_role("heading", name="密码已重置", exact=True).wait_for()
                recover.screenshot(path=str(args.artifacts / "customer-password-recovered.png"), full_page=True)
            else:
                reset = http.api(base, "/admin/recovery/customer/" + customer["id"], {"reason": "已核实购买记录与绑定信息", "minutes": 30}, root)
                http.api(base, "/v1/accounts/recovery", {"code": reset["code"], "password": new_password})
            http.http(base, "/user/licenses", token=old_customer, expected=401)
            http.api(base, "/user/login", {"username": customer["username"], "password": new_password})
            assert http.http(base, "/v1/accounts/recovery", {"code": reset["code"], "password": password}, expected=401)["error"]["code"] == "RECOVERY_INVALID"
            report["checks"].append("Administrator verifies identity and issues a one-time recovery code; user resets password, old session is rejected and code cannot be replayed")
            agent = http.api(base, "/admin/agents", {"name": "历史流水验证代理 " + suffix}, root)
            prefix = "/admin/agents/" + agent["id"]
            agent_user = http.api(base, prefix + "/users", {"username": "owner-" + suffix, "password": password}, root)
            credentials = {"login_code": agent["login_code"], "username": agent_user["username"], "password": password}
            old_agent = http.api(base, "/agent/login", credentials)["access_token"]
            http.api(base, "/admin/recovery/agent/" + agent_user["id"] + "/password", {"password": new_password, "reason": "核实代理身份后重置"}, root)
            http.http(base, "/agent/account", token=old_agent, expected=401)
            token = http.api(base, "/agent/login", {**credentials, "password": new_password})["access_token"]
            other = http.api(base, "/admin/agents", {"name": "隔离验证代理 " + suffix}, root)
            other_prefix = "/admin/agents/" + other["id"]
            http.api(base, other_prefix + "/users", {"username": "other-" + suffix, "password": password}, root)
            other_token = http.api(base, "/agent/login", {"login_code": other["login_code"], "username": "other-" + suffix, "password": password})["access_token"]
            for n in range(505):
                http.api(base, prefix + "/balance", {"amount": 1, "reason": "=历史导出公式验证" if n == 0 else f"历史流水 {n:04d}", "request_id": uuid.uuid4().hex}, root)
            last = http.api(base, "/agent/history/money?page=6&page_size=100&agent_id=" + other["id"], token=token)
            assert last["total"] == 505 and len(last["items"]) == 5 and all(r["agent_id"] == agent["id"] for r in last["items"])
            assert http.api(base, "/agent/history/money?agent_id=" + agent["id"], token=other_token)["total"] == 0
            found = http.api(base, "/agent/history/money?q=0001", token=token)
            assert found["total"] == 1 and found["items"][0]["remark"] == "历史流水 0001"
            exported = list(csv.reader(io.StringIO(http.http(base, "/agent/history/money/export?page=99&agent_id=" + other["id"], token=token).decode())))
            assert len(exported) == 506 and all(r[2] == agent["id"] for r in exported[1:]) and any(r[10] == "'=历史导出公式验证" for r in exported[1:])
            http.http(base, "/agent/history/cases", token=token, expected=403)
            report["checks"].append("505 real HTTP balance adjustments remain pageable and fully exportable; CSV formulas are escaped and forged identity filters cannot cross agent boundaries")
            if args.screenshots:
                agent_page = browser.new_page(viewport={"width": 1540, "height": 1100}, locale="zh-CN")
                agent_page.on("pageerror", lambda e: errors.append(str(e)))
                agent_page.goto(base + "/agent-console/#money")
                agent_page.get_by_label("代理代码", exact=True).fill(agent["login_code"])
                agent_page.get_by_label("登录名", exact=True).fill(credentials["username"])
                agent_page.get_by_label("密码", exact=True).fill(new_password)
                agent_page.get_by_role("button", name="进入工作台", exact=True).click()
                agent_page.get_by_role("link", name="额度流水", exact=True).click()
                agent_page.get_by_text("共 505 条", exact=False).wait_for()
                agent_page.get_by_role("button", name="下一页", exact=True).click()
                agent_page.get_by_text("第 2 / 26 页", exact=False).wait_for()
                agent_page.screenshot(path=str(args.artifacts / "agent-history-pagination.png"), full_page=True)
                agent_page.get_by_label("搜索历史记录", exact=True).fill("0001")
                agent_page.get_by_role("button", name="查询历史记录", exact=True).click()
                agent_page.get_by_role("cell", name="历史流水 0001", exact=True).wait_for()
                with agent_page.expect_download() as download:
                    agent_page.get_by_role("button", name="导出全部筛选结果", exact=True).click()
                assert len(list(csv.reader(io.StringIO(pathlib.Path(download.value.path()).read_text())))) == 2
                agent_page.screenshot(path=str(args.artifacts / "agent-history-filter-export.png"), full_page=True)
                assert not errors, errors
            logs = http.api(base, "/admin/operation-logs?category=account&page_size=5000", token=root)
            assert any("recovery" in r["route"] for r in logs["items"])
            assert all(secret not in json.dumps(logs) for secret in [password, new_password, reset["code"], root, token])
            report["checks"].append("Recovery requests are attributable in operation logs without passwords, reset codes or access tokens")
            if process:
                result = subprocess.run([str(args.server.resolve()), "--reset-admin", "admin", "--reason", "服务器持有人离线恢复验证"], input=new_password + "\n", text=True, cwd=REPO / "backend", env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
                assert result.returncode == 0 and new_password not in result.stdout + result.stderr
                http.http(base, "/admin/profile", token=root, expected=401)
                http.api(base, "/admin/login", {"username": "admin", "password": new_password})
                report["checks"].append("Offline administrator reset reads the password from stdin, records its reason and invalidates old sessions")
            report["result"] = "passed"
            print(json.dumps({"result": "passed", "checks": len(report["checks"]), "artifacts": str(args.artifacts)}))
        finally:
            if report["result"] != "passed":
                report["result"] = "failed"
            (args.artifacts / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
            if browser:
                browser.close()
            if playwright:
                playwright.stop()
            if process and process.poll() is None:
                process.terminate()
                process.wait(timeout=10)


if __name__ == "__main__":
    main()
