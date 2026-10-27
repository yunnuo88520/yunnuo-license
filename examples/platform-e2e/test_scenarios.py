#!/usr/bin/env python3
"""Exercise standard-product delivery against the real authorization backend."""
import argparse
import datetime
import hashlib
import gzip
import importlib.util
import io
import json
import os
import pathlib
import platform
import secrets
import shutil
import sqlite3
import subprocess
import tempfile
import time
import zipfile

REPO = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("scenario_http", REPO / "examples/update-demo/test_e2e.py")
http = importlib.util.module_from_spec(spec)
spec.loader.exec_module(http)

class Lab:
    def __init__(self, args, folder):
        self.args, self.folder = args, folder
        self.processes, self.handles = [], []
        self.report = {"result": "running", "cases": [], "source_commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REPO, text=True).strip(), "source_dirty": bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=REPO)), "started_at": datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat()}
        if args.base_url:
            self.base=args.base_url.rstrip("/")
            self.password=os.environ["YN_TEST_ADMIN_PASSWORD"]
            username="admin"
            self.db=None
        else:
            self.port = http.free_port()
            self.base = f"http://127.0.0.1:{self.port}"
            self.db = folder / "license.sqlite"
            self.password = secrets.token_hex(24)
            username="scenario-root"
            env = os.environ.copy()
            env.update(YN_ADDR=f"127.0.0.1:{self.port}", YN_DB_DRIVER="sqlite", YN_DB=f"file:{self.db}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", YN_DB_CONFIG_FILE=str(folder / "database.json"), YN_RELEASE_DIR=str(folder / "releases"), YN_SDK_DIR=str(REPO / "sdk"), YN_CLIENT_WRITE_RATE_LIMIT="2000", YN_CLIENT_READ_RATE_LIMIT="10000")
            self.server = self.start([str(args.server.resolve())], REPO / "backend", env, "license-server.log")
            http.wait_ready(self.base, "/healthz", self.server)
            http.initialize(self.base, username, self.password, env["YN_DB"])
        self.report["database"]=args.database if args.base_url else "sqlite"
        self.root = http.api(self.base, "/admin/login", {"username": username, "password": self.password})["access_token"]
        self.product = http.api(self.base, "/admin/products", {"name": "Ebox 通用产品场景", "bind_mode": "device"}, self.root)
        self.terms = http.api(self.base, "/admin/products/" + self.product["id"] + "/authorization-terms", token=self.root)
        self.public_config = {"base_url": self.base, "app_key": self.product["id"], "product_id": self.product["id"], "product_code": self.product["code"], "public_keys": {"1": self.product["public_key_pem"]}, "current_version": "1.0.0", "bind_value": ""}
        self.config = folder / "product.json"
        self.config.write_text(json.dumps(self.public_config))
        self.client = args.client.resolve()

    def start(self, command, cwd, env, name):
        handle = (self.args.artifacts / name).open("w")
        self.handles.append(handle)
        process = subprocess.Popen(command, cwd=cwd, env=env, stdout=handle, stderr=handle)
        self.processes.append(process)
        return process

    def record(self, name, **details):
        self.report["cases"].append({"name": name, "passed": True, **details})
        (self.args.artifacts / "report.json").write_text(json.dumps(self.report, ensure_ascii=False, indent=2))
        print(name, flush=True)

    def package(self, name, mode, **overrides):
        terms = {**self.terms, "version": 2, "card_purpose": "license", "forms": ["service"], "license_mode": "perpetual", "update_duration_days": 30, "bind_mode": mode, "require_instance": True, "max_deployments": 3, "max_bindings": 1, "max_devices": 0, "max_concurrent_seats": 0, "unbind_limit": 3, "expire_grace_days": 0, "features": ["articles"], **overrides}
        return http.api(self.base, "/admin/packages", {"product_id": self.product["id"], "name": name, "duration_days": 30, "is_permanent": False, "base_cost": 0, "authorization_terms": terms}, self.root)

    def card(self, package):
        return http.api(self.base, "/admin/card-batches", {"product_id": self.product["id"], "package_id": package["id"], "quantity": 1, "name": package["name"]}, self.root)["codes"][0]

    def cli(self, state, command, *arguments, host="", ip="", session="", expected=True, config=None, client=None):
        argv = [str(client or self.client), "--config", str(config or self.config), "--data", str(state)]
        if host: argv += ["--host", host]
        if ip: argv += ["--ip", ip]
        if session: argv += ["--session", session]
        result = subprocess.run([*argv, command, *map(str, arguments)], capture_output=True, text=True, timeout=180)
        assert (result.returncode == 0) == expected, command + " returned the wrong status: " + result.stdout[-500:] + result.stderr[-500:]
        return json.loads(result.stdout)

    def node(self, state, command, value="", registration="", host="", ip="", expected=True):
        result = subprocess.run(["node", str(REPO / "examples/universal-client/node-driver.mjs"), str(self.config), str(state), command, value, registration, host, ip], capture_output=True, text=True, timeout=60)
        assert (result.returncode == 0) == expected, command + " Node result: " + result.stdout[-500:] + result.stderr[-500:]
        return json.loads(result.stdout)

    def publish(self, version, data, target=None):
        import uuid
        boundary = uuid.uuid4().hex
        parts = []
        for name, value in [("version", version), ("notes", "真实多形态产品交付验收"), *([("target", json.dumps(target))] if target else [])]:
            parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode())
        filename = "image.tar.gz" if target and target.get("kind") == "container_archive" else "product.zip"
        parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="package"; filename="{filename}"\r\nContent-Type: application/zip\r\n\r\n'.encode() + data + f'\r\n--{boundary}--\r\n'.encode())
        prefix = "/admin/products/" + self.product["id"]
        http.api(self.base, prefix + "/updates", {"enabled": True}, self.root)
        release = http.api(self.base, prefix + "/releases", b"".join(parts), self.root, "multipart/form-data; boundary=" + boundary)
        http.api(self.base, prefix + "/releases/" + release["id"] + "/publish", {}, self.root)
        return release

    def close(self):
        for process in reversed(self.processes):
            if process.poll() is None:
                process.terminate()
                try: process.wait(timeout=10)
                except subprocess.TimeoutExpired: process.kill(); process.wait()
        for handle in self.handles: handle.close()


