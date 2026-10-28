"""Standard HTTP + Ed25519 request proof + RS256 JWT. No vendor SDK.
AppID/public key are public. NEVER distribute your developer AppSecret.
Install: python -m pip install -r requirements.txt
"""
import base64, hashlib, ipaddress, json, os, pathlib, secrets, sys, time, urllib.parse, urllib.request, urllib.error
import jwt
import ssl, socket
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives import serialization
APP_ID = "@@APP_ID@@"
BASE_URL = os.environ.get("YN_LICENSE_URL", "@@LICENSE_URL@@").rstrip("/")
STATE = pathlib.Path(os.environ.get("YN_STATE_DIR", "./license-state")) / APP_ID
FORM = os.environ.get("YN_FORM", "@@DEFAULT_FORM@@")
KIND = os.environ.get("YN_BINDING_TYPE", "@@DEFAULT_BINDING@@")
ISSUER = "urn:yunnuo:licensing:v2"

def save(name, value):
    STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
    raw = value if isinstance(value, bytes) else json.dumps(value).encode()
    temp = STATE / (name + "." + secrets.token_hex(8))
    fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "wb") as stream:
        stream.write(raw); stream.flush(); os.fsync(stream.fileno())
    os.replace(temp, STATE / name)

def discard_tokens():
    path = STATE / "cache.json"
    cache = json.loads(path.read_text()) if path.exists() else {}
    # Preserve installation identity so suspension, expiry and seat release
    # can recover online without asking the customer to activate again.
    state = {k: cache[k] for k in ("activation_id", "license_id", "highwater") if cache.get(k)}
    save("cache.json", state)

def identity():
    STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
    keyfile = STATE / "installation.key"
    # Exclusive creation prevents replacing a key if another process starts.
    try:
        key = Ed25519PrivateKey.generate()
        fd = os.open(keyfile, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "wb") as stream:
            stream.write(key.private_bytes(serialization.Encoding.Raw, serialization.PrivateFormat.Raw, serialization.NoEncryption()))
    except FileExistsError:
        key = Ed25519PrivateKey.from_private_bytes(keyfile.read_bytes())
    public = key.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
    return key, public
KEY, PUBLIC = identity()
PUBLIC_HASH = hashlib.sha256(PUBLIC).hexdigest()
PUBLIC_TEXT = base64.urlsafe_b64encode(PUBLIC).rstrip(b"=").decode()

def binding():
    value = (os.environ.get("YN_BINDING_VALUE") or (PUBLIC_HASH if KIND == "installation" else ""))
    if KIND == "seat": value = "seat"
    elif KIND == "domain":
        value = urllib.parse.urlsplit(value if "://" in value else "//" + value).hostname or ""
        value = value.rstrip(".").lower().encode("idna").decode()
    elif KIND == "ip": value = str(ipaddress.ip_address(value))
    elif KIND == "machine_id":
        # For Docker/K8s provide a host fingerprint file mounted read-only.
        host_file = os.environ.get("YN_HOST_FINGERPRINT_FILE")
        if host_file: value = pathlib.Path(host_file).read_text().strip().lower()
        elif FORM in ("container", "cluster") or pathlib.Path("/.dockerenv").exists() or os.environ.get("KUBERNETES_SERVICE_HOST"):
            raise RuntimeError("Container machine fingerprints are forbidden: use seat or mount a host SHA-256 fingerprint file plus persistent state volume")
        elif not value:
            for source in ("/sys/class/dmi/id/product_uuid", "/etc/machine-id"):
                try:
                    hardware = pathlib.Path(source).read_text().strip()
                    if hardware: value = hashlib.sha256(hardware.encode()).hexdigest(); break
                except OSError: pass
        if len(value) != 64 or any(c not in "0123456789abcdef" for c in value): raise RuntimeError("A real SHA-256 hardware fingerprint is required; no random fallback")
    if not value: raise RuntimeError("Set YN_BINDING_VALUE")
    return {"type": KIND, "value": value}
BINDING = binding()
LOCAL_HASH = hashlib.sha256((KIND + "\0" + (PUBLIC_HASH if KIND == "seat" else BINDING["value"])).encode()).hexdigest()

