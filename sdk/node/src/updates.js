import { createHash, createPublicKey, verify } from "node:crypto";

const MAX_PACKAGE_BYTES = 256 * 1024 * 1024;

// publicKeyPem and productId must be pinned by the product, not supplied by an update response.
export function verifyReleaseManifest(release, { publicKeyPem, productId }) {
  const parts = String(release?.manifest_token || "").split(".");
  if (parts.length !== 2 || !parts.every(p => /^[A-Za-z0-9_-]+$/.test(p))) {
    throw new Error("ynlicense: invalid release manifest");
  }
  const body = Buffer.from(parts[0], "base64url");
  const key = createPublicKey(publicKeyPem);
  if (key.asymmetricKeyType !== "rsa" || !verify("RSA-SHA256", body, key, Buffer.from(parts[1], "base64url"))) {
    throw new Error("ynlicense: invalid release signature");
  }
  const claims = JSON.parse(body.toString("utf8"));
  if (!productId || claims.type !== "product_release" || claims.product_id !== productId ||
      claims.release_id !== release.id || claims.product_id !== release.product_id ||
      claims.version !== release.version || claims.sha256 !== release.sha256 ||
      claims.size !== release.size || claims.key_version !== release.key_version ||
      !/^[a-f0-9]{64}$/.test(claims.sha256) ||
      !Number.isSafeInteger(claims.size) || claims.size <= 0 || claims.size > MAX_PACKAGE_BYTES) {
    throw new Error("ynlicense: release manifest does not match expected product or package");
  }
  return claims;
}

export function verifyReleasePackage(release, content, expected) {
  const claims = verifyReleaseManifest(release, expected);
  const bytes = Buffer.from(content);
  if (bytes.length !== claims.size || createHash("sha256").update(bytes).digest("hex") !== claims.sha256) {
    throw new Error("ynlicense: release package checksum mismatch");
  }
  return claims;
}
