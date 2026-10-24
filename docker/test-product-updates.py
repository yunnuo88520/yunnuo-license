#!/usr/bin/env python3
"""Exercise developer isolation, package delivery and volume persistence on a live server."""
import hashlib
import io
import json
import os
import pathlib
import sys
import urllib.error
import urllib.request
import uuid
import zipfile

mode, base, state_path = sys.argv[1:]


def request(path, data=None, token="", expected=200, raw=False, content_type="application/json"):
    headers = {"Content-Type": content_type}
    if token:
        headers["Authorization"] = "Bearer " + token
    payload = data if isinstance(data, bytes) else json.dumps(data).encode() if data is not None else None
    req = urllib.request.Request(base + path, data=payload, headers=headers)
    try:
        response = urllib.request.urlopen(req, timeout=20)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        assert response.status == expected, f"{path}: status {response.status}, expected {expected}"
        if raw or expected != 200:
            return body
        envelope = json.loads(body)
        assert envelope["success"], path
        return envelope["data"]


if mode == "prepare":
    password = os.environ["YN_TEST_ADMIN_PASSWORD"]
    admin = request("/admin/login", {"username": "admin", "password": password})["access_token"]
    tokens = []
    for name in ["update-developer-a", "update-developer-b"]:
        request("/admin/users", {"username": name, "password": password, "role": "developer"}, admin)
        tokens.append(request("/developer/login", {"username": name, "password": password})["access_token"])
    developer, other = tokens
    product = request("/developer/products", {"name": "Ebox update E2E", "code": "EBOX", "bind_mode": "domain"}, developer)
    prefix = "/developer/products/" + product["id"]
    assert request("/developer/products", token=other) == []
    request(prefix + "/releases", token=other, expected=404)
    request("/admin/products", token=developer, expected=403)
    assert not request(prefix + "/updates", token=developer)["enabled"]
    request(prefix + "/updates", {"enabled": True}, developer)
    archive = io.BytesIO()
    with zipfile.ZipFile(archive, "w") as package:
        package.writestr("version.txt", "Ebox 1.2.0")
    package_bytes = archive.getvalue()
    boundary = uuid.uuid4().hex
    parts = []
    for key, value in [("version", "1.2.0"), ("notes", "Ebox integration release")]:
        parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{key}"\r\n\r\n{value}\r\n'.encode())
    parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="package"; filename="ebox.zip"\r\nContent-Type: application/zip\r\n\r\n'.encode() + package_bytes + f'\r\n--{boundary}--\r\n'.encode())
    release = request(prefix + "/releases", b"".join(parts), developer, content_type="multipart/form-data; boundary=" + boundary)
    batch = request(prefix + "/card-batches", {"name": "Update E2E", "quantity": 1, "duration_days": 30}, developer)
    binding = {"app_key": product["app_key"], "bind_mode": "domain", "bind_value": "ebox.example.com"}
    activation = request("/v1/licenses/activate", {**binding, "card_code": batch["codes"][0]})
    binding["license_no"] = activation["license_no"]
    check_input = {**binding, "current_version": "1.0.0"}
    assert not request("/v1/updates/check", check_input)["update_available"]
    request(prefix + "/releases/" + release["id"] + "/publish", {}, developer)
    assert request("/v1/updates/check", check_input)["release"]["id"] == release["id"]
    state = {"binding": binding, "release": release, "prefix": prefix, "developer": developer}
    pathlib.Path(state_path).write_text(json.dumps(state))
    os.chmod(state_path, 0o600)
    print("Developer isolation and release publishing passed")
elif mode == "verify":
    state = json.loads(pathlib.Path(state_path).read_text())
    binding, release = state["binding"], state["release"]
    check_input = {**binding, "current_version": "1.0.0"}
    update = request("/v1/updates/check", check_input)
    assert update["update_available"] and update["release"]["id"] == release["id"]
    path = "/v1/updates/" + release["id"] + "/download"
    package_bytes = request(path, binding, raw=True)
    assert len(package_bytes) == release["size"]
    assert hashlib.sha256(package_bytes).hexdigest() == release["sha256"]
    request(path, {**binding, "bind_value": "wrong.example.com"}, expected=403)
    request(state["prefix"] + "/releases/" + release["id"] + "/withdraw", {}, state["developer"])
    request(path, binding, expected=404)
    print("Package persistence, authorized download and withdrawal passed after restart")
else:
    raise SystemExit("mode must be prepare or verify")
