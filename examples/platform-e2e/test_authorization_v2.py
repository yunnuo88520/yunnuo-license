#!/usr/bin/env python3
"""Operate the application-scoped authorization pages in a real browser."""
from contextlib import contextmanager
import sys
import csv
import argparse, hashlib, importlib.util, io, json, os, pathlib, secrets, sqlite3, subprocess, tempfile, zipfile
from playwright.sync_api import sync_playwright, expect
REPO=pathlib.Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location("v2_http",REPO/"examples/update-demo/test_e2e.py");http=importlib.util.module_from_spec(spec);spec.loader.exec_module(http)
def main():
 parser=argparse.ArgumentParser();parser.add_argument("--server",type=pathlib.Path,required=True);parser.add_argument("--artifacts",type=pathlib.Path,required=True);parser.add_argument("--chromium",default="/usr/bin/chromium");args=parser.parse_args();args.artifacts.mkdir(parents=True,exist_ok=True)
 report={"result":"running","cases":[],"screenshots":[],"source_commit":subprocess.check_output(["git","rev-parse","HEAD"],cwd=REPO,text=True).strip(),"source_dirty":bool(subprocess.check_output(["git","status","--porcelain"],cwd=REPO))}
 def record(name):report["cases"].append({"name":name,"passed":True});print(name,flush=True)
 server=None;page=None;errors=[]
 @contextmanager
 def guarded_browser():
  with sync_playwright() as pw:
   try:yield pw
   except Exception:
    report["browser_errors"]=errors
    if page is not None and not page.is_closed():
     page.screenshot(path=str(args.artifacts/"failure.png"),full_page=True,mask=page.locator("input,textarea").all())
     (args.artifacts/"failure.html").write_text(page.content())
    raise
 with tempfile.TemporaryDirectory(prefix="authorization-v2-") as temporary,(args.artifacts/"server.log").open("w") as log:
  folder=pathlib.Path(temporary);port=http.free_port();base=f"http://127.0.0.1:{port}";database=folder/"license.sqlite";password=secrets.token_hex(24);env=os.environ.copy();env.update(YN_ADDR=f"127.0.0.1:{port}",YN_DB_DRIVER="sqlite",YN_DB=f"file:{database}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)",YN_DB_CONFIG_FILE=str(folder/"database.json"),YN_RELEASE_DIR=str(folder/"packages"),YN_CLIENT_WRITE_RATE_LIMIT="2000",YN_CLIENT_READ_RATE_LIMIT="10000")
  try:
   server=subprocess.Popen([str(args.server.resolve())],cwd=REPO/"backend",env=env,stdout=log,stderr=log);http.wait_ready(base,"/healthz",server)
   with guarded_browser() as pw:
    browser=pw.chromium.launch(executable_path=None if args.chromium=="bundled" else args.chromium,headless=True,args=["--no-sandbox"]);context=browser.new_context(viewport={"width":1440,"height":1050},locale="zh-CN",color_scheme="light");page=context.new_page();errors=[];page.on("pageerror",lambda e:errors.append(str(e)))
    def screenshot(name,mask=None):page.evaluate("window.scrollTo({top:0,behavior:'instant'})");page.screenshot(path=str(args.artifacts/name),full_page=page.viewport_size["width"]>=600,animations="disabled",mask=mask or []);report["screenshots"].append(name)
    page.goto(base+"/admin/");expect(page).to_have_url(base+"/install");record("首次访问管理页强制进入安装引导")
    http.initialize(base,"admin",password,env["YN_DB"]);page.goto(base+"/admin/#applications");page.get_by_label("账号",exact=True).fill("admin");page.get_by_label("密码",exact=True).fill(password);page.get_by_role("button",name="安全登录",exact=True).click();page.get_by_role("heading",name="应用授权中心",exact=True,level=2).wait_for()
    page.get_by_role("button",name="创建应用",exact=True).click();dialog=page.get_by_role("dialog",name="创建应用",exact=True);dialog.get_by_label("应用名称",exact=True).fill("Ebox · 通用产品授权")
    for box in dialog.locator('input[type="checkbox"]').all():box.check()
    dialog.get_by_role("button",name="创建并生成凭证",exact=True).click();credential=page.get_by_role("dialog",name="保存应用服务端凭证",exact=True);credential.wait_for();secret_text=credential.get_by_label("一次性凭证",exact=True).input_value();assert "AppSecret:" in secret_text
    with page.expect_download() as download:credential.get_by_role("button",name="下载保存",exact=True).click()
    download.value.save_as(folder/"private-application-credentials.txt");credential.get_by_role("button",name="关闭",exact=True).click();record("页面创建应用并仅一次展示、下载服务端凭证")
    app_id=secret_text.split("AppID: ",1)[1].splitlines()[0];token=page.evaluate("sessionStorage.getItem('yn.admin_token')");prefix="/v2/console/applications/"+app_id
    assert len(app_id)==18
    retired=http.http(base,"/v1/licenses/verify",{},expected=410);assert retired["error"]["code"]=="AUTHORIZATION_V2_REQUIRED";record("生产入口拒绝旧授权协议，不允许绕过新核心")
    page.get_by_role("button",name="套餐",exact=True).click();page.get_by_label("套餐名称",exact=True).fill("年度标准版");page.get_by_label("桌面 GUI",exact=True).check();page.get_by_label("代理基础成本（元）",exact=True).fill("10");page.get_by_role("button",name="创建套餐",exact=True).click();expect(page.get_by_role("cell").filter(has_text="年度标准版").first).to_be_visible();record("在应用内建立冻结销售规格的授权套餐")
    page.get_by_role("button",name="销售与卡密",exact=True).click();page.get_by_label("数量",exact=True).fill("2");page.get_by_label("客户参考号",exact=True).fill("Ebox 客户交付");page.get_by_label("确认 / 备注",exact=True).fill("已人工收款，场景验收");page.get_by_role("button",name="建立销售订单",exact=True).click();cards=page.get_by_label("本次卡密",exact=True);cards.wait_for();codes=cards.input_value().splitlines();assert len(codes)==2
    with page.expect_download() as download:page.get_by_role("button",name="导出本次卡密 CSV",exact=True).click()
    exported=folder/"private-cards.csv";download.value.save_as(exported);assert all(c in exported.read_text() for c in codes);target_license=next(r["license_id"] for r in csv.DictReader(exported.read_text(encoding="utf-8-sig").splitlines()) if r["code"]==codes[0]);page.get_by_role("button",name="已保存，隐藏原文",exact=True).click();record("销售确认后批量生成卡密并一次性导出")
    with sqlite3.connect(database) as db:
     persisted=db.execute("SELECT data_json,key_hash FROM application_card_keys WHERE application_id=?",(app_id,)).fetchall();assert len(persisted)==2;assert all(code not in json.dumps(persisted) for code in codes)
    record("数据库只保存卡密哈希与尾号，不保存可恢复原文");screenshot("v2-cards-light-desktop.png")
    page.reload();page.get_by_role("heading",name="应用授权中心",exact=True,level=2).wait_for();page.get_by_role("button",name="销售与卡密",exact=True).click();assert page.get_by_label("本次卡密",exact=True).count()==0;record("刷新页面后无法重新获取卡密原文")
    page.get_by_role("button",name="许可证与安装",exact=True).click();page.locator(".application-table").first.get_by_role("row").filter(has_text=target_license).get_by_role("button",name="预录绑定",exact=True).click();dialog=page.get_by_role("dialog",name="预录授权绑定",exact=True);dialog.get_by_label("绑定值",exact=True).fill("example.com");dialog.get_by_label("同组域名别名（每行一个，须含绑定值）",exact=True).fill("example.com\nwww.example.com");dialog.get_by_label("操作原因",exact=True).fill("客户合同预录站点");dialog.get_by_role("button",name="确认预录授权绑定",exact=True).click();expect(dialog).not_to_be_visible();record("在许可证页面预录主域名和 www 别名，无需服务器操作")
    page.locator(".application-table").first.get_by_role("row").filter(has_text=target_license).get_by_role("button",name="安装邀请",exact=True).click();dialog=page.get_by_role("dialog",name="生成一次性安装邀请",exact=True);dialog.get_by_role("button",name="确认生成一次性安装邀请",exact=True).click();invite=page.get_by_role("dialog",name="安装邀请（只显示本次）",exact=True);invite.wait_for();assert "Invitation token:" in invite.get_by_label("一次性凭证",exact=True).input_value();invite.get_by_role("button",name="关闭",exact=True).click();record("通过页面生成一次性安装邀请，支持免卡密产品关联");screenshot("v2-licenses-light-desktop.png")
    page.get_by_role("button",name="域名 / IP 规则",exact=True).click();page.get_by_label("模式",exact=True).fill("*.example.com");page.get_by_label("备注",exact=True).fill("客户泛域名白名单");page.get_by_role("button",name="添加规则",exact=True).click();expect(page.get_by_role("cell").filter(has_text="客户泛域名白名单").first).to_be_visible();record("页面维护泛域名模式与规则范围")
    page.get_by_role("button",name="接入文档",exact=True).click();page.get_by_role("heading",name="产品接入文档",exact=True).wait_for()
    for language in ["go","java","python","rust","typescript","http"]:
     print("checking language: "+language,flush=True)
     page.get_by_label("语言",exact=True).select_option(language);page.wait_for_function("() => document.querySelector('.integration-v2 textarea')?.value.length>10")
     # Wait for the language response instead of reading stale previous content.
     page.wait_for_timeout(150);assert app_id in page.locator(".integration-v2").inner_text()
    page.get_by_label("语言",exact=True).select_option("python");page.wait_for_timeout(200)
    with page.expect_download() as download:page.get_by_role("button",name="保存接入示例文件",exact=True).click()
    archive=folder/"integration.zip";download.value.save_as(archive)
    with zipfile.ZipFile(archive) as kit:assert {"client.py","requirements.txt","gui.py","openapi.json","public.pem"}<=set(kit.namelist());assert app_id in kit.read("client.py").decode();assert "AppSecret:" not in kit.read("client.py").decode()
    record("六种语言 / HTTP 接入视图与完整示例 ZIP 可从页面下载");page.get_by_role("button",name="本地验签",exact=True).click();screenshot("v2-integration-light-desktop.png")
    page.get_by_role("button",name="应用与策略",exact=True).click();page.get_by_label("启用此应用的在线升级服务",exact=True).check();page.get_by_role("button",name="保存应用策略",exact=True).click();expect(page.get_by_role("status")).to_contain_text("应用策略已保存");record("在页面启用可选升级服务并保持授权使用策略独立")
    page.get_by_label("公告内容",exact=True).fill("站点未授权，联系QQ531023977");page.get_by_role("button",name="保存拦截公告",exact=True).click();expect(page.get_by_role("status")).to_contain_text("应用策略已保存")
    with page.expect_popup() as opened_preview:page.get_by_role("link",name="预览拦截页",exact=True).click()
    preview=opened_preview.value;preview.get_by_role("heading",name="授权安全拦截",exact=True).wait_for();expect(preview.locator(".announcement-text")).to_contain_text("QQ531023977");preview.screenshot(path=str(args.artifacts/"v2-block-light-desktop.png"),full_page=True,animations="disabled");report["screenshots"].append("v2-block-light-desktop.png");preview.emulate_media(color_scheme="dark");preview.set_viewport_size({"width":390,"height":844});assert preview.evaluate("document.documentElement.scrollWidth<=innerWidth+1");preview.screenshot(path=str(args.artifacts/"v2-block-dark-mobile.png"),animations="disabled");report["screenshots"].append("v2-block-dark-mobile.png");preview.close();record("管理员页面配置拦截公告，预览保留设计并适配浅色电脑和深色手机")
    page.get_by_role("button",name="在线升级",exact=True).click();page.get_by_label("版本号",exact=True).fill("2.0.0");package=folder/"product.zip"
    with zipfile.ZipFile(package,"w") as archive:archive.writestr("index.html","<h1>Ebox 2.0.0</h1>")
    page.get_by_label("升级包（最多 256 MiB）",exact=True).set_input_files(package);page.get_by_role("button",name="上传为草稿",exact=True).click();expect(page.get_by_role("cell",name="草稿",exact=True)).to_be_visible();page.get_by_role("button",name="发布",exact=True).click();page.get_by_label("操作原因",exact=True).fill("已完成客户环境升级验收");page.get_by_role("button",name="确认",exact=True).click();expect(page.get_by_role("cell",name="已发布",exact=True)).to_be_visible();record("页面上传升级包并发布，版本与文件 SHA-256 可查");screenshot("v2-updates-light-desktop.png")
    page.get_by_role("button",name="审计与诊断",exact=True).click();page.get_by_role("button",name="验证审计完整性",exact=True).click();expect(page.get_by_role("status")).to_contain_text("审计链验证通过");record("按应用查询操作审计并验证追加事件链完整性");screenshot("v2-audit-light-desktop.png")
    page.get_by_role("button",name="外观主题",exact=True).click();page.get_by_role("menuitemradio",name="深色",exact=True).click();screenshot("v2-audit-dark-desktop.png");page.set_viewport_size({"width":390,"height":844});page.get_by_role("button",name="接入文档",exact=True).click();page.get_by_role("heading",name="产品接入文档",exact=True).wait_for();screenshot("v2-integration-dark-mobile.png");assert page.evaluate("document.documentElement.scrollWidth<=innerWidth+1");record("深色主题与手机布局无页面横向溢出")
    # Operate customer and agent portals with real accounts and the same app.
    customer=http.api(base,"/admin/customers",{"username":"v2-customer","password":password,"display_name":"Ebox 客户"},token)
    opened=http.api(base,"/admin/agent-accounts",{"username":"v2-seller","password":password,"name":"Ebox 渠道代理","initial_balance":5000,"reason":"人工确认渠道入账"},token)
    plan=http.api(base,prefix+"/plans",token=token)[0]
    page.set_viewport_size({"width":1440,"height":1050});page.reload();page.get_by_role("heading",name="应用授权中心",exact=True,level=2).wait_for();page.get_by_role("button",name="代理权限",exact=True).click();page.get_by_label("授权代理",exact=True).select_option(opened["agent"]["id"]);page.get_by_label("可销售套餐",exact=True).select_option(plan["id"]);page.get_by_role("button",name="保存代理权限",exact=True).click();expect(page.get_by_role("status")).to_contain_text("代理应用与套餐权限已保存");record("管理员通过页面给现有代理授予指定应用与套餐权限")
    # Run the exact Python project downloaded from the page with customer config.
    project=folder/"generated-product";project.mkdir()
    with zipfile.ZipFile(folder/"integration.zip") as z:z.extractall(project)
    dependencies=folder/"python-dependencies";subprocess.run([sys.executable,"-m","pip","install","--quiet","--target",str(dependencies),"-r",str(project/"requirements.txt")],check=True,capture_output=True,text=True)
    product_env=os.environ.copy();product_env.update(PYTHONPATH=str(dependencies),YN_LICENSE_URL=base,YN_BINDING_TYPE="domain",YN_BINDING_VALUE="example.com",YN_FORM="web",YN_CARD=codes[0],YN_STATE_DIR=str(folder/"customer-installation"))
    for command in ["activate","verify","local"]:
     output=subprocess.run([sys.executable,str(project/"client.py"),command],cwd=project,env=product_env,capture_output=True,text=True,check=True);assert json.loads(output.stdout)["valid"]
    record("从页面下载的实际产品示例完成卡密激活、在线刷新和本地验签")
    alias_env={**product_env,"YN_BINDING_VALUE":"www.example.com"};alias=subprocess.run([sys.executable,str(project/"client.py"),"verify"],cwd=project,env=alias_env,capture_output=True,text=True,check=True);assert json.loads(alias.stdout)["valid"];subprocess.run([sys.executable,str(project/"client.py"),"verify"],cwd=project,env=product_env,capture_output=True,text=True,check=True);assert len(http.api(base,prefix+"/activations?license_id="+target_license,token=token))==1;record("页面预录的根域名和 www 在同一实际安装验证，不额外消耗节点")
    product_env.update(YN_EVIDENCE_SHA256=hashlib.sha256((project/"client.py").read_bytes()).hexdigest(),YN_INTEGRITY_SUMMARY="真实产品签名完整性上报验收");reported=subprocess.run([sys.executable,str(project/"client.py"),"report"],cwd=project,env=product_env,capture_output=True,text=True,check=True);assert json.loads(reported.stdout)["status"]=="pending";record("实际下载客户端使用安装签名上报完整性证据，仅进入待核实审计")

    user_context=browser.new_context(viewport={"width":1440,"height":1050},locale="zh-CN");user=user_context.new_page();user.on("pageerror",lambda e:errors.append(str(e)));user.goto(base+"/user-console/");user.get_by_label("账号",exact=True).fill("v2-customer");user.get_by_label("密码",exact=True).fill(password);user.get_by_role("button",name="登录用户中心",exact=True).click();user.get_by_role("heading",name="关联购买的授权",exact=True).wait_for();user.get_by_label("选择应用",exact=True).select_option(app_id);user.get_by_label("购买的激活卡密",exact=True).fill(codes[0]);user.get_by_role("button",name="关联到我的账号",exact=True).click();expect(user.get_by_role("heading",name="Ebox · 通用产品授权",exact=True)).to_be_visible();assert user.get_by_role("button",name="更换部署绑定",exact=True).is_disabled();record("客户登录后凭卡密关联授权、查看应用期限，默认禁止自助换绑")
    user.evaluate("window.scrollTo({top:0,behavior:'instant'})");user.screenshot(path=str(args.artifacts/"v2-customer-portal.png"),full_page=True,animations="disabled");report["screenshots"].append("v2-customer-portal.png")
    seller_context=browser.new_context(viewport={"width":1440,"height":1050},locale="zh-CN");seller=seller_context.new_page();page=seller;seller.on("pageerror",lambda e:errors.append(str(e)));seller.goto(base+"/agent-console/#generate");seller.get_by_label("登录名",exact=True).fill("v2-seller");seller.get_by_label("密码",exact=True).fill(password);seller.get_by_role("button",name="进入工作台",exact=True).click();seller.get_by_role("heading",name="生成应用卡密",exact=True).wait_for();seller.get_by_label("选择代理应用",exact=True).select_option(app_id);seller.get_by_label("可售套餐",exact=True).select_option(plan["id"]);seller.get_by_label("发卡数量",exact=True).fill("2");seller.get_by_label("客户参考号",exact=True).fill("代理客户订单");seller.get_by_role("button",name="确认成本并生成卡密",exact=True).click();seller.get_by_label("代理本次卡密",exact=True).wait_for();assert len(seller.get_by_label("代理本次卡密",exact=True).input_value().splitlines())==2
    with seller.expect_download() as download:seller.get_by_role("button",name="导出本次卡密 CSV",exact=True).click()
    download.value.save_as(folder/"private-agent-cards.csv");seller.get_by_role("button",name="已保存，隐藏卡密",exact=True).click();seller.evaluate("window.scrollTo({top:0,behavior:'instant'})");seller.screenshot(path=str(args.artifacts/"v2-agent-issuance.png"),full_page=True,animations="disabled");report["screenshots"].append("v2-agent-issuance.png");record("代理通过应用与套餐选择发卡并一次性导出，继续使用同一用户账号")
    public_context=browser.new_context(viewport={"width":1440,"height":1050},locale="zh-CN");public=public_context.new_page();page=public;public.on("pageerror",lambda e:errors.append(str(e)));public.goto(base+"/");public.get_by_label("查询应用",exact=True).select_option(app_id);public.get_by_label("授权查询",exact=True).fill("example.com");public.get_by_role("button",name="查询",exact=True).click();expect(public.locator(".query-result")).to_contain_text("已授权");assert public.locator(".query-result").get_by_text("Ebox 客户",exact=True).count()==0;record("免登录按应用查询域名授权，返回脱敏状态且不签发运行凭证")
    public.screenshot(path=str(args.artifacts/"v2-public-license-verification.png"),full_page=True,animations="disabled");report["screenshots"].append("v2-public-license-verification.png");public.get_by_role("button",name="代理验真",exact=True).click();public.get_by_label("公开代理编号",exact=True).fill(opened["agent"]["agent_no"]);public.get_by_role("button",name="核验代理身份",exact=True).click();expect(public.locator(".agent-result")).to_contain_text("Ebox · 通用产品授权");record("免登录代理验真展示新模型实际授予的可售应用")
    # Platform ownership, grouping and archive remain operable from pages.
    first_developer=http.api(base,"/admin/users",{"username":"v2-developer-a","password":password,"role":"developer","display_name":"开发者甲"},token)
    http.api(base,"/admin/users",{"username":"v2-developer-b","password":password,"role":"developer","display_name":"开发者乙"},token)
    administrator=context.pages[0];page=administrator;administrator.goto(base+"/admin/#applications");administrator.reload();administrator.get_by_role("button",name="应用与策略",exact=True).click();administrator.get_by_label("应用分组",exact=True).fill("商业应用");administrator.get_by_role("button",name="保存应用策略",exact=True).click();expect(administrator.get_by_role("status")).to_contain_text("应用策略已保存")
    administrator.get_by_label("分配开发者",exact=True).select_option(first_developer["id"]);administrator.get_by_label("分配原因",exact=True).fill("产品归属开发者甲，验证账号隔离");administrator.get_by_role("button",name="保存开发者归属",exact=True).click();expect(administrator.get_by_role("status")).to_contain_text("开发者归属已保存");record("管理员在页面分配开发者与应用分组")
    developer_a=http.api(base,"/developer/login",{"username":"v2-developer-a","password":password})["access_token"];developer_b=http.api(base,"/developer/login",{"username":"v2-developer-b","password":password})["access_token"]
    assert len(http.api(base,"/v2/console/applications",token=developer_a))==1;assert http.api(base,"/v2/console/applications",token=developer_b)==[];assert http.http(base,prefix+"/licenses",token=developer_b,expected=404)["error"]["code"]=="NOT_FOUND";record("独立开发者仅能访问所属应用，另一账号直接请求对象仍被拒绝")
    administrator.get_by_role("button",name="代理权限",exact=True).click();administrator.get_by_label("授权代理",exact=True).select_option(opened["agent"]["id"]);administrator.get_by_role("button",name="按当前分组保存代理权限",exact=True).click();expect(administrator.get_by_role("status")).to_contain_text("分组已处理 1 个套餐");record("页面按应用分组授予当前上架套餐并报告处理结果")
    administrator.get_by_role("button",name="许可证与安装",exact=True).click();license_row=administrator.locator(".application-table").first.get_by_role("row").filter(has_text=target_license);license_row.get_by_role("button",name="安全处置",exact=True).click();decision=administrator.get_by_role("dialog",name="人工核实安全处置",exact=True);decision.get_by_label("处置措施",exact=True).select_option("deny_updates");decision.get_by_label("操作原因",exact=True).fill("人工核实，保留使用但暂时限制更新");decision.get_by_role("button",name="确认人工核实安全处置",exact=True).click();expect(license_row).to_contain_text("更新已被人工限制")
    user.reload();user.get_by_role("button",name="提交授权申诉",exact=True).click();appeal=user.get_by_role("dialog",name="授权申诉",exact=True);appeal.get_by_label("操作原因",exact=True).fill("已购买正版，请核对我的订单并恢复更新服务");appeal.get_by_role("button",name="确认授权申诉",exact=True).click();expect(user.get_by_role("status")).to_contain_text("申诉已提交")
    license_row.get_by_role("button",name="安全处置",exact=True).click();decision=administrator.get_by_role("dialog",name="人工核实安全处置",exact=True);decision.get_by_label("处置措施",exact=True).select_option("allow_updates");decision.get_by_label("操作原因",exact=True).fill("客户申诉已核实，解除更新限制");decision.get_by_role("button",name="确认人工核实安全处置",exact=True).click();expect(license_row).not_to_contain_text("更新已被人工限制");record("页面完成人工只限更新、客户申诉和核实后解除限制的闭环")
    def navigate_admin(label,group):
     toggle=administrator.get_by_role("button",name=group,exact=True)
     if toggle.get_attribute("aria-expanded")=="false":toggle.click()
     administrator.get_by_role("link",name=label,exact=True).click()
    navigate_admin("等级与结算","代理");administrator.get_by_label("当前代理",exact=True).select_option(opened["agent"]["id"]);expect(administrator.get_by_role("row").filter(has_text="发卡扣款")).to_contain_text("-20.00");screenshot("v2-agent-finance-issued.png");record("管理员资金页显示 V2 应用发卡实际扣款，不丢失新核心流水")
    orders=http.api(base,prefix+"/orders",token=token);agent_order=next(o for o in orders if o["agent_id"]==opened["agent"]["id"])
    navigate_admin("应用授权中心","产品");administrator.get_by_role("button",name="销售与卡密",exact=True).click();administrator.get_by_role("row").filter(has_text=agent_order["id"]).get_by_role("button",name="确认退款",exact=True).click();refund=administrator.get_by_role("dialog",name="订单退款与权益收回",exact=True);refund.get_by_label("操作原因",exact=True).fill("页面验收：渠道订单退款，原成本只退平台余额一次");refund.get_by_role("button",name="确认订单退款与权益收回",exact=True).click();expect(administrator.get_by_role("row").filter(has_text=agent_order["id"])).to_contain_text("已退款")
    navigate_admin("等级与结算","代理");administrator.get_by_label("当前代理",exact=True).select_option(opened["agent"]["id"]);expect(administrator.get_by_role("row").filter(has_text="退款退回")).to_contain_text("20.00");screenshot("v2-agent-finance-refunded.png");record("页面确认代理订单退款后，许可证收回且原成交成本退回资金流水")
    navigate_admin("历史授权档案","产品");administrator.get_by_role("heading",name="历史授权档案",exact=True,level=2).wait_for()
    with administrator.expect_download() as download:administrator.get_by_role("button",name="导出历史盘点报告",exact=True).click()
    download.value.save_as(folder/"archive-report.json");assert json.loads((folder/"archive-report.json").read_text())["automatic_conversion"] is False;assert download.value.suggested_filename=="legacy-authorization-inventory.json";record("历史授权只读盘点报告可通过页面导出，无旧卡密原文恢复入口")
    assert not errors,"Browser errors: "+json.dumps(errors);report.update(result="passed",app_id=app_id,browser=browser.version,case_count=len(report["cases"]));browser.close()
  except Exception as e:
   report.update(result="failed",error=type(e).__name__+": "+str(e));raise
  finally:
   if server is not None:server.terminate();server.wait(timeout=10)
   (args.artifacts/"report.json").write_text(json.dumps(report,ensure_ascii=False,indent=2))
if __name__=="__main__":main()
