#!/usr/bin/env python3
"""Browser appearance, responsive layout, persistence and keyboard acceptance checks."""
import argparse
import importlib.util
import json
import os
import pathlib
import secrets
import subprocess
import traceback
import tempfile

from playwright.sync_api import sync_playwright

REPO = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("http_helpers", REPO / "examples/update-demo/test_e2e.py")
http = importlib.util.module_from_spec(spec)
spec.loader.exec_module(http)


def select_theme(page, label):
    page.get_by_role("button", name="外观主题", exact=True).click()
    page.get_by_role("menuitemradio", name=label, exact=True).click()
    assert page.get_by_role("button", name="外观主题", exact=True).get_attribute("aria-expanded") == "false"


def theme_is(page, expected):
    page.locator(f"html[data-theme='{expected}']").wait_for(state="attached")
    assert page.locator("html").evaluate("e => getComputedStyle(e).colorScheme") == expected
    colors = page.locator("html").evaluate("e => { const s = getComputedStyle(e); return ['--canvas', '--paper', '--field', '--red-soft', '--amber-soft'].map(k => s.getPropertyValue(k).trim()); }")
    assert all(colors), colors


def verify_layout(page, name):
    result = page.evaluate("""() => ({
      width: innerWidth, page: document.documentElement.scrollWidth,
      // Read visibility and geometry in one frame. Vue may replace controls
      // between separate locator RPCs while route data or branding loads.
      controls: [...document.querySelectorAll('input, select, textarea')].flatMap(e => {
        const r=e.getBoundingClientRect(), s=getComputedStyle(e);
        if(e.closest('.table-wrap') || !r.width || !r.height || s.visibility==='hidden' || s.display==='none') return [];
        return [{tag:e.tagName,x:r.x,width:r.width}];
      }),
      overflow: [...document.querySelectorAll('body *')].filter(e => {
        const r = e.getBoundingClientRect(), s = getComputedStyle(e);
        return r.width && r.right > innerWidth + 1 && s.position !== 'fixed'
          && !e.closest('.table-wrap, .nav, .code-box, .query-types, .license-list');
      }).slice(0,5).map(e => e.tagName + '.' + e.className)
    })""")
    assert result["page"] <= result["width"] + 1, (name, result)
    # Each form control remains inside the viewport, except horizontally scrolling tables.
    for box in result["controls"]:
        assert box["x"] >= -1 and box["x"] + box["width"] <= result["width"] + 1, (name, box)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server", required=True, type=pathlib.Path)
    parser.add_argument("--artifacts", required=True, type=pathlib.Path)
    parser.add_argument("--chromium", default="/usr/bin/chromium", help="Chromium path, or 'bundled' for Playwright's browser")
    parser.add_argument("--screenshots", action="store_true")
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    report = {"result": "running", "checks": [], "pages": []}
    password = secrets.token_hex(24)
    process = None
    with tempfile.TemporaryDirectory(prefix="appearance-") as temporary, (args.artifacts / "server.log").open("w") as log:
        try:
            port = http.free_port()
            base = f"http://127.0.0.1:{port}"
            env = os.environ.copy()
            env.update(YN_ADDR=f"127.0.0.1:{port}", YN_DB_DRIVER="sqlite", YN_DB=f"file:{temporary}/db.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", YN_DB_CONFIG_FILE=temporary + "/config.json", YN_RELEASE_DIR=temporary + "/packages")
            process = subprocess.Popen([str(args.server.resolve())], cwd=REPO / "backend", env=env, stdout=log, stderr=log)
            http.wait_ready(base, "/healthz", process)
            with sync_playwright() as pw:
                browser = pw.chromium.launch(executable_path=None if args.chromium == "bundled" else args.chromium, headless=True, args=["--no-sandbox"])
                report["browser"] = browser.version
                context = browser.new_context(viewport={"width": 1440, "height": 1050}, color_scheme="light", locale="zh-CN")
                page = context.new_page()
                errors = []
                context.on("page", lambda p: p.on("pageerror", lambda e: errors.append(str(e))))
                page.on("pageerror", lambda e: errors.append(str(e)))
                page.goto(base + "/")
                assert page.url == base + "/install"
                page.get_by_role("button", name="外观主题", exact=True).wait_for()
                theme_is(page, "light")
                assert page.locator("html").get_attribute("data-theme-preference") == "system"
                page.emulate_media(color_scheme="dark")
                theme_is(page, "dark")
                select_theme(page, "浅色")
                page.emulate_media(color_scheme="light")
                page.emulate_media(color_scheme="dark")
                theme_is(page, "light")
                page.reload()
                theme_is(page, "light")
                report["checks"].append("System mode follows live OS changes; explicit choice overrides OS and survives reload")
                # The bootstrap script must work before Vue/module scripts run.
                probe = context.new_page()
                probe.route("**/*.js", lambda route: route.continue_() if route.request.url.endswith("/assets/theme-init.js") else route.abort())
                probe.goto(base + "/install")
                theme_is(probe, "light")
                assert probe.locator('meta[name="theme-color"]').get_attribute("content") == "#f3f5f2"
                probe.close()
                report["checks"].append("Saved theme is applied before the Vue application loads")
                for width, device in [(1440, "desktop"), (390, "mobile"), (320, "small-mobile")]:
                    page.set_viewport_size({"width": width, "height": 1050 if width > 620 else 844})
                    for mode, label in [("light", "浅色"), ("dark", "深色")]:
                        select_theme(page, label)
                        verify_layout(page, "install-" + device)
                        if args.screenshots and width != 320:
                            page.screenshot(path=str(args.artifacts / f"install-{device}-{mode}.png"), full_page=True, animations="disabled")
                http.initialize(base, "admin", password, env["YN_DB"])
                root = http.api(base, "/admin/login", {"username": "admin", "password": password})["access_token"]
                http.api(base, "/admin/users", {"username": "developer", "password": password, "role": "developer", "display_name": "小王开发者"}, root)
                developer = http.api(base, "/developer/login", {"username": "developer", "password": password})["access_token"]
                product = http.api(base, "/developer/products", {"name": "Ebox 企业网站", "code": "EBOX", "bind_mode": "domain"}, developer)
                customer = http.api(base, "/admin/customers", {"username": "customer", "password": password, "display_name": "正版应用用户"}, root)
                customer_token = http.api(base, "/user/login", {"username": customer["username"], "password": password})["access_token"]
                batch = http.api(base, "/admin/card-batches", {"product_id": product["id"], "name": "主题验收", "quantity": 1, "duration_days": 30}, root)
                http.api(base, "/user/activate", {"application_code": "EBOX", "card_code": batch["codes"][0], "bind_value": "ebox.example.com"}, customer_token)
                agent = http.api(base, "/admin/agents", {"name": "企业应用代理"}, root)
                http.api(base, "/admin/agents/" + agent["id"] + "/users", {"username": "owner", "password": password}, root)
                agent_token = http.api(base, "/agent/login", {"login_code": agent["login_code"], "username": "owner", "password": password})["access_token"]
                tokens = {"yn.admin_token": root, "yn.agent_token": agent_token, "yn.user_token": customer_token, "yn.developer_token": developer}
                page.goto(base + "/")
                page.evaluate("tokens => { for (const [k,v] of Object.entries(tokens)) sessionStorage.setItem(k,v); }", tokens)
                pages = [("public", "/"), ("admin", "/admin"), ("agent", "/agent-console/"), ("user", "/user-console/"), ("developer", "/developer-console/"), ("recover", "/recover")]
                for width, device in [(1440, "desktop"), (390, "mobile"), (320, "small-mobile")]:
                    page.set_viewport_size({"width": width, "height": 1050 if width > 620 else 844})
                    for name, path in pages:
                        page.goto(base + path)
                        page.get_by_role("button", name="外观主题", exact=True).wait_for()
                        if name in ["admin", "agent"]:
                            page.locator(".console .workspace").wait_for()
                        if name == "user":
                            page.get_by_text("Ebox 企业网站", exact=False).first.wait_for()
                        for mode, label in [("light", "浅色"), ("dark", "深色")]:
                            select_theme(page, label)
                            theme_is(page, mode)
                            verify_layout(page, name + "-" + device)
                            if args.screenshots and width != 320:
                                page.screenshot(path=str(args.artifacts / f"{name}-{device}-{mode}.png"), full_page=True, animations="disabled")
                        report["pages"].append(name + "-" + device)
                # Fallback font metrics and text enlargement must not widen identifiers.
                page.set_viewport_size({"width": 320, "height": 844})
                page.goto(base + "/agent-console/")
                page.locator(".console .workspace").wait_for()
                page.add_style_tag(content="code { font-family: monospace !important; font-size: 20px !important; }")
                verify_layout(page, "agent-small-mobile-large-monospace")
                if args.screenshots:
                    page.screenshot(path=str(args.artifacts / "agent-small-mobile-large-font.png"), full_page=True, animations="disabled")
                report["checks"].append("Long identifiers wrap on a 320px phone with enlarged monospace fallback fonts")
                # Exercise real mobile navigation and a horizontally scrolling history table.
                page.set_viewport_size({"width": 390, "height": 844})
                page.goto(base + "/admin#recovery")
                page.get_by_role("heading", name="账号密码恢复", exact=True).wait_for()
                verify_layout(page, "mobile-account-recovery")
                page.get_by_role("link", name="套餐与结算", exact=True).click()
                page.get_by_role("tab", name="代理运营", exact=True).click()
                page.get_by_role("heading", name="代理授权与资金", exact=True).wait_for()
                verify_layout(page, "mobile-commerce")
                if args.screenshots:
                    page.screenshot(path=str(args.artifacts / "admin-mobile-commerce-dark.png"), full_page=True, animations="disabled")
                for width in [390, 320]:
                    page.set_viewport_size({"width": width, "height": 844})
                    for label in ["产品", "产品升级", "代理", "套餐与结算", "用户", "账号恢复", "卡密", "授权", "风控", "盗版处置", "联调工具", "离线授权", "审计", "系统"]:
                        page.get_by_role("link", name=label, exact=True).click()
                        verify_layout(page, f"admin-{label}-{width}")
                page.get_by_role("link", name="盗版处置", exact=True).click()
                history = page.locator(".paged-records").first
                history.get_by_label("开始时间", exact=True).fill("2026-01-01T00:00")
                history.get_by_label("结束时间（不含）", exact=True).fill("2027-01-01T00:00")
                with page.expect_response(lambda r: "/admin/history/cases?" in r.url and "from=" in r.url) as filtered:
                    history.get_by_role("button", name="查询历史记录", exact=True).click()
                assert filtered.value.status == 200
                verify_layout(page, "admin-security-filled-dates-320")
                if args.screenshots:
                    page.screenshot(path=str(args.artifacts / "admin-security-small-mobile-date-filters.png"), full_page=True, animations="disabled")
                report["checks"].append("Native date/time filters fit 320px phones with populated dates and submit successfully")
                report["checks"].append("Seven entry pages adapt to desktop, 390px and 320px phones; mobile navigation and forms remain usable")
                guest = browser.new_context(viewport={"width": 390, "height": 844}, color_scheme="dark", locale="zh-CN")
                guest_page = guest.new_page()
                guest_page.on("pageerror", lambda e: errors.append(str(e)))
                for name, path in pages[1:5]:
                    guest_page.goto(base + path)
                    guest_page.get_by_role("button", name="外观主题", exact=True).wait_for()
                    guest_page.locator('input[type="password"]').first.wait_for()
                    for width in [390, 320]:
                        guest_page.set_viewport_size({"width": width, "height": 844})
                        for mode, label in [("light", "浅色"), ("dark", "深色")]:
                            select_theme(guest_page, label)
                            verify_layout(guest_page, f"{name}-login-{width}-{mode}")
                        if args.screenshots and width == 390:
                            guest_page.screenshot(path=str(args.artifacts / f"{name}-login-mobile-dark.png"), full_page=True, animations="disabled")
                guest.close()
                report["checks"].append("All four account login forms support mobile light/dark mode; every administrator navigation section fits narrow phones")
                # Cross-page / cross-tab persistence and auto switching when system mode is restored.
                page.goto(base + "/")
                other = context.new_page()
                other.goto(base + "/recover")
                other.get_by_role("button", name="外观主题", exact=True).wait_for()
                select_theme(page, "浅色")
                theme_is(other, "light")
                select_theme(page, "跟随系统")
                page.emulate_media(color_scheme="dark")
                theme_is(page, "dark")
                page.emulate_media(color_scheme="light")
                theme_is(page, "light")
                other.close()
                # Keyboard access and Escape restore focus to the trigger.
                trigger = page.get_by_role("button", name="外观主题", exact=True)
                trigger.focus()
                page.keyboard.press("ArrowDown")
                page.get_by_role("menu", name="选择外观主题", exact=True).wait_for()
                page.keyboard.press("Home")
                page.keyboard.press("ArrowDown")
                page.keyboard.press("Enter")
                theme_is(page, "dark")
                trigger.click()
                page.keyboard.press("Escape")
                assert trigger.get_attribute("aria-expanded") == "false"
                assert trigger.evaluate("e => e === document.activeElement")
                report["checks"].append("Theme choice is shared across pages and tabs; keyboard, Escape and system-mode restoration work")
                assert not errors, errors
                browser.close()
            report["result"] = "passed"
            print(json.dumps({"result": "passed", "checks": len(report["checks"]), "layouts": len(report["pages"]), "artifacts": str(args.artifacts)}))
        except Exception:
            report["error"] = traceback.format_exc()
            raise
        finally:
            if report["result"] != "passed":
                report["result"] = "failed"
            (args.artifacts / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
            if process and process.poll() is None:
                process.terminate()
                process.wait(timeout=10)


if __name__ == "__main__":
    main()