def run_core(lab):
    shapes = [
        ("Web 精确域名", "domain", "web.example.com", {"host": "web.example.com"}),
        ("Web 泛域名", "domain_wildcard", "*.example.com", {"host": "tenant.example.com"}),
        ("IPv4 服务", "ip", "192.0.2.40", {}),
        ("IPv6 服务", "ip", "2001:0DB8:0000:0000:0000:0000:0000:0040", {}),
        ("IPv4 网段", "ip_cidr", "192.0.2.0/24", {"ip": "192.0.2.40"}),
        ("IPv6 网段", "ip_cidr", "2001:db8::/48", {"ip": "2001:0DB8:0000:0001:0000:0000:0000:0040"}),
        ("GUI 设备", "device", "", {}),
        ("账号授权", "account", "buyer@example.com", {}),
        ("SaaS 租户", "tenant", "OrgTenant_A1", {}),
        ("Docker 安装实例", "instance", "", {}),
        ("Kubernetes 集群", "cluster", "cluster-ebox-a", {}),
        ("组合绑定", "composite", json.dumps({"domain_wildcard": "*.example.com", "device": "server-A"}), {"host": "actual.example.com"}),
        ("无绑定", "none", "", {}),
    ]
    for index, (name, mode, registration, context) in enumerate(shapes):
        overrides = {"binding_kinds": ["domain_wildcard", "device"]} if mode == "composite" else {}
        package = lab.package(name, mode, **overrides)
        card, state = lab.card(package), lab.folder / f"shape-{index}"
        activated = lab.cli(state, "activate", card, registration)
        verified = lab.cli(state, "verify", **context)
        assert activated["can_use"] and verified["can_use"] and activated["license_no"] == verified["license_no"]
        assert verified["authorization_terms"]["bind_mode"] == mode
        if True:
            node_state = lab.folder / f"node-shape-{index}"
            node_activated = lab.node(node_state, "activate", lab.card(package), registration)
            node_verified = lab.node(node_state, "verify", host=context.get("host", ""), ip=context.get("ip", ""))
            assert node_activated["can_use"] and node_verified["can_use"]
        lab.record(name + "：同一标准程序按卡密识别规则并通过实际在线核验")

    package = lab.package("多客户 Web 交付", "domain_wildcard", forms=["web"])
    card = lab.card(package)
    a, b = lab.folder / "customer-a", lab.folder / "customer-b"
    first = lab.cli(a, "activate", card, "*.customer.example.com")
    second_card = lab.card(package)
    second = lab.cli(b, "activate", second_card, "*.customer.example.com")
    assert first["license_no"] != second["license_no"]
    assert lab.cli(a, "verify", host="a.customer.example.com")["can_use"]
    assert lab.cli(b, "verify", host="b.customer.example.com")["can_use"]
    assert lab.cli(a, "verify", host="customer.example.com", expected=False)["code"] == "BINDING_MISMATCH"
    assert lab.cli(a, "verify", host="a.b.customer.example.com", expected=False)["code"] == "BINDING_MISMATCH"
    lab.record("两个客户共用同一个二进制和公开配置，分别激活；默认泛域名拒绝根域名和深层子域名", binary_sha256=hashlib.sha256(lab.client.read_bytes()).hexdigest())

    port = http.free_port()
    env = os.environ.copy();env["DEMO_PORT"] = str(port)
    process = lab.start([str(lab.client), "--config", str(lab.config), "--data", str(a), "serve"], REPO, env, "web-product.log")
    site = f"http://127.0.0.1:{port}"
    http.wait_ready(site, "/healthz", process)
    assert http.http(site, "/protected", extra_headers={"Host": "a.customer.example.com"})["can_use"]
    http.http(site, "/protected", expected=403, extra_headers={"Host": "unlicensed.example.net", "X-Forwarded-Host": "a.customer.example.com"})
    lab.record("真实 Web 入口核对实际 Host，未授权域名及未受信任的代理头不能绕过")

    ticket = lab.cli(a, "claim", host="a.customer.example.com")["claim_token"]
    for username in ["customer-one", "customer-two"]:
        http.api(lab.base, "/admin/customers", {"username": username, "password": lab.password}, lab.root)
    user = http.api(lab.base, "/user/login", {"username": "customer-one", "password": lab.password})["access_token"]
    other = http.api(lab.base, "/user/login", {"username": "customer-two", "password": lab.password})["access_token"]
    claimed = http.api(lab.base, "/user/licenses/claim", {"claim_token": ticket}, user)
    assert claimed["license_no"] == first["license_no"]
    http.http(lab.base, "/user/licenses/claim", {"claim_token": ticket}, other, expected=403)
    http.http(lab.base,"/user/licenses/"+first["license_no"]+"/installations",token=other,expected=404)
    http.http(lab.base,"/admin/products/"+lab.product["id"]+"/authorization-terms",token=user,expected=401)
    lab.record("产品内激活后通过一次性凭证关联用户中心，其他账号不能抢认领或查看实例")
    lab.report["browser_fixture"] = {"product_id": lab.product["id"]}
    return {"user": user, "state": a, "card": card, "site": site, "host": "a.customer.example.com", "license": first, "other_license": second}



