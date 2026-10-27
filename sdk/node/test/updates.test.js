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
    if(path.endsWith("/authorize")) return Response.json({success:true,data:{authorized:true,release_id:release.id}});
    return new Response(bytes);
  } });
  const update = await client.checkUpdate({ ...binding, current_version: "1.0.0" });
  const download = await client.downloadUpdate(update.release, binding, expected);
  assert.deepEqual(Buffer.from(download), bytes);
  assert.deepEqual(paths, ["/v1/updates/check", "/v1/updates/rel_1/download", "/v1/updates/rel_1/authorize"]);
});

test("package verification rejects wrong product, tampered signature, metadata and content", () => {
  assert.equal(verifyReleaseManifest(release, expected).version, "1.2.0");
  assert.throws(() => verifyReleaseManifest(release, { ...expected, productId: "other" }), /manifest/);
  assert.throws(() => verifyReleaseManifest({ ...release, version: "99.0.0" }, expected), /manifest/);
  assert.throws(() => verifyReleaseManifest({ ...release, manifest_token: `${body.toString("base64url")}.AAAA` }, expected), /signature/);
  assert.throws(() => verifyReleasePackage(release, Buffer.from("tampered"), expected), /checksum/);
});

test("optional integrity reporting sends observed checksum and preserves original failure", async () => {
  const tampered=Buffer.from(bytes);tampered[0]^=1;const reports=[];
  const client=new LicenseClient({baseUrl:"https://license.example.com",appKey:"app_ebox",reportIntegrityFailures:true,fetch:async(url,options)=>{
    if(url.endsWith("/security/report")){reports.push(JSON.parse(options.body));return Response.json({success:false,error:{code:"SECURITY_REPORTS_DISABLED",message:"disabled"}},{status:403})}
    return new Response(tampered)
  }});
  await assert.rejects(client.downloadUpdate(release,{license_no:"lic_1",bind_mode:"domain",bind_value:"ebox.example.com"},expected),/checksum/);
  assert.equal(reports.length,1);assert.equal(reports[0].release_id,release.id);assert.equal(reports[0].observed_sha256,createHash("sha256").update(tampered).digest("hex"));
});

test("update download enforces size and preserves expired-license errors", async () => {
  for (const content of [new Uint8Array(bytes.length + 1), new Uint8Array(bytes.length - 1)]) {
    const client = new LicenseClient({ baseUrl: "https://license.example.com", appKey: "app_ebox", fetch: async () => new Response(content) });
    await assert.rejects(client.downloadUpdate(release, {}, expected), /limit|checksum/);
  }
  const client = new LicenseClient({ baseUrl: "https://license.example.com", appKey: "app_ebox", fetch: async () => Response.json({ success: false, error: { code: "LICENSE_EXPIRED", message: "license expired" } }, { status: 403 }) });
  await assert.rejects(client.downloadUpdate(release, {}, expected), e => isAPIErrorCode(e, "LICENSE_EXPIRED"));
});

 test("package downloaded before expiry is not returned when post-download authorization expires",async()=>{
 const client=new LicenseClient({baseUrl:"https://license.example.com",appKey:"app_ebox",fetch:async(url)=>url.endsWith("/download")?new Response(bytes):Response.json({success:false,error:{code:"UPDATES_EXPIRED",message:"expired during transfer"}},{status:403})});
 await assert.rejects(client.downloadUpdate(release,{},expected),e=>isAPIErrorCode(e,"UPDATES_EXPIRED"));
 });

test("signed targets cannot cross client platforms, channels or source version ranges",()=>{
 const target={os:"linux",arch:"amd64",runtime:"node",channel:"stable",kind:"archive",min_version:"1.0.0",max_version:"1.9.9"};
 const raw=Buffer.from(JSON.stringify({...claims,target}));const platformRelease={...release,target,manifest_token:raw.toString("base64url")+"."+sign("RSA-SHA256",raw,privateKey).toString("base64url")};
 const context={...expected,target,currentVersion:"1.0.0"};
 assert.equal(verifyReleaseManifest(platformRelease,context).target.os,"linux");
 assert.throws(()=>verifyReleaseManifest(platformRelease,{...context,target:{...target,os:"windows"}}),/platform/);
 assert.throws(()=>verifyReleaseManifest(platformRelease,{...context,currentVersion:"0.9.0"}),/upgrade path/);
 assert.throws(()=>verifyReleaseManifest({...platformRelease,target:{...target,channel:"beta"}},context),/signed manifest/);
});

test("install authorization requires explicit approval for the exact release",async()=>{
 for(const data of [{authorized:false,release_id:release.id},{authorized:true,release_id:"other"},{}]){
  const client=new LicenseClient({baseUrl:"https://license.example.com",appKey:"app_ebox",fetch:async()=>Response.json({success:true,data})});
  await assert.rejects(client.authorizeUpdate(release.id,{}),/authorization was not granted/);
 }
});
