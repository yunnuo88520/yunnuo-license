#!/usr/bin/env python3
"""Reference updater for this product only, using signed authenticated image archives."""
import argparse
import json
import pathlib
import subprocess
import time
import tarfile
import urllib.request
import uuid

def docker(*args, capture=True):
    result = subprocess.run(["docker", *map(str, args)], capture_output=capture, text=True, timeout=180)
    if result.returncode:
        reason = result.stderr[-800:] if capture else args[0]
        if capture:
            try: reason = json.loads(result.stdout).get("code", reason)
            except ValueError: pass
        raise RuntimeError("Docker operation failed: " + reason)
    return result.stdout.strip() if capture else ""

def inspect_container(name):
    return json.loads(docker("inspect", name))[0]

def client(args, image, command, *values, mounts=()):
    argv = ["run", "--rm", "--network", args.network, "-v", args.volume + ":/data", "-v", str(args.config.resolve()) + ":/product/config.json:ro"]
    for source, target in mounts:
        argv += ["-v", str(source.resolve()) + ":" + target + ":ro"]
    output = docker(*argv, image, command, *values)
    value = json.loads(output)
    if value.get("success") is False:
        raise RuntimeError(value.get("code", "CLIENT_ERROR"))
    return value

def start_product(args, image):
    config = json.loads(args.config.read_text())
    argv = ["run", "-d", "--name", args.container, "--network", args.network, "--label", "yunnuo.product=" + config["app_key"], "-v", args.volume + ":/data", "-v", str(args.config.resolve()) + ":/product/config.json:ro"]
    if args.network == "host":
        argv += ["-e", f"DEMO_ADDR=127.0.0.1:{args.port}"]
    else:
        argv += ["-p", f"127.0.0.1:{args.port}:8080", "-e", "DEMO_ADDR=0.0.0.0:8080"]
    return docker(*argv, image, "serve")

def wait_product(args, version):
    base = f"http://127.0.0.1:{args.port}"
    for _ in range(100):
        try:
            with urllib.request.urlopen(base + "/healthz", timeout=2): pass
            with urllib.request.urlopen(base + "/protected", timeout=2) as response:
                data = json.load(response)
                if data.get("can_use") and data.get("version") == version:
                    return data
        except (OSError, ValueError):
            pass
        time.sleep(0.1)
    raise RuntimeError("New product did not become healthy")

def stage_update(args):
    current = inspect_container(args.container)
    config = json.loads(args.config.read_text())
    if current.get("Config", {}).get("Labels", {}).get("yunnuo.product") != config["app_key"]:
        raise RuntimeError("Container does not belong to this product")
    image = current["Config"]["Image"]
    result = client(args, image, "download")
    if not result.get("downloaded"):
        return {"updated": False, "update_available": False}
    args.staging.mkdir(parents=True, exist_ok=True, mode=0o700)
    release = result["release"]
    docker("cp", args.container + ":/data/update.package", args.staging / "image.tar.gz")
    (args.staging / "image.tar.gz").chmod(0o644)
    release_file = args.staging / "release.json"
    release_file.write_text(json.dumps(release))
    release_file.chmod(0o644)
    (args.staging / "source.json").write_text(json.dumps({"image": image, "product": config["app_key"]}))
    return {"staged": True, "version": release["version"]}

def install_staged(args):
    current = inspect_container(args.container)
    config = json.loads(args.config.read_text())
    if current.get("Config", {}).get("Labels", {}).get("yunnuo.product") != config["app_key"]:
        raise RuntimeError("Container does not belong to this product")
    old_image = current["Config"]["Image"]
    release_file, archive = args.staging / "release.json", args.staging / "image.tar.gz"
    mounts = [(release_file, "/staged/release.json"), (archive, "/staged/image.tar.gz")]
    verified = client(args, old_image, "verify-package", "/staged/release.json", "/staged/image.tar.gz", mounts=mounts)
    target = verified.get("target") or {}
    if target.get("kind") != "container_archive":
        raise RuntimeError("This updater only accepts container archives")
    if not config.get("image_repository") or target["image_tag"].rsplit(":", 1)[0] != config["image_repository"]:
        raise RuntimeError("The signed image is outside this product's configured repository")
    with tarfile.open(archive, "r:*") as package:
        members = [item for item in package if item.name == "manifest.json"]
        if len(members) != 1 or not members[0].isfile() or members[0].size > 1024 * 1024:
            raise RuntimeError("Invalid Docker archive manifest")
        manifest = json.load(package.extractfile(members[0]))
        if len(manifest) != 1 or manifest[0].get("RepoTags") != [target["image_tag"]]:
            raise RuntimeError("Docker archive must contain exactly the authorized product image")
    client(args, old_image, "authorize", verified["id"])
    docker("load", "-i", archive)
    loaded = json.loads(docker("image", "inspect", target["image_tag"]))[0]
    if loaded["Id"] != target["image_digest"]:
        raise RuntimeError("Loaded image does not match the signed immutable image identity")
    client(args, old_image, "authorize", verified["id"])
    backup = args.container + "-previous-" + uuid.uuid4().hex[:8]
    stopped = renamed = False
    try:
        docker("stop", "--time", "20", args.container)
        stopped = True
        docker("rename", args.container, backup)
        renamed = True
        start_product(args, target["image_digest"])
        status = wait_product(args, verified["version"])
    except Exception as cause:
        try:
            if renamed:
                # Only the task's replacement can use this name after rename.
                result = subprocess.run(["docker", "inspect", args.container], capture_output=True, text=True)
                if result.returncode == 0:
                    docker("rm", "-f", args.container)
                docker("rename", backup, args.container)
            if stopped:
                docker("start", args.container)
        except Exception as recovery:
            raise RuntimeError("Update failed; automatic recovery failed: " + str(recovery)) from cause
        raise RuntimeError("Update failed; the previous container and its data volume were restored") from cause
    docker("rm", backup)
    return {"updated": True, "version": verified["version"], "instance_id": status["instance_id"]}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--container", required=True)
    parser.add_argument("--volume", required=True)
    parser.add_argument("--config", type=pathlib.Path, required=True)
    parser.add_argument("--staging", type=pathlib.Path, required=True)
    parser.add_argument("--port", type=int, required=True)
    parser.add_argument("--network", default="bridge")
    parser.add_argument("--stage-only", action="store_true")
    parser.add_argument("--install-staged", action="store_true")
    args = parser.parse_args()
    try:
        result = None if args.install_staged else stage_update(args)
        if args.stage_only or result and result.get("update_available") is False:
            print(json.dumps(result))
        else:
            print(json.dumps(install_staged(args)))
    except Exception as error:
        print(json.dumps({"updated": False, "error": str(error)}))
        raise SystemExit(1)

if __name__ == "__main__":
    main()