def build_client(lab, destination, version, faulty=False):
    flags = "-X main.bundledVersion=" + version + (" -X main.healthFailure=true" if faulty else "")
    result = subprocess.run(["go", "build", "-buildvcs=false", "-ldflags", flags, "-o", str(destination), "."], cwd=REPO / "examples/universal-client", env={**os.environ,"CGO_ENABLED":"0"}, capture_output=True, text=True, timeout=180)
    assert result.returncode == 0, "native product build failed"
    destination.chmod(0o700)
    return destination

def installer_package(binary, version):
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as package:
        package.writestr("product-client.exe" if os.name == "nt" else "product-client", binary.read_bytes())
        package.writestr("release.json", json.dumps({"version": version, "healthy": True}))
    return output.getvalue()

def update_deadline(lab, license_no, expired):
    value = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=-1 if expired else 30)).isoformat().replace("+00:00", "Z")
    with sqlite3.connect(lab.db) as database:
        database.execute("UPDATE license_entitlements SET updates_expired_at=? WHERE license_id=(SELECT id FROM licenses WHERE license_no=?)", (value, license_no))

def run_gui(lab):
    os.environ["XDG_CACHE_HOME"]=str(lab.folder/"gui-cache")
    if os.name != "nt" and not os.environ.get("DISPLAY"):
        display = ":" + str(90 + secrets.randbelow(1000))
        process = lab.start([lab.args.xvfb, display, "-screen", "0", "1100x900x24", "-nolisten", "tcp", "-ac"], REPO, os.environ.copy(), "gui-display.log")
        os.environ["DISPLAY"] = display
        for _ in range(100):
            if process.poll() is not None:
                raise RuntimeError("Desktop test display failed")
            if pathlib.Path("/tmp/.X11-unix/X" + display[1:]).exists():
                break
            time.sleep(0.05)
    spec = importlib.util.spec_from_file_location("product_gui", REPO / "examples/universal-client/gui.py")
    gui = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(gui)
    import tkinter as tk
    from PIL import ImageGrab
    install = lab.folder / "gui-install"
    install.mkdir()
    binary = install / ("product-client.exe" if os.name == "nt" else "product-client")
    shutil.copy2(lab.client, binary)
    target = {"os": {"Darwin": "darwin", "Windows": "windows"}.get(platform.system(), "linux"), "arch": "arm64" if platform.machine().lower() in ["arm64", "aarch64"] else "amd64", "runtime": "native", "channel": "stable", "kind": "installer"}
    config = lab.folder / "gui-product.json"
    config.write_text(json.dumps({**lab.public_config, "target": target}))
    state = lab.folder / "gui-data"
    package = lab.package("GUI 设备永久使用", "device", forms=["gui"], max_devices=1, max_deployments=1, features=["articles", "export"])
    card = lab.card(package)
    root = tk.Tk()
    window = gui.ProductWindow(root, binary, config, state)
    def settle():
        end = time.monotonic() + 120
        while window.busy and time.monotonic() < end:
            root.update()
            time.sleep(0.03)
        root.update()
        assert not window.busy, "native GUI operation did not finish"
    def screenshot(name):
        root.update()
        x, y = root.winfo_rootx(), root.winfo_rooty()
        ImageGrab.grab(bbox=(x, y, x + root.winfo_width(), y + root.winfo_height()), xdisplay=os.environ.get("DISPLAY")).save(lab.args.artifacts / name)
    try:
        window.card.insert(0, card)
        window.activate_button.invoke()
        settle()
        assert window.last_result.get("can_use"), window.last_result
        original = json.loads((state / "license.json").read_text())
        original_identity = hashlib.sha256((state / "instance.json").read_bytes()).hexdigest()
        license_no = original["license"]["license_no"]
        screenshot("gui-activated.png")
        (state / "customer-note.txt").write_text("客户配置必须保留")
        v2 = build_client(lab, lab.folder / "gui-v2", "2.0.0")
        release = lab.publish("2.0.0", installer_package(v2, "2.0.0"), target)
        assert release["target"]["kind"] == "installer"
        window.update_button.invoke()
        settle()
        assert window.last_result.get("updated") and window.last_result["version"] == "2.0.0", window.last_result
        assert hashlib.sha256(binary.read_bytes()).hexdigest() == hashlib.sha256(v2.read_bytes()).hexdigest()
        assert (state / "customer-note.txt").read_text() == "客户配置必须保留"
        assert hashlib.sha256((state / "instance.json").read_bytes()).hexdigest() == original_identity
        screenshot("gui-updated.png")
        lab.record("真实桌面窗口激活，并实际替换 v2 核心二进制；客户配置和安装身份保留", screenshots=["gui-activated.png", "gui-updated.png"])

        faulty = build_client(lab, lab.folder / "gui-faulty", "2.1.0", True)
        bad_release = lab.publish("2.1.0", installer_package(faulty, "2.1.0"), target)
        window.update_button.invoke()
        settle()
        assert "error" in window.last_result and "恢复原程序" in window.last_result["error"], window.last_result
        assert hashlib.sha256(binary.read_bytes()).hexdigest() == hashlib.sha256(v2.read_bytes()).hexdigest()
        screenshot("gui-rollback.png")
        http.api(lab.base, "/admin/products/" + lab.product["id"] + "/releases/" + bad_release["id"] + "/withdraw", {}, lab.root)
        lab.record("桌面新版程序真实健康检查失败后回滚到 v2，客户数据保持完整", screenshots=["gui-rollback.png"])

        v3 = build_client(lab, lab.folder / "gui-v3", "3.0.0")
        lab.publish("3.0.0", installer_package(v3, "3.0.0"), target)
        original_call = window.call
        def expire_before_install(command, *values, **kwargs):
            if command == "authorize":
                update_deadline(lab, license_no, True)
            return original_call(command, *values, **kwargs)
        window.call = expire_before_install
        window.update_button.invoke()
        settle()
        assert "error" in window.last_result and "更新服务已到期" in window.last_result["error"], window.last_result
        assert hashlib.sha256(binary.read_bytes()).hexdigest() == hashlib.sha256(v2.read_bytes()).hexdigest()
        screenshot("gui-expired-before-install.png")
        window.call = original_call
        window.verify_button.invoke()
        settle()
        assert window.last_result.get("can_use") and not window.last_result.get("can_update")
        lab.record("桌面升级下载完成后、安装前更新资格到期，原二进制未被替换，永久旧版仍可用", screenshots=["gui-expired-before-install.png"])
        machine=window.call("machine-code")["device_id"]
        offline=http.api(lab.base,"/admin/offline-licenses",{"product_id":lab.product["id"],"machine_code":machine,"label":"GUI 离线安装","duration_days":7},lab.root)
        file=lab.folder/"gui-offline.key";file.write_bytes(http.http(lab.base,"/admin/offline-licenses/"+offline["id"]+"/download",token=lab.root))
        window.call("import-offline",str(file));window.verify_button.invoke();settle()
        assert window.last_result["can_use"] and window.last_result["offline"] and not window.last_result["can_update"]
        screenshot("gui-offline-imported.png")
        lab.record("桌面离线文件导入后再次点击验证仍可用，并明确关闭在线更新资格",screenshots=["gui-offline-imported.png"])
    finally:
        window.close()



