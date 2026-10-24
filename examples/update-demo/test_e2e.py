#!/usr/bin/env python3
"""Run a real license server and demo website, then upgrade and roll back the website."""
import argparse
import datetime
import hashlib
import io
import json
import os
import pathlib
import secrets
import shutil
import socket
import sqlite3
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid
import zipfile

HERE = pathlib.Path(__file__).resolve().parent
REPO = HERE.parent.parent


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def http(base, path, data=None, token="", content_type="application/json", expected=200):
    headers = {"Content-Type": content_type}
    if token:
        headers["Authorization"] = "Bearer " + token
    payload = data if isinstance(data, bytes) else json.dumps(data).encode() if data is not None else None
    request = urllib.request.Request(base + path, data=payload, headers=headers)
    try:
        response = urllib.request.urlopen(request, timeout=40)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        assert response.status == expected, f"{path}: HTTP {response.status}, expected {expected}"
        if "json" not in response.headers.get("Content-Type", ""):
            return body
        return json.loads(body)


def api(base, path, data=None, token="", content_type="application/json"):
    envelope = http(base, path, data, token, content_type)
    assert envelope["success"], path
    return envelope["data"]


def initialize(base, username, password, dsn):
    if not api(base, "/v1/setup/status")["initialized"]:
        return api(base, "/v1/setup/initialize", {"database_driver":"sqlite", "database_dsn":dsn, "admin_username":username, "admin_password":password})


def wait_ready(base, path, process):
    for _ in range(100):
        if process.poll() is not None:
            raise RuntimeError("test service exited before readiness")
        try:
            http(base, path)
            return
        except (OSError, AssertionError):
            time.sleep(0.1)
    raise RuntimeError("test service failed to become ready")


def archive(version, schema, healthy=True):
    headlines = {"1.0.0": "让网站管理更简单。", "2.0.0": "网站已升级，客户数据完整保留。", "2.1.0": "用于验证失败回滚的版本。"}
    html = (HERE / "site.html").read_text().replace("@@headline@@", headlines[version]).replace("@@description@@", "Ebox " + version + " · " + ("支持文章标签和新版数据库结构" if schema > 1 else "文章管理与授权接入"))
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as package:
        package.writestr("index.html", html)
        package.writestr("release.json", json.dumps({"version": version, "schema_version": schema, "healthy": healthy}))
    return output.getvalue()