def clock(cache):
    current = int(time.time())
    if current + 120 < cache.get("highwater", 0): raise RuntimeError("System clock moved backwards; online validation is required")
    return max(current, cache.get("highwater", 0))

def jwks(force=False):
    path = STATE / "jwks.json"
    if path.exists() and not force: return json.loads(path.read_text())
    try:
        with urllib.request.urlopen(BASE_URL + "/v2/applications/" + APP_ID + "/.well-known/jwks.json", timeout=10) as response: keys = json.load(response)
        save("jwks.json", keys)
        return keys
    except (urllib.error.URLError, TimeoutError, OSError):
        path = STATE / "jwks.json"
        if not path.exists(): raise
        return json.loads(path.read_text())

def local_verify(token, purpose, activation, cache):
    # A JWKS URL is configured by the developer, NEVER read from token jku/x5u.
    header = jwt.get_unverified_header(token)
    if header.get("alg") != "RS256" or not header.get("kid", "").startswith(APP_ID + ".v"): raise RuntimeError("Untrusted JWT algorithm or application key")
    keys = jwks()
    key = next((k for k in keys["keys"] if k.get("kid") == header["kid"]), None)
    if key is None:
        keys = jwks(True)
        key = next((k for k in keys["keys"] if k.get("kid") == header["kid"]), None)
        if key is None: raise RuntimeError("Unknown signing key")
    claims = jwt.decode(token, jwt.PyJWK.from_dict(key).key, algorithms=["RS256"], audience=APP_ID, issuer=ISSUER,
        options={"require": ["exp", "iat", "nbf", "aud", "iss", "sub", "jti", "token_use", "binding", "activation_id"]})
    if clock(cache) >= claims["exp"] or claims["token_use"] != purpose or claims["activation_id"] != activation:
        raise RuntimeError("Expired, wrong-purpose or wrong-installation JWT")
    if claims["binding"] != {"type": KIND, "hash": LOCAL_HASH}: raise RuntimeError("Binding fingerprint mismatch")
    return claims

def signed_request(path, payload):
    body = json.dumps(payload, separators=(",", ":"), ensure_ascii=False).encode()
    stamp = str(int(time.time())); nonce = secrets.token_urlsafe(24)
    message = "\n".join(("YN-AUTH-V2", "POST", path, APP_ID, stamp, nonce, hashlib.sha256(body).hexdigest()))
    signature = base64.urlsafe_b64encode(KEY.sign(message.encode())).rstrip(b"=").decode()
    request = urllib.request.Request(BASE_URL + path, body, headers={"Content-Type": "application/json", "X-YN-Timestamp": stamp, "X-YN-Nonce": nonce, "X-YN-Signature": signature}, method="POST")
    return request

def handle_refusal(detail, preserve_update_errors=False):
    try: code = json.loads(detail).get("error", {}).get("code", "")
    except ValueError: code = ""
    # Update-only refusal never takes away separately purchased usage. A
    # revoked/suspended/expired installation still clears both cached grants.
    usage_refusal = code.startswith(("LICENSE_", "ACTIVATION_", "BINDING_", "TOKEN_", "REQUEST_", "SIGNATURE_", "SEAT_")) or code in {"APPLICATION_DISABLED", "MODEL_DISABLED", "SOURCE_IP_MISMATCH"}
    if not preserve_update_errors or usage_refusal: discard_tokens()

def send(path, payload, preserve_update_errors=False):
    request = signed_request(path, payload)
    try:
        with urllib.request.urlopen(request, timeout=15) as response: return json.load(response)["data"]
    except urllib.error.HTTPError as error:
        # Any explicit HTTP refusal is authoritative. It NEVER enables offline mode.
        detail = error.read().decode(errors="replace")
        handle_refusal(detail, preserve_update_errors)
        raise RuntimeError("Authorization server refused request: " + detail) from error

