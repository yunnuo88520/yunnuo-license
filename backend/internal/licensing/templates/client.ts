// Standard HTTP and JOSE. Node >=22.18 can run this TypeScript file directly.
// npm install && YN_CARD=... node client.ts activate
import type { KeyObject } from "node:crypto";
import {
  generateKeyPairSync,
  createPrivateKey,
  createPublicKey,
  createHash,
  randomBytes,
  sign,
} from "node:crypto";
import {
  mkdir,
  readFile,
  writeFile,
  rename,
  open,
  unlink,
} from "node:fs/promises";
import { join } from "node:path";
import { domainToASCII } from "node:url";
import { createLocalJWKSet, jwtVerify, errors } from "jose";
const APP_ID = "@@APP_ID@@",
  ISSUER = "urn:yunnuo:licensing:v2";
const BASE = (process.env.YN_LICENSE_URL || "@@LICENSE_URL@@").replace(
  /\/$/,
  "",
);
const STATE = join(process.env.YN_STATE_DIR || "./license-state", APP_ID);
await mkdir(STATE, { recursive: true, mode: 0o700 });
const sha = (value: string | Uint8Array) =>
  createHash("sha256").update(value).digest("hex");
async function load(name: string, fallback: any): Promise<any> {
  try {
    return JSON.parse(await readFile(join(STATE, name), "utf8"));
  } catch (e: any) {
    if (e.code === "ENOENT") return fallback;
    throw e;
  }
}
async function save(name: string, value: any) {
  const path = join(STATE, name),
    tmp = path + "." + randomBytes(8).toString("hex");
  await writeFile(tmp, JSON.stringify(value), { mode: 0o600, flag: "wx" });
  await rename(tmp, path);
}
async function discardTokens() {
  const cache = await load("cache.json", {});
  const identity: any = {};
  for (const key of ["activation_id", "license_id", "highwater"]) {
    if (cache[key]) identity[key] = cache[key];
  }
  await save("cache.json", identity);
}

let privateKey: KeyObject;
try {
  privateKey = createPrivateKey(
    await readFile(join(STATE, "installation.pem")),
  );
} catch (e: any) {
  if (e.code !== "ENOENT") throw e;
  const generated = generateKeyPairSync("ed25519").privateKey;
  try {
    await writeFile(
      join(STATE, "installation.pem"),
      generated.export({ format: "pem", type: "pkcs8" }),
      { mode: 0o600, flag: "wx" },
    );
    privateKey = generated;
  } catch (e: any) {
    if (e.code !== "EEXIST") throw e;
    privateKey = createPrivateKey(
      await readFile(join(STATE, "installation.pem")),
    );
  }
}
const publicRaw = createPublicKey(privateKey)
  .export({ format: "der", type: "spki" })
  .subarray(-32);
const PUBLIC = publicRaw.toString("base64url"),
  PUBLIC_HASH = sha(publicRaw);
const KIND = process.env.YN_BINDING_TYPE || "@@DEFAULT_BINDING@@",
  FORM = process.env.YN_FORM || "@@DEFAULT_FORM@@";