def publish(base, product, developer, version, package):
    prefix = "/developer/products/" + product["id"]
    boundary = uuid.uuid4().hex
    parts = []
    for key, value in [("version", version), ("notes", "Independent website update E2E")]:
        parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{key}"\r\n\r\n{value}\r\n'.encode())
    parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="package"; filename="ebox-{version}.zip"\r\nContent-Type: application/zip\r\n\r\n'.encode() + package + f'\r\n--{boundary}--\r\n'.encode())
    release = api(base, prefix + "/releases", b"".join(parts), developer, "multipart/form-data; boundary=" + boundary)
    api(base, prefix + "/releases/" + release["id"] + "/publish", {}, developer)
    return release


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--license-server", required=True, type=pathlib.Path)
    parser.add_argument("--artifacts", required=True, type=pathlib.Path)
    parser.add_argument("--screenshots", action="store_true", help="Requires Python Playwright and Chromium")
    parser.add_argument("--license-mode", choices=["subscription", "perpetual"], default="subscription")
    args = parser.parse_args()
    binary = args.license_server.resolve(strict=True)
    artifacts = args.artifacts.resolve()
    artifacts.mkdir(parents=True, exist_ok=True)
    node = shutil.which("node")
    if not node:
        raise RuntimeError("Node.js is required")
    report = {"test": "independent Ebox website online update", "license_mode":args.license_mode, "run_id": uuid.uuid4().hex, "started_at": datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat(), "result": "running", "cases": []}
    (artifacts / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
    with tempfile.TemporaryDirectory(prefix="yunnuo-update-demo-") as temporary:
        temporary = pathlib.Path(temporary)
        license_db = temporary / "license.sqlite"
        site_data = temporary / "site"
        password = secrets.token_hex(24)
        license_port, site_port = free_port(), free_port()
        while site_port == license_port:
            site_port = free_port()
        license_base, site_base = f"http://127.0.0.1:{license_port}", f"http://127.0.0.1:{site_port}"
        environment = os.environ.copy()
        environment.update(YN_ADDR=f"127.0.0.1:{license_port}", YN_DB_DRIVER="sqlite", YN_DB=f"file:{license_db}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", YN_DB_CONFIG_FILE=str(temporary / "database.json"), YN_RELEASE_DIR=str(temporary / "packages"), YN_ADMIN_USERNAME="demo-root", YN_ADMIN_PASSWORD=password, YN_CARD_PEPPER=secrets.token_hex(32), YN_DATA_KEY=secrets.token_hex(32))
        processes = []
        browser = None
        playwright = None
        with (artifacts / "license-server.log").open("w") as license_log, (artifacts / "demo-server.log").open("w") as site_log:
            try:
                license_process = subprocess.Popen([str(binary)], cwd=REPO / "backend", env=environment, stdout=license_log, stderr=license_log)
                processes.append(license_process)
                wait_ready(license_base, "/healthz", license_process)
                initialize(license_base,"demo-root",password,environment["YN_DB"])
                admin = api(license_base, "/admin/login", {"username": "demo-root", "password": password})["access_token"]
                api(license_base, "/admin/users", {"username": "demo-developer", "password": password, "role": "developer"}, admin)
                developer = api(license_base, "/developer/login", {"username": "demo-developer", "password": password})["access_token"]
                product = api(license_base, "/developer/products", {"name": "Ebox Website Demo", "code": "EBOX", "bind_mode": "domain", "license_mode":args.license_mode,"expire_grace_days":0}, developer)
                prefix = "/developer/products/" + product["id"]
                api(license_base, prefix + "/updates", {"enabled": True}, developer)
                batch = api(license_base, prefix + "/card-batches", {"name": "Website demo", "quantity": 1, "duration_days": 30}, developer)
                binding = {"bind_mode": "domain", "bind_value": "demo.ebox.example.com"}
                license = api(license_base, "/v1/licenses/activate", {**binding, "app_key": product["app_key"], "card_code": batch["codes"][0]})
                binding["license_no"] = license["license_no"]
                initial_archive = temporary / "initial.zip"
                initial_archive.write_bytes(archive("1.0.0", 1))
                subprocess.run([shutil.which("python3"), str(HERE / "installer.py"), "init", str(site_data), str(initial_archive)], check=True, capture_output=True)
                configuration = temporary / "private-config.json"
                configuration.write_text(json.dumps({"baseUrl": license_base, "appKey": product["app_key"], "productId": product["id"], "publicKeyPem": product["public_key_pem"], "binding": binding}))
                configuration.chmod(0o600)
                site_environment = os.environ.copy()
                site_environment.update(YN_DEMO_CONFIG=str(configuration), YN_DEMO_DATA_DIR=str(site_data), YN_DEMO_PORT=str(site_port), YN_DEMO_PYTHON=shutil.which("python3"))
                def start_site():
                    process = subprocess.Popen([node, str(HERE / "server.mjs")], cwd=REPO, env=site_environment, stdout=site_log, stderr=site_log)
                    processes.append(process)
                    wait_ready(site_base, "/api/health", process)
                    return process
                site_process = start_site()
                before = http(site_base, "/api/status")
                before_html = http(site_base, "/")
                assert before["version"] == "1.0.0" and before["schema_version"] == 1
                if args.screenshots:
                    from playwright.sync_api import sync_playwright
                    playwright = sync_playwright().start()
                    browser = playwright.chromium.launch(executable_path=shutil.which("chromium"), headless=True, args=["--no-sandbox"])
                    page = browser.new_page(viewport={"width": 1440, "height": 1000}, locale="zh-CN", timezone_id="Asia/Shanghai")
                    page.goto(site_base)
                    page.get_by_text("v1.0.0", exact=True).wait_for()
                    page.screenshot(path=str(artifacts / "before-v1.png"), full_page=True)
                publish(license_base, product, developer, "2.0.0", archive("2.0.0", 2))
                with sqlite3.connect(license_db) as fixture:
                    license_id, original_expiry = fixture.execute("SELECT id, expired_at FROM licenses WHERE license_no = ?", (license["license_no"],)).fetchone()
                    expired = (datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=1)).isoformat().replace("+00:00", "Z")
                    if args.license_mode=="perpetual":
                        original_expiry=fixture.execute("SELECT updates_expired_at FROM license_entitlements WHERE license_id = ?",(license_id,)).fetchone()[0]
                        fixture.execute("UPDATE license_entitlements SET updates_expired_at = ? WHERE license_id = ?",(expired,license_id))
                    else:
                        fixture.execute("UPDATE licenses SET expired_at = ? WHERE license_no = ?", (expired, license["license_no"]))
                denied = http(site_base, "/api/update", {}, expected=403)
                assert denied["code"] == ("UPDATES_EXPIRED" if args.license_mode=="perpetual" else "LICENSE_EXPIRED")
                credentials={**binding,"app_key":product["app_key"]}
                if args.license_mode=="perpetual":
                    rights=api(license_base,"/v1/licenses/verify",credentials)
                    assert rights["can_use"] and not rights["can_update"] and rights["usage_expired_at"] is None
                    assert api(license_base,"/v1/licenses/heartbeat",credentials)["accepted"]
                    assert http(site_base,"/")==before_html
                    assert http(site_base,"/api/status")["version"]=="1.0.0"
                else:
                    assert http(license_base,"/v1/licenses/verify",credentials,expected=403)["error"]["code"]=="LICENSE_EXPIRED"
                    assert http(license_base,"/v1/licenses/heartbeat",credentials,expected=403)["error"]["code"]=="LICENSE_EXPIRED"
                    blocked_page=http(site_base,"/",expected=403).decode()
                    assert "应用授权暂不可用" in blocked_page and "已确认的授权安全处置" not in blocked_page
                    assert http(site_base,"/api/status",expected=403)["code"]=="LICENSE_EXPIRED"
                assert (site_data/"current"/"index.html").read_bytes()==before_html
                report["cases"].append({"name": "Expired update rights reject updates; perpetual website stays usable and expired subscription website denies access", "passed": True, "fixture": "Only the isolated test entitlement deadline was moved to yesterday"})
                with sqlite3.connect(license_db) as fixture:
                    if args.license_mode=="perpetual":
                        fixture.execute("UPDATE license_entitlements SET updates_expired_at = ? WHERE license_id = ?",(original_expiry,license_id))
                    else:
                        fixture.execute("UPDATE licenses SET expired_at = ? WHERE license_no = ?", (original_expiry, license["license_no"]))
                if args.screenshots:
                    page.get_by_role("button", name="检查并安装更新").click()
                    page.get_by_text("v2.0.0", exact=True).wait_for()
                    page.get_by_text("Schema 2", exact=True).wait_for()
                    page.get_by_role("heading", name="网站已升级，客户数据完整保留。", exact=True).wait_for()
                    page.screenshot(path=str(artifacts / "after-v2.png"), full_page=True)
                else:
                    result = http(site_base, "/api/update", {})
                    assert result["updated"]
                after = http(site_base, "/api/status")
                after_html = http(site_base, "/")
                assert after["version"] == "2.0.0" and after["schema_version"] == 2 and after["notes"] == before["notes"]
                assert after_html != before_html and "网站已升级".encode() in after_html
                report["cases"].append({"name": "Installed website upgraded 1.0.0 to 2.0.0 after licensed signed download", "passed": True, "before_html_sha256": hashlib.sha256(before_html).hexdigest(), "after_html_sha256": hashlib.sha256(after_html).hexdigest()})
                report["cases"].append({"name": "Database migrated schema 1 to 2 and existing customer article survived", "passed": True})
                site_process.terminate(); site_process.wait(timeout=10)
                site_process = start_site()
                restarted = http(site_base, "/api/status")
                assert restarted["version"] == "2.0.0" and restarted["schema_version"] == 2 and restarted["notes"] == before["notes"]
                report["cases"].append({"name": "Website restart preserves new installed version and customer data", "passed": True})
                broken = publish(license_base, product, developer, "2.1.0", archive("2.1.0", 3, healthy=False))
                failure = http(site_base, "/api/update", {}, expected=500)
                assert failure["code"] == "UPDATE_FAILED"
                assert "新版健康检查未通过" in failure["message"]
                rolled_back = http(site_base, "/api/status")
                assert rolled_back["version"] == "2.0.0" and rolled_back["schema_version"] == 2
                assert rolled_back["notes"] == before["notes"] and "version_three_only" not in rolled_back["tables"]
                assert http(site_base, "/") == after_html and not (site_data / "pending.json").exists()
                assert any("已备份数据库并切换网站文件至 v2.1.0" in event for event in rolled_back["events"])
                assert any("网站文件与数据库已回滚至 v2.0.0" in event for event in rolled_back["events"])
                report["cases"].append({"name": "Unhealthy 2.1.0 rolls back website files and schema 3 migration to healthy 2.0.0/schema 2", "passed": True})
                if args.screenshots:
                    page.reload(); page.get_by_text("v2.0.0", exact=True).wait_for()
                    page.get_by_text("Schema 2", exact=True).wait_for()
                    page.screenshot(path=str(artifacts / "rollback-v2.png"), full_page=True)
                api(license_base, prefix + "/releases/" + broken["id"] + "/withdraw", {}, developer)
                no_update = http(site_base, "/api/update", {})
                assert no_update["updated"] is False
                report["cases"].append({"name": "After broken version withdrawal the current version reports no update", "passed": True})
                report["result"] = "passed"
                report["limits"] = ["Local loopback services, not an Internet deployment", "Installer and three known schema migrations apply only to this demo, not arbitrary Ebox installations"]
                (artifacts / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
                print(json.dumps({"result": "passed", "cases": len(report["cases"]), "artifacts": str(artifacts)}, ensure_ascii=False))
            finally:
                if report["result"] != "passed":
                    report["result"] = "failed"
                    (artifacts / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
                if browser:
                    browser.close()
                if playwright:
                    playwright.stop()
                for process in reversed(processes):
                    if process.poll() is None:
                        process.terminate()
                        process.wait(timeout=10)


if __name__ == "__main__":
    main()
