#!/usr/bin/env python3
"""Native Tk reference product. All authorization and signature checks use the Go client."""
import argparse
import concurrent.futures
import json
import os
import pathlib
import queue
import secrets
import shutil
import stat
import subprocess
import tempfile
import tkinter as tk
from tkinter import filedialog, ttk
import zipfile

MESSAGES = {
    "CARD_INVALID": "卡密无效，请核对产品和购买凭证。",
    "BINDING_REQUIRED": "请填写此套餐要求的域名、IP 或组合绑定信息。",
    "BINDING_MISMATCH": "当前部署信息与授权绑定不一致，请按规则换绑。",
    "LICENSE_EXPIRED": "使用期限已结束，请续费后重试。",
    "UPDATES_EXPIRED": "更新服务已到期，当前版本的使用资格单独计算。",
    "LICENSE_REVOKED": "此授权已被吊销，请联系服务提供方。",
    "INSTANCE_INVALID": "安装身份无效或已停用，请核实设备迁移记录。",
    "DEPLOYMENT_LIMIT_EXCEEDED": "已达到购买的安装实例额度。",
    "DEVICE_LIMIT_EXCEEDED": "已达到购买的设备额度，请先停用旧设备。",
    "SEAT_LIMIT_EXCEEDED": "并发席位正在使用，请等待释放。",
    "SEAT_REQUIRED": "当前没有有效的并发席位。",
    "LICENSE_REQUIRED": "请先输入卡密激活产品。",
    "CARD_RENEWAL_ONLY": "这是续费卡，请在已有授权上续费。",
}

class ClientError(RuntimeError):
    pass