def run_docker(lab):
    spec = importlib.util.spec_from_file_location("product_container", REPO / "examples/universal-client/container_update.py")
    updater = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(updater)
    from types import SimpleNamespace
    repository = "yunnuo-scenario-" + secrets.token_hex(5)
    volume, container = repository + "-data", repository + "-site"
    images = []
    config = lab.folder / "container-product.json"
    target = {"os": "linux", "arch": "arm64" if platform.machine().lower() in ["arm64", "aarch64"] else "amd64", "runtime": "native", "channel": "stable", "kind": "container_archive"}
    config.write_text(json.dumps({**lab.public_config, "target": target, "image_repository": repository}))
    config.chmod(0o644)
    options = SimpleNamespace(container=container, volume=volume, config=config, staging=lab.folder / "container-staging", port=http.free_port(), network="host")
    def build(version, faulty=False):
        folder = lab.folder / ("docker-build-" + version)
        folder.mkdir()
        build_client(lab, folder / "product-client", version, faulty)
        shutil.copy2(REPO / "examples/universal-client/Dockerfile", folder / "Dockerfile")
        shutil.copy2("/etc/ssl/certs/ca-certificates.crt", folder / "ca-certificates.crt")
        (folder / "empty-data").mkdir()
        (folder / "empty-data/.keep").touch()
        image = repository + ":" + version
        updater.docker("build", "-t", image, folder)
        images.append(image)
        return image, json.loads(updater.docker("image", "inspect", image))[0]["Id"]
    def publish_image(version, image, digest, minimum, maximum):
        archive = lab.folder / ("image-" + version + ".tar.gz")
        process = subprocess.Popen(["docker", "save", image], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        with gzip.open(archive, "wb") as output:
            shutil.copyfileobj(process.stdout, output)
        error = process.stderr.read()
        assert process.wait(timeout=120) == 0, "Docker image export failed"
        release_target = {**target, "image_tag": image, "image_digest": digest, "min_version": minimum, "max_version": maximum}
        return lab.publish(version, archive.read_bytes(), release_target)
    try:
        updater.docker("volume", "create", volume)
        v1, _ = build("1.0.0")
        package = lab.package("Docker 持久安装身份", "instance", forms=["container", "web"], max_deployments=1)
        issued = updater.client(options, v1, "activate", lab.card(package))
        updater.client(options, v1, "note", "客户文章和配置需要跨镜像升级保留")
        updater.start_product(options, v1)
        first = updater.wait_product(options, "1.0.0")
        updater.docker("rm", "-f", container)
        updater.start_product(options, v1)
        rebuilt = updater.wait_product(options, "1.0.0")
        assert first["instance_id"] == rebuilt["instance_id"] and rebuilt["note"] == "客户文章和配置需要跨镜像升级保留"
        instances = http.api(lab.base, "/admin/products/" + lab.product["id"] + "/licenses/" + issued["license_no"] + "/installations", token=lab.root)
        assert len(instances) == 1 and instances[0]["status"] == "active"
        lab.record("真实 Docker 容器删除重建后，从持久卷恢复原安装身份，不增加部署数且客户数据保留")

        v2, digest2 = build("2.0.0")
        publish_image("2.0.0", v2, digest2, "1.0.0", "1.9.9")
        staged = updater.stage_update(options)
        assert staged["staged"]
        update_deadline(lab, issued["license_no"], True)
        try:
            updater.install_staged(options)
            raise AssertionError("expired update rights installed a container")
        except RuntimeError as error:
            assert "UPDATES_EXPIRED" in str(error), str(error)
        assert updater.wait_product(options, "1.0.0")["instance_id"] == first["instance_id"]
        lab.record("Docker 镜像已下载暂存后更新资格到期，安装前复验拒绝，原容器持续可用")

        update_deadline(lab, issued["license_no"], False)
        installed = updater.install_staged(options)
        current = updater.wait_product(options, "2.0.0")
        assert installed["updated"] and current["instance_id"] == first["instance_id"]
        assert current["note"] == first["note"]
        lab.record("Docker 实际导入已签名镜像归档，核对镜像 ID 并切换至 v2，客户数据和授权身份保留", image_id=digest2)

        broken, broken_digest = build("2.1.0", True)
        bad_release = publish_image("2.1.0", broken, broken_digest, "2.0.0", "2.9.9")
        updater.stage_update(options)
        try:
            updater.install_staged(options)
            raise AssertionError("unhealthy container was accepted")
        except RuntimeError as error:
            assert "restored" in str(error), str(error)
        restored = updater.wait_product(options, "2.0.0")
        assert restored["note"] == first["note"] and restored["instance_id"] == first["instance_id"]
        http.api(lab.base, "/admin/products/" + lab.product["id"] + "/releases/" + bad_release["id"] + "/withdraw", {}, lab.root)
        lab.record("Docker 新镜像实际健康检查失败后，恢复旧容器及同一数据卷")
    finally:
        subprocess.run(["docker", "rm", "-f", container], capture_output=True)
        subprocess.run(["docker", "volume", "rm", volume], capture_output=True)
        for image in reversed(images):
            subprocess.run(["docker", "image", "rm", image], capture_output=True)


def run_constraints(lab):
    from concurrent.futures import ThreadPoolExecutor
    plan=lab.package("集群与并发席位", "cluster", forms=["cluster","service"], max_deployments=2, max_devices=1, max_concurrent_seats=1, offline_grace_days=1)
    card=lab.card(plan)
    instance={"instance_id":"installation_cluster_0001","instance_secret":secrets.token_hex(32),"device_id":"host-cluster-1"}
    request={"app_key":lab.product["id"],"card_code":card,"bind_mode":"cluster","bind_value":"ebox-cluster-a",**instance}
    issued=http.api(lab.base,"/v1/licenses/activate",request)
    proof={key:value for key,value in request.items() if key!="card_code"};proof["license_no"]=issued["license_no"]
    http.http(lab.base,"/v1/licenses/activate",{**request,"instance_id":"installation_cluster_0002","device_id":"other-physical-device"},expected=403)
    second={**request,"instance_id":"installation_cluster_0002"}
    http.api(lab.base,"/v1/licenses/activate",second)
    def acquire(index):
        value={**proof,"instance_id":"installation_cluster_0001" if index%2==0 else "installation_cluster_0002","session_id":f"cluster_session_{index:016d}"}
        import urllib.error,urllib.request
        req=urllib.request.Request(lab.base+"/v1/licenses/seats/acquire",data=json.dumps(value).encode(),headers={"Content-Type":"application/json"})
        try: response=urllib.request.urlopen(req,timeout=40)
        except urllib.error.HTTPError as error: response=error
        with response: return response.status,json.load(response)
    with ThreadPoolExecutor(max_workers=12) as pool: outcomes=list(pool.map(acquire,range(12)))
    winners=[v[1]["data"] for v in outcomes if v[0]==200]
    assert len(winners)==1,outcomes
    assert all(status==200 or status==403 and value["error"]["code"]=="SEAT_LIMIT_EXCEEDED" for status,value in outcomes)
    seat=winners[0]
    proof["instance_id"]=seat["instance_id"]
    http.http(lab.base,"/v1/licenses/verify",{**proof,"seat_id":seat["id"],"session_id":"wrong_session_0001"},expected=403)
    assert http.api(lab.base,"/v1/licenses/verify",{**proof,"seat_id":seat["id"],"session_id":seat["session_id"]})["can_use"]
    http.api(lab.base,"/v1/licenses/seats/release",{**proof,"seat_id":seat["id"],"session_id":seat["session_id"]})
    borrowed=http.api(lab.base,"/v1/licenses/seats/acquire",{**proof,"session_id":"borrow_session_0001","borrow_days":1})
    assert borrowed["borrowed"]
    http.api(lab.base,"/v1/licenses/seats/release",{**proof,"seat_id":borrowed["id"],"session_id":"borrow_session_0001"})
    http.http(lab.base,"/v1/licenses/seats/acquire",{**proof,"session_id":"other_session_0001"},expected=403)
    http.api(lab.base,"/admin/products/"+lab.product["id"]+"/licenses/"+issued["license_no"]+"/installations/"+proof["instance_id"]+"/disable",{"reason":"验证借用仍占用"},lab.root)
    next_proof={**proof,"instance_id":"installation_cluster_0002" if proof["instance_id"].endswith("0001") else "installation_cluster_0001"}
    http.http(lab.base,"/v1/licenses/seats/acquire",{**next_proof,"session_id":"disabled_borrow_hold_0001"},expected=403)
    lab.record("两个实例间 12 路并发申请仅分配一个席位；跨进程重用、超设备额度被拒绝，借用席位在释放或停用实例后仍占用配额")
    tickets=[http.api(lab.base,"/v1/licenses/claim-ticket",next_proof)["claim_token"] for _ in range(2)]
    tokens=[http.api(lab.base,"/user/login",{"username":name,"password":lab.password})["access_token"] for name in ["customer-one","customer-two"]]
    def redeem(index):
        import urllib.error,urllib.request
        req=urllib.request.Request(lab.base+"/user/licenses/claim",data=json.dumps({"claim_token":tickets[index]}).encode(),headers={"Content-Type":"application/json","Authorization":"Bearer "+tokens[index]})
        try: response=urllib.request.urlopen(req,timeout=40)
        except urllib.error.HTTPError as error: response=error
        with response: return response.status
    with ThreadPoolExecutor(max_workers=2) as pool: claimed=list(pool.map(redeem,range(2)))
    assert sorted(claimed)==[200,403],claimed
    lab.record("两个不同用户持不同有效关联凭证并发认领同一授权，仅一个成功，既有归属不被覆盖")

    http.api(lab.base,"/admin/users",{"username":"scenario-developer","password":lab.password,"role":"developer"},lab.root)
    developer=http.api(lab.base,"/developer/login",{"username":"scenario-developer","password":lab.password})["access_token"]
    http.http(lab.base,"/developer/products/"+lab.product["id"]+"/authorization-terms",token=developer,expected=404)
    http.http(lab.base,"/developer/products/"+lab.product["id"]+"/licenses/"+issued["license_no"]+"/installations",token=developer,expected=404)
    http.http(lab.base,"/developer/products/"+lab.product["id"]+"/packages",token=developer,expected=404)
    lab.record("独立开发者不能读取其他产品条款、套餐或安装实例")


def run_offline(lab):
    plan=lab.package("CLI 席位离线借用","device",forms=["cli"],max_concurrent_seats=1,offline_grace_days=1)
    borrowed_state=lab.folder/"cli-borrowed"
    lab.cli(borrowed_state,"activate",lab.card(plan))
    assert lab.cli(borrowed_state,"borrow","1")["borrowed"]
    rights=lab.cli(borrowed_state,"verify-offline");assert rights["can_use"] and not rights["can_update"]
    state_file=borrowed_state/"license.json";original=state_file.read_bytes();changed=json.loads(original);changed["seat"]["session_id"]="other_session_000001";state_file.write_text(json.dumps(changed))
    lab.cli(borrowed_state,"verify-offline",expected=False)
    state_file.write_bytes(original)
    lab.record("真实 CLI 获取服务端签名的借用席位后离线验签可用；修改进程身份后拒绝，离线不授予更新")
    state = lab.folder / "offline-product"
    machine = lab.cli(state,"machine-code")["device_id"]
    license = http.api(lab.base,"/admin/offline-licenses",{"product_id":lab.product["id"],"machine_code":machine,"label":"工控设备离线交付","duration_days":7},lab.root)
    raw = http.http(lab.base,"/admin/offline-licenses/"+license["id"]+"/download",token=lab.root)
    file = lab.folder / "device.key"
    file.write_bytes(raw)
    assert lab.cli(state,"import-offline",file)["offline"]
    assert lab.cli(state,"verify-file")["can_use"]
    assert lab.cli(state,"verify-file")["can_update"] is False
    lab.cli(lab.folder/"wrong-device","offline-file",file,expected=False)
    parsed=json.loads(raw);token=parsed["token"];parsed["token"]=("A" if token[0]!="A" else "B")+token[1:];file.write_text(json.dumps(parsed))
    lab.cli(state,"import-offline",file,expected=False)
    assert lab.cli(state,"verify-file")["can_use"], "a bad import must not replace the existing valid file"
    lab.cli(state,"updates",expected=False)
    lab.record("完全离线文件按设备签发、导入后可重复验签；错误设备和篡改文件被拒绝，离线文件不能取得在线更新")


def run_browser(lab, fixture):
    from playwright.sync_api import sync_playwright
    with sync_playwright() as pw:
        browser=pw.chromium.launch(executable_path=None if lab.args.chromium=="bundled" else lab.args.chromium,headless=True,args=["--no-sandbox"])
        context=browser.new_context(viewport={"width":1440,"height":1080},locale="zh-CN",timezone_id="Asia/Shanghai",color_scheme="light")
        page=context.new_page();errors=[]
        page.on("pageerror",lambda e:errors.append(str(e)))
        try:
            page.goto(lab.base+"/admin#releases")
            page.get_by_label("账号",exact=True).fill("scenario-root");page.get_by_label("密码",exact=True).fill(lab.password)
            page.get_by_role("button",name="安全登录",exact=True).click()
            page.locator(".console").wait_for()
            page.goto(lab.base+"/admin#releases")
            page.get_by_label("选择产品",exact=False).select_option(lab.product["id"])
            panel=page.get_by_role("region",name="默认授权条款")
            panel.get_by_label("授权绑定策略",exact=False).select_option("domain_wildcard")
            panel.get_by_label("Web 网站",exact=True).check()
            panel.get_by_label("Docker／Compose",exact=True).check()
            panel.get_by_label("要求持久化安装实例凭证",exact=True).check()
            panel.get_by_label("安装实例额度",exact=True).fill("3")
            panel.get_by_label("使用授权模式",exact=False).select_option("perpetual")
            panel.get_by_label("更新服务天数（0 跟随卡密）",exact=True).fill("30")
            panel.get_by_label("功能标识（逗号分隔）",exact=True).fill("articles, export")
            with page.expect_response(lambda r:r.url.endswith("/authorization-terms") and r.request.method=="POST") as response:
                panel.get_by_role("button",name="保存默认授权条款",exact=True).click()
            assert response.value.status==200
            panel.get_by_text("默认条款已保存。库存卡和已有客户授权保持原条款。",exact=True).wait_for()
            saved=http.api(lab.base,"/admin/products/"+lab.product["id"]+"/authorization-terms",token=lab.root)
            assert saved["bind_mode"]=="domain_wildcard" and saved["require_instance"] and saved["max_deployments"]==3
            assert lab.cli(fixture["state"],"verify",host=fixture["host"])["authorization_terms"]["features"]==["articles"]
            panel.screenshot(path=str(lab.args.artifacts/"admin-default-terms.png"))
            page.get_by_role("tab",name="升级包",exact=True).click()
            releases=page.locator(".release-panel")
            releases.get_by_role("button",name="启用升级服务",exact=True).click()
            releases.get_by_role("button",name="关闭升级服务",exact=True).wait_for()
            releases.get_by_label("版本号",exact=True).fill("9.0.0")
            releases.get_by_label("按平台、渠道和升级路径发布",exact=True).check()
            releases.get_by_label("操作系统",exact=False).select_option("linux")
            releases.get_by_label("CPU 架构",exact=False).select_option("amd64")
            releases.get_by_label("运行时",exact=True).fill("node")
            releases.get_by_label("最低来源版本",exact=True).fill("1.0.0")
            releases.locator('input[type=file]').set_input_files({"name":"ebox-web.zip","mimeType":"application/zip","buffer":http.archive("2.0.0",2)})
            with page.expect_response(lambda r:r.url.endswith("/releases") and r.request.method=="POST") as upload:
                releases.get_by_role("button",name="上传为草稿",exact=True).click()
            assert upload.value.status==200
            item=releases.locator(".release-item").filter(has_text="v9.0.0")
            item.get_by_role("button",name="发布版本",exact=True).click()
            item.get_by_text("v9.0.0 · 已发布",exact=False).wait_for()
            assert "linux / amd64" in item.inner_text()
            assert releases.get_by_label("按平台、渠道和升级路径发布",exact=True).is_checked()
            page.screenshot(path=str(lab.args.artifacts/"admin-targeted-release.png"),full_page=True)
            page.goto(lab.base+"/admin#commerce")
            page.get_by_role("button",name="建立套餐",exact=True).click()
            dialog=page.get_by_role("dialog",name="建立套餐",exact=True)
            dialog.get_by_label("所属应用",exact=False).select_option(lab.product["id"])
            dialog.get_by_label("套餐名称",exact=True).fill("GUI 三设备专业版")
            dialog.get_by_label("自定义套餐授权条款",exact=True).check()
            dialog.get_by_label("授权绑定策略",exact=False).select_option("device")
            dialog.get_by_label("桌面 GUI",exact=True).check()
            dialog.get_by_label("设备额度（0 不额外限制）",exact=True).fill("3")
            dialog.get_by_label("安装实例额度",exact=True).fill("3")
            dialog.get_by_label("功能标识（逗号分隔）",exact=True).fill("articles, export, api")
            dialog.screenshot(path=str(lab.args.artifacts/"admin-package-terms.png"))
            with page.expect_response(lambda r:r.url.endswith("/packages") and r.request.method=="POST") as created:
                dialog.get_by_role("button",name="建立套餐",exact=True).click()
            assert created.value.status==200
            assert created.value.json()["data"]["authorization_terms"]["max_devices"]==3
            lab.record("真实管理页面保存默认条款、建立独立套餐、上传和发布目标平台版本；已售授权条款不随默认配置变更")
            user=context.new_page();user.on("pageerror",lambda e:errors.append(str(e)))
            user.goto(lab.base+"/user-console/")
            user.get_by_label("账号",exact=True).fill("customer-one");user.get_by_label("密码",exact=True).fill(lab.password)
            user.get_by_role("button",name="登录用户中心",exact=True).click()
            user.locator(".license-card").filter(has_text=fixture["license"]["license_no"]).get_by_role("button",name="查看安装实例",exact=True).click()
            user.get_by_text("最近核验",exact=False).first.wait_for()
            assert fixture["license"]["license_no"] in user.locator("body").inner_text()
            user.screenshot(path=str(lab.args.artifacts/"user-license-installations.png"),full_page=True)
            for width in [390,320]:
                user.set_viewport_size({"width":width,"height":844});user.emulate_media(color_scheme="dark")
                user.locator('html[data-theme="dark"]').wait_for(state="attached")
                assert user.evaluate("document.documentElement.scrollWidth<=innerWidth+1")
                user.screenshot(path=str(lab.args.artifacts/f"user-mobile-dark-{width}.png"),full_page=True)
            assert not errors,errors
            lab.record("用户中心展示已关联授权及安装实例，PC 浅色和手机深色实际浏览器验证通过")
        except Exception:
            page.screenshot(path=str(lab.args.artifacts/"browser-failure.png"),full_page=True)
            raise
        finally:
            context.close();browser.close()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server", type=pathlib.Path)
    parser.add_argument("--base-url", default="")
    parser.add_argument("--database", default="mysql")
    parser.add_argument("--client", type=pathlib.Path, required=True)
    parser.add_argument("--artifacts", type=pathlib.Path, required=True)
    parser.add_argument("--screenshots", action="store_true")
    parser.add_argument("--gui", action="store_true")
    parser.add_argument("--docker", action="store_true")
    parser.add_argument("--chromium", default="/usr/bin/chromium")
    parser.add_argument("--xvfb", default="Xvfb")
    args = parser.parse_args()
    if not args.server and not args.base_url: parser.error("--server or --base-url is required")
    if args.base_url and (args.gui or args.docker or args.screenshots): parser.error("external database mode runs API and SDK scenarios only")
    args.artifacts.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="licensing-scenarios-") as temporary:
        lab = Lab(args, pathlib.Path(temporary))
        try:
            fixture=run_core(lab)
            run_constraints(lab)
            run_offline(lab)
            if args.screenshots: run_browser(lab,fixture)
            if args.gui: run_gui(lab)
            if args.docker: run_docker(lab)
            lab.report["result"] = "passed"
        except Exception:
            lab.report["result"] = "failed"
            raise
        finally:
            lab.close()
            (args.artifacts / "report.json").write_text(json.dumps(lab.report, ensure_ascii=False, indent=2))

if __name__ == "__main__":
    main()