def main():
    operation = sys.argv[1] if len(sys.argv) > 1 else "verify"
    path = STATE / "cache.json"; cache = json.loads(path.read_text()) if path.exists() else {}
    clock(cache)
    if operation == "local":
        result = local_verify(cache["jwt"], "license", cache["activation_id"], cache)
        print(json.dumps({"valid": True, "license_id": result["sub"]})); return
    if cache.get("jwt"):
        try: local_verify(cache["jwt"], "license", cache["activation_id"], cache)
        except jwt.ExpiredSignatureError: pass
        except Exception:
            discard_tokens()
            cache = json.loads((STATE / "cache.json").read_text())
    if operation == "report":
        if not cache.get("activation_id"): raise RuntimeError("Activate before reporting integrity evidence")
        result = send("/v2/security/report", {"app_id": APP_ID, "activation_id": cache["activation_id"], "signal": os.environ.get("YN_INTEGRITY_SIGNAL", "binary_modified"), "evidence_sha256": os.environ["YN_EVIDENCE_SHA256"], "summary": os.environ.get("YN_INTEGRITY_SUMMARY", "")})
        print(json.dumps(result)); return
    if operation == "request":
        print(json.dumps(send("/v2/installations/request", {"app_id": APP_ID, "public_key": PUBLIC_TEXT, "binding": BINDING, "label": os.environ.get("YN_INSTALLATION_LABEL", "")}))); return
    activating = operation == "activate" or not cache.get("activation_id")
    if activating:
        payload = {"app_id": APP_ID, "public_key": PUBLIC_TEXT, "binding": BINDING, "form": FORM, "label": os.environ.get("YN_INSTALLATION_LABEL", "")}
        if os.environ.get("YN_CARD"): payload["card_code"] = os.environ["YN_CARD"]
        elif os.environ.get("YN_INVITATION_ID"): payload.update(invitation_id=os.environ["YN_INVITATION_ID"], invitation_token=os.environ["YN_INVITATION_TOKEN"])
        else: payload["pending_id"] = os.environ.get("YN_PENDING_ID", "")
        if os.environ.get("YN_BORROW_SECONDS"): payload["borrow_seconds"] = int(os.environ["YN_BORROW_SECONDS"])
        endpoint = "/v2/activate"
    else:
        payload = {"app_id": APP_ID, "activation_id": cache["activation_id"], "binding": BINDING, "form": FORM}
        # Installation proof permits recovery after an old JWT's key retires.
        # Do not treat an expired JWT as locally valid or ship AppSecret.
        if operation in ("release", "consume", "renew"): payload["action"] = operation
        if operation == "consume": payload.update(units=int(os.environ.get("YN_UNITS", "1")), idempotency_key=os.environ["YN_OPERATION_ID"])
        if operation == "renew": payload["renewal_card"] = os.environ["YN_RENEWAL_CARD"]
        endpoint = "/v2/verify"
    try:
        result = send(endpoint, payload)
    except (urllib.error.URLError, TimeoutError, ConnectionError) as error:
        reason = error.reason if isinstance(error, urllib.error.URLError) else error
        if isinstance(reason, ssl.SSLError) or not isinstance(reason, (TimeoutError, ConnectionError, socket.gaierror)): raise
        if activating or operation != "verify" or not cache.get("offline_jwt"): raise
        claims = local_verify(cache["offline_jwt"], "offline_grace", cache["activation_id"], cache)
        save("cache.json", {**cache, "highwater": max(int(time.time()), cache.get("highwater", 0))})
        print(json.dumps({"valid": True, "offline": True, "grace_until": claims["exp"]})); return
    if result.get("released"): discard_tokens(); print(json.dumps(result)); return
    # Verify the new token before persisting or enabling product features.
    local_verify(result["jwt"], "license", result["activation_id"], {})
    result["highwater"] = max(int(time.time()), int(__import__("datetime").datetime.fromisoformat(result["server_time"].replace("Z", "+00:00")).timestamp()))
    save("cache.json", result)
    # Do not log JWT, credentials, or raw machine identifiers.
    print(json.dumps({"valid": True, "license_id": result["license_id"], "activation_id": result["activation_id"], "remaining_units": result.get("remaining_units")}))
if __name__ == "__main__": main()