class ProductWindow:
    def __init__(self, root, client, config, data):
        self.root, self.client, self.config, self.data = root, pathlib.Path(client), pathlib.Path(config), pathlib.Path(data)
        self.session = secrets.token_hex(16)
        self.events, self.pool = queue.Queue(), concurrent.futures.ThreadPoolExecutor(max_workers=1)
        self.busy = False
        self.last_result = None
        root.title("Ebox · 产品授权")
        root.geometry("900x800")
        root.minsize(740, 740)
        root.configure(bg="#f3f6f2")
        style = ttk.Style(root)
        style.theme_use("clam")
        style.configure("TFrame", background="#f3f6f2")
        style.configure("TLabel", background="#f3f6f2", foreground="#193a2b", font=("Noto Sans CJK SC", 11))
        style.configure("Title.TLabel", font=("Noto Sans CJK SC", 25, "bold"))
        style.configure("TButton", padding=(14, 10), font=("Noto Sans CJK SC", 11))
        style.configure("TEntry", padding=8)
        body = ttk.Frame(root, padding=30)
        body.pack(fill="both", expand=True)
        ttk.Label(body, text="EBOX / 桌面产品", foreground="#728477").pack(anchor="w")
        ttk.Label(body, text="激活一次，持续管理授权", style="Title.TLabel").pack(anchor="w", pady=(8, 12))
        ttk.Label(body, text="标准程序交付 · 独立授权 · 私有安装身份").pack(anchor="w", pady=(0, 20))
        ttk.Label(body, text="授权卡密").pack(anchor="w")
        self.card = ttk.Entry(body, show="•")
        self.card.pack(fill="x", pady=(6, 14))
        ttk.Label(body, text="部署绑定信息（设备／安装实例授权可留空）").pack(anchor="w")
        self.binding = ttk.Entry(body)
        self.binding.pack(fill="x", pady=(6, 14))
        actions = ttk.Frame(body)
        actions.pack(fill="x", pady=(0, 14))
        self.activate_button = ttk.Button(actions, text="激活产品", command=self.activate)
        self.activate_button.pack(side="left")
        self.verify_button = ttk.Button(actions, text="检查授权状态", command=self.verify)
        self.verify_button.pack(side="left", padx=12)
        ttk.Button(actions,text="查看本机设备码",command=lambda:self.submit(lambda:self.call("machine-code"))).pack(side="left")
        self.status = tk.StringVar(value="尚未检查授权")
        self.detail = tk.StringVar(value="激活信息保存在本机私有目录，重装或升级请保留此目录。")
        ttk.Label(body, textvariable=self.status, font=("Noto Sans CJK SC", 15, "bold")).pack(anchor="w", pady=(8, 8))
        ttk.Label(body, textvariable=self.detail, wraplength=760, justify="left").pack(anchor="w")
        self.result_text = tk.Text(body, height=4, wrap="word", background="#ffffff", foreground="#284b35", relief="flat", padx=16, pady=14, font=("Noto Sans CJK SC", 11))
        self.result_text.pack(fill="both", expand=True, pady=18)
        self.result_text.configure(state="disabled")
        bottom = ttk.Frame(body)
        bottom.pack(fill="x")
        self.update_button = ttk.Button(bottom, text="检查并安装更新", command=lambda: self.submit(self.update))
        self.update_button.pack(side="left")
        self.claim_button = ttk.Button(bottom, text="关联用户中心", command=lambda: self.submit(lambda: self.call("claim")))
        self.claim_button.pack(side="left", padx=12)
        ttk.Button(bottom, text="导入离线授权", command=self.import_offline).pack(side="left")
        root.after(60, self.poll)
        root.protocol("WM_DELETE_WINDOW", self.close)

    def call(self, command, *values, client=None):
        cmd = [str(client or self.client), "--config", str(self.config), "--data", str(self.data), "--session", self.session, command, *map(str, values)]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
        try:
            response = json.loads(result.stdout)
        except (json.JSONDecodeError, ValueError):
            raise ClientError("产品程序没有返回有效结果。")
        if result.returncode or response.get("success") is False:
            code = response.get("code", "CLIENT_ERROR")
            raise ClientError(MESSAGES.get(code, "操作未完成：" + code))
        return response

    def submit(self, operation):
        if self.busy:
            return
        self.busy = True
        self.status.set("正在核验，请稍候…")
        self.activate_button.configure(state="disabled")
        self.verify_button.configure(state="disabled")
        self.update_button.configure(state="disabled")
        def work():
            try:
                self.events.put((operation(), None))
            except Exception as error:
                self.events.put((None, str(error)))
        self.pool.submit(work)

    def activate(self):
        card, binding = self.card.get().strip(), self.binding.get().strip()
        if not card:
            self.status.set("请输入购买的授权卡密")
            return
        self.submit(lambda: self.call("activate", card, binding) if binding else self.call("activate", card))

    def verify(self):
        self.submit(lambda: self.call("verify-file" if (self.data / "offline.key").is_file() else "verify"))

    def poll(self):
        try:
            result, error = self.events.get_nowait()
        except queue.Empty:
            pass
        else:
            self.busy = False
            for button in [self.activate_button, self.verify_button, self.update_button]:
                button.configure(state="normal")
            if error:
                self.status.set("操作未完成")
                self.detail.set(error)
                self.last_result = {"error": error}
            else:
                self.last_result = result
                self.card.delete(0, "end")
                if "device_id" in result:
                    self.status.set("本机设备码")
                    self.detail.set("将设备码交给管理员签发离线文件；设备码本身不能用于登录。")
                    text=result["device_id"]
                elif "claim_token" in result:
                    self.status.set("一次性关联凭证已生成")
                    self.detail.set("复制下方凭证，在授权站用户中心关联。10 分钟内有效。")
                    text = result["claim_token"]
                elif result.get("updated"):
                    self.status.set("产品更新成功")
                    self.detail.set("核心程序已替换并通过授权检查，客户配置和数据保持完整。")
                    text = "当前版本：" + result["version"]
                elif result.get("update_available") is False:
                    self.status.set("当前已是最新版本")
                    self.detail.set("暂无适用此平台和来源版本的新版本。")
                    text = ""
                else:
                    allowed = result.get("can_use", True)
                    self.status.set("授权有效，产品可以使用" if allowed else "当前授权不允许使用")
                    self.detail.set("使用与更新资格分别校验。" + (" 此次使用的是离线凭证。" if result.get("offline") else ""))
                    text = "当前版本：" + result.get("version", "—") + "\n使用期限：" + (result.get("usage_expired_at") or "永久使用") + "\n更新服务：" + (result.get("updates_expired_at") or ("离线状态不提供在线更新" if result.get("offline") else "永久更新"))
                    features = (result.get("authorization_terms") or {}).get("features") or []
                    if features:
                        text += "\n授权功能：" + "、".join(features)
                self.result_text.configure(state="normal")
                self.result_text.delete("1.0", "end")
                self.result_text.insert("1.0", text)
                self.result_text.configure(state="disabled")
        self.root.after(60, self.poll)

    def update(self):
        downloaded = self.call("download")
        if not downloaded.get("downloaded"):
            return downloaded
        release, archive = downloaded["release"], pathlib.Path(downloaded["path"])
        if (release.get("target") or {}).get("kind") != "installer":
            raise ClientError("此桌面示例仅安装已签名的原生程序归档。")
        binary_name = "product-client.exe" if os.name == "nt" else "product-client"
        with tempfile.TemporaryDirectory(prefix="ebox-gui-update-", dir=self.client.parent) as directory:
            stage = pathlib.Path(directory)
            with zipfile.ZipFile(archive) as package:
                entries = package.infolist()
                if len(entries) != 2 or len({item.filename for item in entries}) != 2 or {item.filename for item in entries} != {binary_name, "release.json"}:
                    raise ClientError("安装包文件清单不符合此产品的安装规范。")
                if sum(item.file_size for item in entries) > 256 * 1024 * 1024 or any(stat.S_ISLNK(item.external_attr >> 16) for item in entries):
                    raise ClientError("安装包内容不安全。")
                for item in entries:
                    (stage / item.filename).write_bytes(package.read(item))
            metadata = json.loads((stage / "release.json").read_text())
            if metadata.get("version") != release["version"]:
                raise ClientError("安装包版本与已签名清单不一致。")
            new_binary = stage / binary_name
            new_binary.chmod(0o700)
            backup = stage / "previous-client"
            shutil.copy2(self.client, backup)
            # The old, running version authorizes this exact target immediately
            # before any program files are replaced.
            self.call("authorize", release["id"])
            try:
                replacement = self.client.with_name(self.client.name + ".new")
                shutil.copy2(new_binary, replacement)
                os.replace(replacement, self.client)
                health = self.call("verify")
                if health.get("version") != release["version"] or not health.get("can_use") or metadata.get("healthy") is False:
                    raise ClientError("新版健康检查未通过")
            except Exception:
                restored = self.client.with_name(self.client.name + ".restore")
                shutil.copy2(backup, restored)
                os.replace(restored, self.client)
                raise ClientError("新版检查失败，已恢复原程序；客户数据没有被替换。")
            return {"updated": True, "version": release["version"]}

    def import_offline(self):
        path = filedialog.askopenfilename(title="选择管理员签发的离线授权", filetypes=[("授权文件", "*.key"), ("所有文件", "*")])
        if path:
            self.submit(lambda: self.call("import-offline", path))

    def close(self):
        try:
            subprocess.run([str(self.client), "--config", str(self.config), "--data", str(self.data), "--session", self.session, "release"], capture_output=True, timeout=3)
        except Exception:
            pass
        self.pool.shutdown(wait=False, cancel_futures=True)
        self.root.destroy()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--client", type=pathlib.Path, required=True)
    parser.add_argument("--config", type=pathlib.Path, required=True)
    parser.add_argument("--data", type=pathlib.Path, required=True)
    args = parser.parse_args()
    root = tk.Tk()
    ProductWindow(root, args.client.resolve(), args.config.resolve(), args.data.resolve())
    root.mainloop()

if __name__ == "__main__":
    main()
