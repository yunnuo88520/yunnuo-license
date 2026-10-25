import { createHash, createPublicKey, timingSafeEqual, verify as verifySignature } from "node:crypto";
import { VerificationError } from "./errors.js";

export const VerificationCode = Object.freeze({
  INVALID_FILE: "INVALID_FILE",
  INVALID_SIGNATURE: "INVALID_SIGNATURE",
  WRONG_PRODUCT: "WRONG_PRODUCT",
  WRONG_LICENSE: "WRONG_LICENSE",
  BINDING_MISMATCH: "BINDING_MISMATCH",
  LICENSE_EXPIRED: "LICENSE_EXPIRED",
  OFFLINE_WINDOW_EXPIRED: "OFFLINE_WINDOW_EXPIRED",
  ISSUED_IN_FUTURE: "ISSUED_IN_FUTURE",
  CLOCK_ROLLBACK: "CLOCK_ROLLBACK",
  KEY_VERSION_UNKNOWN: "KEY_VERSION_UNKNOWN",
});

export class OfflineVerifier {
  // 单公钥模式：#publicKey 有值，忽略 token 的 key_version（向后兼容）。
  #publicKey;
  // 密钥环模式：#keyRing 有值，按 token 的 key_version 选钥。
  #keyRing;
  #productCode;
  #clock;
  #clockSkewMs;

  constructor({ publicKeyPem, publicKeys, productCode, clock = () => new Date(), clockSkewMs = 5 * 60 * 1000 }) {
    const normalizedProductCode = String(productCode ?? "").trim();
    if (!normalizedProductCode) throw new TypeError("ynlicense: product code is required");
    if (typeof clock !== "function" || !Number.isFinite(clockSkewMs) || clockSkewMs < 0) {
      throw new TypeError("ynlicense: clock must be a function and clockSkewMs cannot be negative");
    }
    if (publicKeys != null) {
      if (typeof publicKeys !== "object" || Array.isArray(publicKeys)) {
        throw new TypeError("ynlicense: publicKeys must map key versions to PEM strings");
      }
      const ring = new Map();
      for (const [version, pem] of Object.entries(publicKeys)) {
        const numeric = Number(version);
        if (!Number.isInteger(numeric) || numeric < 1) {
          throw new TypeError(`ynlicense: invalid key version ${version}`);
        }
        try {
          const key = createPublicKey(pem);
          if (key.asymmetricKeyType !== "rsa") throw new Error("not an RSA key");
          ring.set(numeric, key);
        } catch (error) {
          throw new TypeError(`ynlicense: invalid RSA public key for version ${version}: ${error.message}`, { cause: error });
        }
      }
      if (ring.size === 0) throw new TypeError("ynlicense: publicKeys must contain at least one key");
      this.#keyRing = ring;
    } else {
      if (publicKeyPem == null) {
        throw new TypeError("ynlicense: publicKeyPem or publicKeys is required");
      }
      try {
        this.#publicKey = createPublicKey(publicKeyPem);
      } catch (error) {
        throw new TypeError(`ynlicense: invalid RSA public key: ${error.message}`, { cause: error });
      }
      if (this.#publicKey.asymmetricKeyType !== "rsa") {
        throw new TypeError("ynlicense: public key must be RSA");
      }
    }
    this.#productCode = normalizedProductCode;
    this.#clock = clock;
    this.#clockSkewMs = clockSkewMs;
  }

