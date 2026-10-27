import assert from "node:assert/strict";
import { generateKeyPairSync, sign } from "node:crypto";
import test from "node:test";

import {
  OfflineVerifier,
  VerificationCode,
  bindingDigest,
  isVerificationErrorCode,
} from "../src/index.js";

const now = new Date("2026-08-12T03:00:00Z");
const { privateKey, publicKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
const publicKeyPem = publicKey.export({ type: "spki", format: "pem" });

test("expired update entitlement does not expire perpetual offline usage", () => {
  const token = signClaims({ type: "offline_cache", license_no: "lic_perpetual", product_code: "YN", license_mode: "perpetual", bind_mode: "device", bind_digest: bindingDigest("device", "machine-A"), license_expired_at: null, usage_expired_at: null, updates_expired_at: "2026-08-01T00:00:00Z", offline_until: "2026-08-13T00:00:00Z", issued_at: now.toISOString(), key_version: 1 });
  const verifier = new OfflineVerifier({ publicKeyPem, productCode: "YN", clock: () => now });
  const claims = verifier.verifyOfflineToken(token, { bindMode: "device", bindValue: "machine-A" });
  assert.equal(claims.license_mode, "perpetual");
  assert.equal(claims.usage_expired_at, null);
});

test("verifies online cache token and rejects wrong binding or tampering", () => {
  const claims = {
    type: "offline_cache",
    license_no: "lic_test",
    product_code: "YN",
    bind_mode: "device",
    bind_digest: bindingDigest("device", "machine-A"),
    license_expired_at: "2026-08-14T03:00:00Z",
    offline_until: "2026-08-13T03:00:00Z",
    issued_at: now.toISOString(),
    key_version: 1,
  };
  const token = signClaims(claims);
  const verifier = new OfflineVerifier({ publicKeyPem, productCode: "YN", clock: () => now });
  assert.equal(verifier.verifyOfflineToken(token, { licenseNo: "lic_test", bindMode: "device", bindValue: "machine-A" }).license_no, "lic_test");
  assert.throws(
    () => verifier.verifyOfflineToken(token, { licenseNo: "lic_test", bindMode: "device", bindValue: "machine-B" }),
    (error) => isVerificationErrorCode(error, VerificationCode.BINDING_MISMATCH),
  );
  const separator = token.indexOf(".") + 1;
  const replacement = token[separator] === "A" ? "B" : "A";
  const tampered = `${token.slice(0, separator)}${replacement}${token.slice(separator + 1)}`;
  assert.throws(
    () => verifier.verifyOfflineToken(tampered, { licenseNo: "lic_test", bindMode: "device", bindValue: "machine-A" }),
    (error) => isVerificationErrorCode(error, VerificationCode.INVALID_SIGNATURE),
  );
});

test("separates license expiry and offline-window expiry", () => {
  const verifier = new OfflineVerifier({ publicKeyPem, productCode: "YN", clock: () => now });
  const base = {
    type: "offline_cache",
    license_no: "lic_test",
    product_code: "YN",
    bind_mode: "device",
    bind_digest: bindingDigest("device", "machine-A"),
    issued_at: now.toISOString(),
    key_version: 1,
  };
  assert.throws(
    () => verifier.verifyOfflineToken(signClaims({ ...base, license_expired_at: "2026-08-11T03:00:00Z", offline_until: "2026-08-13T03:00:00Z" }), expectation()),
    (error) => isVerificationErrorCode(error, VerificationCode.LICENSE_EXPIRED),
  );
  assert.throws(
    () => verifier.verifyOfflineToken(signClaims({ ...base, license_expired_at: "2026-08-14T03:00:00Z", offline_until: "2026-08-11T03:00:00Z" }), expectation()),
    (error) => isVerificationErrorCode(error, VerificationCode.OFFLINE_WINDOW_EXPIRED),
  );
});

test("verifies full offline license file", () => {
  const claims = {
    version: 1,
    license_no: "off_test",
    product_id: "prod_test",
    product_code: "YN",
    product_name: "Test Product",
    app_key: "app_test",
    bind_mode: "device",
    machine_code: "MACHINE-ABC",
    issued_at: now.toISOString(),
    expired_at: "2027-08-12T03:00:00Z",
    is_permanent: false,
  };
  const file = JSON.stringify({ format: "yn-license-key", version: 1, token: signClaims(claims) });
  const verifier = new OfflineVerifier({ publicKeyPem, productCode: "YN", clock: () => now });
  assert.equal(verifier.verifyLicenseFile(Buffer.from(file), "machine-abc").license_no, "off_test");
  assert.throws(
    () => verifier.verifyLicenseFile(file, "other-machine"),
    (error) => isVerificationErrorCode(error, VerificationCode.BINDING_MISMATCH),
  );
});

function expectation() {
  return { licenseNo: "lic_test", bindMode: "device", bindValue: "machine-A" };
}

function signClaims(claims, signingKey = privateKey) {
  const body = Buffer.from(JSON.stringify(claims));
  const signature = sign("RSA-SHA256", body, signingKey);
  return `${body.toString("base64url")}.${signature.toString("base64url")}`;
}

test("key ring selects the signing key by key_version", async () => {
  const { publicKey: keyV2Public, privateKey: keyV2Private } = generateKeyPairSync("rsa", { modulusLength: 2048 });
  const pemV2 = keyV2Public.export({ type: "spki", format: "pem" });
  const ringVerifier = new OfflineVerifier({
    publicKeys: { 1: publicKeyPem, 2: pemV2 },
    productCode: "YN",
    clock: () => now,
  });
  // v2 私钥签发的新 token（密钥轮换后服务端行为）在密钥环验签器上通过。
  const v2Token = signClaims({
    type: "offline_cache", license_no: "lic_ring", product_code: "YN",
    bind_mode: "device", bind_digest: bindingDigest("device", "machine-A"),
    license_expired_at: "2026-08-14T03:00:00Z", offline_until: "2026-08-13T03:00:00Z",
    issued_at: now.toISOString(), key_version: 2,
  }, keyV2Private);
  assert.equal(ringVerifier.verifyOfflineToken(v2Token, { licenseNo: "lic_ring", bindMode: "device", bindValue: "machine-A" }).license_no, "lic_ring");
  // v1 签名的旧 token 同样通过。
  const v1Token = signClaims({
    type: "offline_cache", license_no: "lic_ring", product_code: "YN",
    bind_mode: "device", bind_digest: bindingDigest("device", "machine-A"),
    license_expired_at: "2026-08-14T03:00:00Z", offline_until: "2026-08-13T03:00:00Z",
    issued_at: now.toISOString(), key_version: 1,
  });
  assert.equal(ringVerifier.verifyOfflineToken(v1Token, { bindMode: "device", bindValue: "machine-A" }).license_no, "lic_ring");
  // 未知版本拒绝。
  const v9Token = signClaims({
    type: "offline_cache", license_no: "lic_ring", product_code: "YN",
    bind_mode: "device", bind_digest: bindingDigest("device", "machine-A"),
    license_expired_at: "2026-08-14T03:00:00Z", offline_until: "2026-08-13T03:00:00Z",
    issued_at: now.toISOString(), key_version: 9,
  });
  assert.throws(
    () => ringVerifier.verifyOfflineToken(v9Token, { bindMode: "device", bindValue: "machine-A" }),
    (error) => isVerificationErrorCode(error, VerificationCode.KEY_VERSION_UNKNOWN),
  );
  // 被未知版本私钥签名的 token 不能用环内公钥伪造通过（签名验证仍生效）。
  assert.throws(
    () => ringVerifier.verifyOfflineToken(signClaims({
      type: "offline_cache", license_no: "lic_ring", product_code: "YN",
      bind_mode: "device", bind_digest: bindingDigest("device", "machine-A"),
      license_expired_at: "2026-08-14T03:00:00Z", offline_until: "2026-08-13T03:00:00Z",
      issued_at: now.toISOString(), key_version: 2,
    }), { bindMode: "device", bindValue: "machine-A" }),
    (error) => isVerificationErrorCode(error, VerificationCode.INVALID_SIGNATURE),
  );
});

test("borrowed seat token is bound to installation, seat and process session",()=>{
 const token=signClaims({type:"offline_cache",purpose:"seat_lease",license_no:"lic_test",product_code:"YN",bind_mode:"device",bind_digest:bindingDigest("device","machine-A"),instance_id:"installation_0001",seat_id:"seat_1",session_id:"session_00000001",offline_until:"2026-08-13T00:00:00Z",issued_at:now.toISOString(),key_version:1});
 const verifier=new OfflineVerifier({publicKeyPem,productCode:"YN",clock:()=>now});
 const expected={licenseNo:"lic_test",bindMode:"device",bindValue:"machine-A",instanceId:"installation_0001",seatId:"seat_1",sessionId:"session_00000001"};
 assert.equal(verifier.verifyOfflineToken(token,expected).seat_id,"seat_1");
 for(const field of ["instanceId","seatId","sessionId"])assert.throws(()=>verifier.verifyOfflineToken(token,{...expected,[field]:"other"}),e=>e.code===VerificationCode.BINDING_MISMATCH);
});