async function binding() {
  let value =
    process.env.YN_BINDING_VALUE ||
    (KIND === "installation" ? PUBLIC_HASH : "");
  if (KIND === "seat") value = "seat";
  if (KIND === "domain") {
    value = domainToASCII(
      new URL(value.includes("://") ? value : "https://" + value).hostname
        .toLowerCase()
        .replace(/\.$/, ""),
    );
  }
  if (KIND === "ip") {
    const parsed = new URL(
      "http://" + (value.includes(":") ? "[" + value + "]" : value),
    );
    value = parsed.hostname.replace(/^\[|\]$/g, "");
  }
  if (KIND === "machine_id") {
    if (process.env.YN_HOST_FINGERPRINT_FILE)
      value = (
        await readFile(process.env.YN_HOST_FINGERPRINT_FILE, "utf8")
      ).trim();
    else {
      try {
        await readFile("/.dockerenv");
        throw new Error(
          "Container machine binding requires a mounted host fingerprint file and persistent state volume; use seat for short-lived containers",
        );
      } catch (e: any) {
        if (e.code !== "ENOENT") throw e;
      }
      if (["container", "cluster"].includes(FORM) || process.env.KUBERNETES_SERVICE_HOST)
        throw new Error("Do not fingerprint a Kubernetes container");
      if (!value) {
        for (const source of [
          "/sys/class/dmi/id/product_uuid",
          "/etc/machine-id",
        ]) {
          try {
            const raw = (await readFile(source, "utf8")).trim();
            if (raw) {
              value = sha(raw);
              break;
            }
          } catch (e: any) {
            if (!["ENOENT", "EACCES"].includes(e.code)) throw e;
          }
        }
      }
    }
    value = value.toLowerCase();
    if (!/^[a-f0-9]{64}$/.test(value))
      throw new Error(
        "A real SHA-256 hardware fingerprint is required; no random fallback",
      );
  }
  if (!value) throw new Error("Set YN_BINDING_VALUE");
  return { type: KIND, value };
}
const BINDING = await binding(),
  LOCAL_HASH = sha(
    KIND + "\0" + (KIND === "seat" ? PUBLIC_HASH : BINDING.value),
  );