  verifyOfflineToken(token, expected) {
    const claims = this.#verifyClaims(token);
    const offlineUntil = parseRequiredDate(claims.offline_until, "offline_until");
    const issuedAt = parseRequiredDate(claims.issued_at, "issued_at");
    if (claims.type !== "offline_cache" || !String(claims.license_no ?? "").trim()) {
      fail(VerificationCode.INVALID_FILE, "invalid offline cache claims");
    }
    if (!equalFoldTrim(claims.product_code, this.#productCode)) {
      fail(VerificationCode.WRONG_PRODUCT, "product code does not match");
    }
    if (expected?.licenseNo && String(claims.license_no).trim() !== String(expected.licenseNo).trim()) {
      fail(VerificationCode.WRONG_LICENSE, "license number does not match");
    }
    if (!equalFoldTrim(claims.bind_mode, expected?.bindMode) || !claims.bind_digest) {
      fail(VerificationCode.BINDING_MISMATCH, "binding mode or digest does not match");
    }
    const expectedDigest = bindingDigest(expected.bindMode, expected.bindValue);
    if (!constantTimeStringEqual(String(claims.bind_digest).toLowerCase(), expectedDigest)) {
      fail(VerificationCode.BINDING_MISMATCH, "binding value does not match");
    }
    const now = validClockDate(this.#clock());
    if (issuedAt.getTime() > now.getTime() + this.#clockSkewMs) {
      fail(VerificationCode.ISSUED_IN_FUTURE, "token issue time is in the future");
    }
    if (claims.license_expired_at != null) {
      const licenseExpiry = parseRequiredDate(claims.license_expired_at, "license_expired_at");
      if (now.getTime() >= licenseExpiry.getTime()) {
        fail(VerificationCode.LICENSE_EXPIRED, "license has expired");
      }
    }
    if (now.getTime() >= offlineUntil.getTime()) {
      fail(VerificationCode.OFFLINE_WINDOW_EXPIRED, "offline window has expired");
    }
    return claims;
  }

  verifyLicenseFile(content, machineCode) {
    let file;
    try {
      if (typeof content === "string") file = JSON.parse(content);
      else if (content instanceof Uint8Array) file = JSON.parse(new TextDecoder().decode(content));
      else file = content;
    } catch {
      fail(VerificationCode.INVALID_FILE, "file is not valid JSON");
    }
    if (!file || file.format !== "yn-license-key" || file.version !== 1 || !file.token) {
      fail(VerificationCode.INVALID_FILE, "unsupported file format or version");
    }
    const claims = this.#verifyClaims(file.token);
    const issuedAt = parseRequiredDate(claims.issued_at, "issued_at");
    if (claims.version !== file.version || !String(claims.license_no ?? "").trim() || claims.bind_mode !== "device") {
      fail(VerificationCode.INVALID_FILE, "invalid offline license claims");
    }
    if (!equalFoldTrim(claims.product_code, this.#productCode)) {
      fail(VerificationCode.WRONG_PRODUCT, "product code does not match");
    }
    if (!equalFoldTrim(claims.machine_code, machineCode)) {
      fail(VerificationCode.BINDING_MISMATCH, "machine code does not match");
    }
    const now = validClockDate(this.#clock());
    if (issuedAt.getTime() > now.getTime() + this.#clockSkewMs) {
      fail(VerificationCode.ISSUED_IN_FUTURE, "license issue time is in the future");
    }
    if (!claims.is_permanent) {
      const expiry = parseRequiredDate(claims.expired_at, "expired_at", VerificationCode.LICENSE_EXPIRED);
      if (now.getTime() >= expiry.getTime()) {
        fail(VerificationCode.LICENSE_EXPIRED, "license has expired");
      }
    }
    return claims;
  }

  #verifyClaims(token) {
    const parts = String(token ?? "").split(".");
    if (parts.length !== 2) fail(VerificationCode.INVALID_SIGNATURE, "invalid signed token");
    let body;
    let signature;
    try {
      body = Buffer.from(parts[0], "base64url");
      signature = Buffer.from(parts[1], "base64url");
    } catch {
      fail(VerificationCode.INVALID_SIGNATURE, "invalid base64url token");
    }
    if (body.length === 0 || signature.length === 0) {
      fail(VerificationCode.INVALID_SIGNATURE, "signature verification failed");
    }
    const key = this.#selectKey(body);
    if (!verifySignature("RSA-SHA256", body, key, signature)) {
      fail(VerificationCode.INVALID_SIGNATURE, "signature verification failed");
    }
    try {
      return JSON.parse(body.toString("utf8"));
    } catch {
      fail(VerificationCode.INVALID_FILE, "signed claims are not valid JSON");
    }
  }

  // #selectKey 先按未验证的 key_version 选出验签公钥（密钥环模式），
  // 签名校验通过之前这些 claims 不可信任；单公钥模式直接返回固定公钥。
  #selectKey(body) {
    if (this.#publicKey) return this.#publicKey;
    let claims;
    try {
      claims = JSON.parse(body.toString("utf8"));
    } catch {
      fail(VerificationCode.INVALID_FILE, "signed claims are not valid JSON");
    }
    const version = Number(claims?.key_version);
    const key = Number.isInteger(version) ? this.#keyRing.get(version) : undefined;
    if (!key) fail(VerificationCode.KEY_VERSION_UNKNOWN, `no public key for key_version ${claims?.key_version}`);
    return key;
  }
}

export function bindingDigest(bindMode, bindValue) {
  const mode = String(bindMode ?? "").trim().toLowerCase();
  const value = String(bindValue ?? "").trim().toLowerCase();
  return createHash("sha256").update(mode).update(Buffer.from([0])).update(value).digest("hex");
}

function parseRequiredDate(value, field, code = VerificationCode.INVALID_FILE) {
  const date = new Date(value);
  if (value == null || Number.isNaN(date.getTime())) fail(code, `${field} is invalid`);
  return date;
}

function validClockDate(value) {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) throw new TypeError("ynlicense: clock returned an invalid date");
  return date;
}

function equalFoldTrim(left, right) {
  return String(left ?? "").trim().toLowerCase() === String(right ?? "").trim().toLowerCase();
}

function constantTimeStringEqual(left, right) {
  const leftBuffer = Buffer.from(left);
  const rightBuffer = Buffer.from(right);
  return leftBuffer.length === rightBuffer.length && timingSafeEqual(leftBuffer, rightBuffer);
}

function fail(code, message) {
  throw new VerificationError(code, message);
}
