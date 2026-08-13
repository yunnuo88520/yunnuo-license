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

function signClaims(claims) {
  const body = Buffer.from(JSON.stringify(claims));
  const signature = sign("RSA-SHA256", body, privateKey);
  return `${body.toString("base64url")}.${signature.toString("base64url")}`;
}
