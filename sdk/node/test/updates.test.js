import assert from "node:assert/strict";
import { createHash, generateKeyPairSync, sign } from "node:crypto";
import test from "node:test";
import { LicenseClient, verifyReleaseManifest, verifyReleasePackage, isAPIErrorCode } from "../src/index.js";

const { privateKey, publicKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
const expected = { publicKeyPem: publicKey.export({ type: "spki", format: "pem" }), productId: "prod_ebox" };
const bytes = Buffer.from("Ebox version package");
const claims = { type: "product_release", release_id: "rel_1", product_id: "prod_ebox", version: "1.2.0", sha256: createHash("sha256").update(bytes).digest("hex"), size: bytes.length, key_version: 1 };
const body = Buffer.from(JSON.stringify(claims));
const release = { ...claims, id: claims.release_id, filename: "ebox.zip", status: "published", manifest_token: `${body.toString("base64url")}.${sign("RSA-SHA256", body, privateKey).toString("base64url")}` };

test("update client checks and downloads a signed package with license credentials", async () => {
  const binding = { license_no: "lic_1", bind_mode: "domain", bind_value: "ebox.example.com" };
  const paths = [];
  const client = new LicenseClient({ baseUrl: "https://license.example.com", appKey: "app_ebox", fetch: async (url, options) => {
    const input = JSON.parse(options.body);
    assert.equal(input.app_key, "app_ebox"); assert.equal(input.license_no, binding.license_no);
    const path = new URL(url).pathname; paths.push(path);
    if (path.endsWith("/check")) {
      assert.equal(input.current_version, "1.0.0");
      return Response.json({ success: true, data: { enabled: true, update_available: true, release } });
    }
    return new Response(bytes);
  } });
  const update = await client.checkUpdate({ ...binding, current_version: "1.0.0" });
  const download = await client.downloadUpdate(update.release, binding, expected);
  assert.deepEqual(Buffer.from(download), bytes);
  assert.deepEqual(paths, ["/v1/updates/check", "/v1/updates/rel_1/download"]);
});

test("package verification rejects wrong product, tampered signature, metadata and content", () => {
  assert.equal(verifyReleaseManifest(release, expected).version, "1.2.0");
  assert.throws(() => verifyReleaseManifest(release, { ...expected, productId: "other" }), /manifest/);
  assert.throws(() => verifyReleaseManifest({ ...release, version: "99.0.0" }, expected), /manifest/);
  assert.throws(() => verifyReleaseManifest({ ...release, manifest_token: `${body.toString("base64url")}.AAAA` }, expected), /signature/);
  assert.throws(() => verifyReleasePackage(release, Buffer.from("tampered"), expected), /checksum/);
});

test("update download enforces size and preserves expired-license errors", async () => {
  for (const content of [new Uint8Array(bytes.length + 1), new Uint8Array(bytes.length - 1)]) {
    const client = new LicenseClient({ baseUrl: "https://license.example.com", appKey: "app_ebox", fetch: async () => new Response(content) });
    await assert.rejects(client.downloadUpdate(release, {}, expected), /limit|checksum/);
  }
  const client = new LicenseClient({ baseUrl: "https://license.example.com", appKey: "app_ebox", fetch: async () => Response.json({ success: false, error: { code: "LICENSE_EXPIRED", message: "license expired" } }, { status: 403 }) });
  await assert.rejects(client.downloadUpdate(release, {}, expected), e => isAPIErrorCode(e, "LICENSE_EXPIRED"));
});
