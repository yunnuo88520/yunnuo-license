import { createHash, createPublicKey, verify } from "node:crypto";

const MAX_PACKAGE_BYTES = 256 * 1024 * 1024;

// publicKeyPem and productId must be pinned by the product, not supplied by an update response.
export function verifyReleaseManifest(release, { publicKeyPem, productId, target, currentVersion }) {
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
  const targetFields=["os","arch","runtime","channel","kind","min_version","max_version","image_tag","image_digest"];
  if(Boolean(claims.target)!==Boolean(release.target)||targetFields.some(k=>(claims.target?.[k]??"")!==(release.target?.[k]??"")))throw new Error("ynlicense: release target does not match signed manifest");
  if(claims.target){
    if(!target)throw new Error("ynlicense: an explicit client target is required for platform releases");
    for(const field of ["os","arch","runtime"]){if(claims.target[field]!=="any"&&claims.target[field]!==target[field])throw new Error("ynlicense: release is for another client platform")}
    if(claims.target.channel!==(target.channel||"stable")||(target.kind&&claims.target.kind!==target.kind))throw new Error("ynlicense: release channel or delivery kind mismatch");
    if(claims.target.min_version||claims.target.max_version){
      const parts=value=>/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(value||"")?value.split(".").map(Number):null;
      const version=parts(currentVersion);if(!version)throw new Error("ynlicense: source version is required");
      const compare=(a,b)=>{for(let i=0;i<3;i++)if(a[i]!==b[i])return a[i]-b[i];return 0};
      if(claims.target.min_version&&compare(version,parts(claims.target.min_version))<0||claims.target.max_version&&compare(version,parts(claims.target.max_version))>0)throw new Error("ynlicense: source version is outside the signed upgrade path");
    }
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
