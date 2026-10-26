#!/usr/bin/env python3
"""Administrator tier/rate management, authoritative quotes and historical cost evidence."""
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
    parser.add_argument("--verify-state", type=pathlib.Path)
    args = parser.parse_args()
    args.artifacts.mkdir(parents=True, exist_ok=True)
    report = {"result": "running", "checks": []}
    password = os.environ["YN_TEST_ADMIN_PASSWORD"] if args.base_url else secrets.token_hex(24)
    suffix = uuid.uuid4().hex[:8]
    process = browser = pw = None
    with tempfile.TemporaryDirectory(prefix="agent-tiers-") as temporary, (args.artifacts / "server.log").open("w") as log:
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
                tiers = http.api(base, "/admin/commerce", token=root)["tiers"]
                gold = next(t for t in tiers if t["id"] == "tier_gold")
                assert gold["cost_rate_bps"] == 5000 and gold["sort_order"] == 5 and gold["name"] == "金牌合作代理"
                account = http.api(base, "/admin/agents/"+saved["agent_id"]+"/account", token=root)
                assert account["account"]["balance"] == 3743 and account["packages"][0]["unit_cost"] == 501
                assert sorted(l["unit_cost"] for l in account["ledgers"] if l["kind"] == "issue") == [501, 756]
                report = saved
                print(json.dumps({"result":"passed","check":"Edited tier rate, order and historical costs survived MySQL restart"}))
                return
            tiers = http.api(base, "/admin/commerce", token=root)["tiers"]
            defaults = {t["id"]:t for t in tiers if t["id"] in ["tier_standard", "tier_advanced", "tier_gold"]}
            assert len(defaults) == 3
            assert [defaults[i]["cost_rate_bps"] for i in ["tier_standard", "tier_advanced", "tier_gold"]] == [10000, 9000, 8000]
            if not args.base_url:
                assert len(tiers) == 3
            report["checks"].append("Fresh installations contain three configurable default tiers at 100%, 90% and 80%")
            product = http.api(base, "/admin/products", {"name":"Ebox 授权应用", "bind_mode":"domain"}, root)
            pkg = http.api(base, "/admin/packages", {"product_id":product["id"], "name":"月度授权", "duration_days":30, "base_cost":1001}, root)
            username = "tier-owner-"+suffix
            opened = http.api(base, "/admin/agent-accounts", {"username":username,"password":password,"name":"金牌合作伙伴","tier_id":"tier_gold","package_ids":[pkg["id"]],"initial_balance":5000,"reason":"等级结算验收"}, root)
            agent = opened["agent"]
            token = http.api(base, "/agent/login", {"username":username,"password":password})["access_token"]
            assert http.api(base, "/agent/packages", token=token)[0]["unit_cost"] == 801
            update = {"name":"金牌合作代理","cost_rate_bps":7550,"sort_order":5}
            errors = []
            if args.screenshots:
                from playwright.sync_api import sync_playwright
                pw = sync_playwright().start()
                browser = pw.chromium.launch(executable_path=None if args.chromium == "bundled" else args.chromium, headless=True, args=["--no-sandbox"])
                context = browser.new_context(viewport={"width":1500,"height":1050}, locale="zh-CN")
                page = context.new_page()
                page.on("pageerror",lambda e:errors.append(str(e)))
                page.goto(base+"/admin#commerce")
                page.get_by_label("账号",exact=True).fill("admin")
                page.get_by_label("密码",exact=True).fill(password)
                page.get_by_role("button",name="安全登录",exact=True).click()
                table = page.get_by_role("table",name="代理等级列表",exact=True)
                table.get_by_role("cell",name="80%",exact=True).wait_for()
                panel = page.locator("section.panel").filter(has=page.get_by_role("heading",name="代理等级与价格",exact=True))
                panel.screenshot(path=str(args.artifacts/"default-three-tiers.png"),animations="disabled")
                gold_row = table.get_by_role("row").filter(has=page.get_by_role("cell",name="金牌代理",exact=True))
                assert gold_row.get_by_role("button",name="删除等级",exact=True).is_disabled()
                gold_row.get_by_role("button",name="编辑等级",exact=True).click()
                dialog = page.get_by_role("dialog",name="编辑代理等级",exact=True)
                dialog.get_by_label("等级名称").fill(update["name"])
                dialog.get_by_label("成本比例（%）").fill("75.5")
                dialog.get_by_label("显示排序").fill("5")
                page.screenshot(path=str(args.artifacts/"admin-edit-tier-rate.png"),full_page=True,animations="disabled")
                page.emulate_media(color_scheme="dark")
                page.locator("html[data-theme='dark']").wait_for(state="attached")
                for width in [390,320]:
                    page.set_viewport_size({"width":width,"height":844})
                    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth+1")
                    if width==390:
                        page.screenshot(path=str(args.artifacts/"admin-tier-rate-mobile-dark.png"),animations="disabled")
                with page.expect_response(lambda r:r.url.endswith("/admin/agent-tiers/tier_gold") and r.request.method=="POST") as response:
                    dialog.get_by_role("button",name="保存等级",exact=True).click()
                assert response.value.status==200 and response.value.json()["data"]["cost_rate_bps"]==7550
                dialog.wait_for(state="hidden")
                table.get_by_role("cell",name="75.5%",exact=True).wait_for()
            else:
                http.api(base,"/admin/agent-tiers/tier_gold",update,root)
            quote = http.api(base,"/agent/packages",token=token)[0]
            assert quote["unit_cost"]==756 and quote["price_source"]=="tier_ratio"
            if args.screenshots:
                owner = browser.new_context(viewport={"width":1440,"height":1000},locale="zh-CN",color_scheme="light")
                user = owner.new_page()
                user.on("pageerror",lambda e:errors.append(str(e)))
                user.goto(base+"/agent-console/")
                user.get_by_label("登录名",exact=True).fill(username)
                user.get_by_label("密码",exact=True).fill(password)
                user.get_by_role("button",name="进入工作台",exact=True).click()
                user.get_by_role("cell",name="¥7.56",exact=True).wait_for()
                user.screenshot(path=str(args.artifacts/"agent-discounted-package.png"),full_page=True,animations="disabled")
                user.emulate_media(color_scheme="dark")
                user.locator("html[data-theme='dark']").wait_for(state="attached")
                user.set_viewport_size({"width":390,"height":844})
                assert user.evaluate("document.documentElement.scrollWidth <= innerWidth+1")
                user.screenshot(path=str(args.artifacts/"agent-tier-price-mobile-dark.png"),animations="disabled")
            report["checks"].append("Administrator edits name, 75.5% rate and display order in the real UI; a 1001-cent package costs 756 cents, visible to the agent on desktop/mobile")
            issue = {"package_id":pkg["id"],"quantity":1,"request_id":uuid.uuid4().hex,"expected_unit_cost":756}
            first = http.api(base,"/agent/card-batches",issue,token)
            http.api(base,"/admin/agent-tiers/tier_gold",{**update,"cost_rate_bps":5000},root)
            assert http.http(base,"/agent/card-batches",{**issue,"request_id":uuid.uuid4().hex},token,expected=400)["error"]["code"]=="PACKAGE_PRICE_CHANGED"
            assert http.api(base,"/agent/card-batches",issue,token)["batch"]["id"]==first["batch"]["id"]
            http.api(base,"/agent/card-batches",{**issue,"request_id":uuid.uuid4().hex,"expected_unit_cost":501},token)
            assert http.api(base,"/agent/account",token=token)["balance"]==3743
            costs = sorted(l["unit_cost"] for l in http.api(base,"/agent/money-ledgers",token=token) if l["kind"]=="issue")
            assert costs==[501,756]
            report["checks"].append("Rate changes reject stale quotes for new orders; replay keeps the original cards, debit and historical unit cost")
            price_path = "/admin/agent-tiers/tier_gold/prices"
            grant_path = "/admin/agents/"+agent["id"]+"/packages"
            http.api(base,price_path,{"package_id":pkg["id"],"unit_cost":700},root)
            assert http.api(base,"/agent/packages",token=token)[0]["unit_cost"]==700
            grant={"package_id":pkg["id"],"enabled":True,"max_batch_quantity":100,"override_cost":500}
            http.api(base,grant_path,grant,root)
            assert http.api(base,"/agent/packages",token=token)[0]["unit_cost"]==500
            http.api(base,grant_path,{**grant,"override_cost":None},root)
            assert http.api(base,"/agent/packages",token=token)[0]["unit_cost"]==700
            http.api(base,price_path,{"package_id":pkg["id"],"unit_cost":None},root)
            assert http.api(base,"/agent/packages",token=token)[0]["unit_cost"]==501
            report["checks"].append("Agent-specific price overrides tier-package price, which overrides the tier percentage; removing overrides restores percentage pricing")
            http.http(base,"/admin/agent-tiers/tier_gold",update,token,expected=401)
            http.http(base,"/admin/agent-tiers/tier_gold",{**update,"cost_rate_bps":0},root,expected=400)
            import urllib.error
            import urllib.request
            def delete(path,expected=200):
                request=urllib.request.Request(base+path,method="DELETE",headers={"Authorization":"Bearer "+root})
                try:response=urllib.request.urlopen(request,timeout=20)
                except urllib.error.HTTPError as error:response=error
                with response:assert response.status==expected
            delete("/admin/agent-tiers/tier_gold",409)
            disposable=http.api(base,"/admin/agent-tiers",{"name":"临时代理等级","cost_rate_bps":6000,"sort_order":99},root)
            delete("/admin/agent-tiers/"+disposable["id"])
            assert not any(t["id"]==disposable["id"] for t in http.api(base,"/admin/commerce",token=root)["tiers"])
            report["checks"].append("Agents cannot edit tiers, invalid rates are rejected, assigned tiers cannot be deleted, and unused tiers can be added/deleted")
            assert not errors,errors
            report.update(result="passed",agent_id=agent["id"])
            (args.artifacts/"failure.png").unlink(missing_ok=True)
            print(json.dumps({"result":"passed","checks":len(report["checks"])}))
        except Exception:
            if args.screenshots and "page" in locals():
                page.screenshot(path=str(args.artifacts/"failure.png"),full_page=True)
            raise
        finally:
            if report["result"]!="passed":report["result"]="failed"
            (args.artifacts/"report.json").write_text(json.dumps(report,ensure_ascii=False,indent=2))
            if browser:browser.close()
            if pw:pw.stop()
            if process:process.terminate();process.wait(timeout=10)


if __name__=="__main__":
    main()