class Refusal extends Error {}
function trustedTime(cache: any) {
  const now = Math.floor(Date.now() / 1000);
  if (now + 120 < (cache.highwater || 0))
    throw new Error("Clock moved backwards; online validation required");
  return Math.max(now, cache.highwater || 0);
}
async function keys(force = false) {
  const cached = await load("jwks.json", null);
  if (cached && !force) return cached;
  const response = await fetch(
    BASE + "/v2/applications/" + APP_ID + "/.well-known/jwks.json",
    { signal: AbortSignal.timeout(10000) },
  );
  if (!response.ok) throw new Refusal("Public key endpoint refused request");
  const value = await response.json();
  await save("jwks.json", value);
  return value;
}
async function localVerify(
  token: string,
  purpose: string,
  activation: string,
  cache: any = {},
) {
  const options = {
    algorithms: ["RS256"],
    issuer: ISSUER,
    audience: APP_ID,
    requiredClaims: [
      "exp",
      "iat",
      "nbf",
      "sub",
      "jti",
      "token_use",
      "binding",
      "activation_id",
    ],
    currentDate: new Date(trustedTime(cache) * 1000),
  };
  let payload: any;
  try {
    ({ payload } = await jwtVerify(
      token,
      createLocalJWKSet(await keys()),
      options,
    ));
  } catch (e: any) {
    if (!(e instanceof errors.JWKSNoMatchingKey)) throw e;
    ({ payload } = await jwtVerify(
      token,
      createLocalJWKSet(await keys(true)),
      options,
    ));
  }
  if (
    payload.token_use !== purpose ||
    payload.activation_id !== activation ||
    payload.binding?.type !== KIND ||
    payload.binding?.hash !== LOCAL_HASH
  )
    throw new Error("JWT purpose, installation or binding mismatch");
  return payload;
}
async function send(path: string, payload: any) {
  const body = JSON.stringify(payload),
    stamp = Math.floor(Date.now() / 1000).toString(),
    nonce = randomBytes(24).toString("base64url");
  const message = [
    "YN-AUTH-V2",
    "POST",
    path,
    APP_ID,
    stamp,
    nonce,
    sha(body),
  ].join("\n");
  const signature = sign(null, Buffer.from(message), privateKey).toString(
    "base64url",
  );
  const response = await fetch(BASE + path, {
    method: "POST",
    body,
    headers: {
      "Content-Type": "application/json",
      "X-YN-Timestamp": stamp,
      "X-YN-Nonce": nonce,
      "X-YN-Signature": signature,
    },
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) {
    await discardTokens();
    throw new Refusal(
      "Authorization server refused: " + (await response.text()),
    );
  }
  const envelope = await response.json();
  if (!envelope.success) {
    await discardTokens();
    throw new Refusal("Invalid server response");
  }
  return envelope.data;
}
async function main() {
  const operation = process.argv[2] || "verify";
  let cache = await load("cache.json", {});
  trustedTime(cache);
  if (operation === "local") {
    const claims = await localVerify(
      cache.jwt,
      "license",
      cache.activation_id,
      cache,
    );
    console.log(JSON.stringify({ valid: true, license_id: claims.sub }));
    return;
  }
  if (cache.jwt) {
    try {
      await localVerify(cache.jwt, "license", cache.activation_id, cache);
    } catch (e: any) {
      if (!(e instanceof errors.JWTExpired)) {
        await discardTokens();
        cache = await load("cache.json", {});
      }
    }
  }
  if (operation === "request") {
    const result = await send("/v2/installations/request", {
      app_id: APP_ID,
      public_key: PUBLIC,
      binding: BINDING,
      label: process.env.YN_INSTALLATION_LABEL || "",
    });
    console.log(JSON.stringify(result));
    return;
  }
  const activating = operation === "activate" || !cache.activation_id;
  let path: string, payload: any;
  if (activating) {
    path = "/v2/activate";
    payload = {
      app_id: APP_ID,
      public_key: PUBLIC,
      binding: BINDING,
      form: FORM,
      label: process.env.YN_INSTALLATION_LABEL || "",
    };
    if (process.env.YN_CARD) payload.card_code = process.env.YN_CARD;
    else if (process.env.YN_INVITATION_ID) {
      payload.invitation_id = process.env.YN_INVITATION_ID;
      payload.invitation_token = process.env.YN_INVITATION_TOKEN;
    } else payload.pending_id = process.env.YN_PENDING_ID || "";
    if (process.env.YN_BORROW_SECONDS)
      payload.borrow_seconds = Number(process.env.YN_BORROW_SECONDS);
  } else {
    path = "/v2/verify";
    payload = {
      app_id: APP_ID,
      activation_id: cache.activation_id,
      binding: BINDING,
      form: FORM,
    };
    if (["release", "consume", "renew"].includes(operation))
      payload.action = operation;
    if (operation === "consume") {
      payload.units = Number(process.env.YN_UNITS || "1");
      payload.idempotency_key = process.env.YN_OPERATION_ID;
    }
    if (operation === "renew")
      payload.renewal_card = process.env.YN_RENEWAL_CARD;
  }
  let result;
  try {
    result = await send(path, payload);
  } catch (e: any) {
    const network =
      (e instanceof TypeError &&
        e.message === "fetch failed" &&
        [
          "ECONNREFUSED",
          "ECONNRESET",
          "ENOTFOUND",
          "EAI_AGAIN",
          "UND_ERR_CONNECT_TIMEOUT",
          "UND_ERR_SOCKET",
        ].includes((e.cause as any)?.code)) ||
      e.name === "TimeoutError";
    if (!network || activating || operation !== "verify" || !cache.offline_jwt)
      throw e;
    const claims = await localVerify(
      cache.offline_jwt,
      "offline_grace",
      cache.activation_id,
      cache,
    );
    await save("cache.json", { ...cache, highwater: trustedTime(cache) });
    console.log(
      JSON.stringify({ valid: true, offline: true, grace_until: claims.exp }),
    );
    return;
  }
  if (result.released) {
    await discardTokens();
    console.log(JSON.stringify(result));
    return;
  }
  await localVerify(result.jwt, "license", result.activation_id);
  result.highwater = Math.max(
    Math.floor(Date.now() / 1000),
    Math.floor(Date.parse(result.server_time) / 1000),
  );
  await save("cache.json", result);
  console.log(
    JSON.stringify({
      valid: true,
      license_id: result.license_id,
      activation_id: result.activation_id,
      remaining_units: result.remaining_units,
    }),
  );
}
await main();
