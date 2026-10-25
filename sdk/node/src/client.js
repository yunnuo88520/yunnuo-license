import { APIError } from "./errors.js";
import { OfflineVerifier } from "./offline.js";
import { verifyReleaseManifest, verifyReleasePackage } from "./updates.js";
import { createHash } from "node:crypto";

const MAX_RESPONSE_BYTES = 2 * 1024 * 1024;

export class LicenseClient {
  #baseUrl;
  #appKey;
  #fetch;
  #timeoutMs;
  #userAgent;
  #reportIntegrityFailures;

  constructor({ baseUrl, appKey, fetch: fetchImpl = globalThis.fetch, timeoutMs = 10_000, userAgent = "yn-license-node/1.0", reportIntegrityFailures = false }) {
    const normalizedBaseUrl = String(baseUrl ?? "").trim().replace(/\/+$/, "");
    const normalizedAppKey = String(appKey ?? "").trim();
    let parsed;
    try {
      parsed = new URL(normalizedBaseUrl);
    } catch {
      throw new TypeError("ynlicense: base URL and app key are required");
    }
    if (!['http:', 'https:'].includes(parsed.protocol) || !normalizedAppKey || typeof fetchImpl !== "function") {
      throw new TypeError("ynlicense: base URL, app key, and fetch implementation are required");
    }
    if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) {
      throw new TypeError("ynlicense: timeoutMs must be positive");
    }
    this.#baseUrl = normalizedBaseUrl;
    this.#appKey = normalizedAppKey;
    this.#fetch = fetchImpl;
    this.#timeoutMs = timeoutMs;
    this.#userAgent = String(userAgent || "yn-license-node/1.0").trim();
    this.#reportIntegrityFailures = reportIntegrityFailures === true;
  }

  activate(input, options) {
    return this.#post("/v1/licenses/activate", input, options);
  }

  verify(input, options) {
    return this.#post("/v1/licenses/verify", input, options);
  }

  heartbeat(input, options) {
    return this.#post("/v1/licenses/heartbeat", input, options);
  }

  renew(input, options) {
    return this.#post("/v1/licenses/renew", input, options);
  }

  unbind(input, options) {
    return this.#post("/v1/licenses/unbind", input, options);
  }

  checkUpdate(input, options) {
    return this.#post("/v1/updates/check", input, options);
  }

  securityDecision(input, options) { return this.#post("/v1/security/decision", input, options); }
  reportIntegrity(input, options) { return this.#post("/v1/security/report", input, options); }

  // productKeys 拉取产品公钥环（含历史版本），供密钥环离线验签器使用。
  productKeys(appKey, options) {
    return this.#get(`/v1/products/keys?app_key=${encodeURIComponent(String(appKey ?? this.#appKey))}`, options);
  }

  async #get(path, options = {}) {
    const timeoutController = new AbortController();
    const timeout = setTimeout(() => timeoutController.abort(new DOMException("Request timed out", "TimeoutError")), this.#timeoutMs);
    const signal = combineSignals(timeoutController.signal, options.signal);
    try {
      const response = await this.#fetch(`${this.#baseUrl}${path}`, {
        method: "GET",
        headers: { Accept: "application/json", "User-Agent": this.#userAgent },
        signal,
      });
      const raw = new TextDecoder().decode(await readLimitedResponse(response, MAX_RESPONSE_BYTES));
      let envelope;
      try {
        envelope = JSON.parse(raw);
      } catch (error) {
        throw new Error(`ynlicense: decode response (HTTP ${response.status}): ${error.message}`, { cause: error });
      }
      if (!response.ok || envelope?.success !== true) {
        throw new APIError({
          code: envelope?.error?.code || "HTTP_ERROR",
          message: envelope?.error?.message || response.statusText || "request failed",
          httpStatus: response.status,
          requestId: envelope?.request_id || "",
        });
      }
      return envelope.data;
    } catch (error) {
      if (error instanceof APIError || String(error?.message).startsWith("ynlicense:")) throw error;
      throw new Error(`ynlicense: request failed: ${error?.message || error}`, { cause: error });
    } finally {
      clearTimeout(timeout);
    }
  }

  async downloadUpdate(release, input, { publicKeyPem, productId, ...options }) {
    const expected = { publicKeyPem, productId };
    verifyReleaseManifest(release, expected);
    const bytes = await this.#post(`/v1/updates/${encodeURIComponent(release.id)}/download`, input, options, release.size);
    try {
      verifyReleasePackage(release, bytes, expected);
    } catch (error) {
      if (this.#reportIntegrityFailures) {
        try { await this.reportIntegrity({ ...input, release_id: release.id, observed_sha256: createHash("sha256").update(bytes).digest("hex") }, options); } catch { /* Preserve the original integrity failure. */ }
      }
      throw error;
    }
    return bytes;
  }

  async #post(path, input, options = {}, binarySize = 0) {
    const timeoutController = new AbortController();
    const timeout = setTimeout(() => timeoutController.abort(new DOMException("Request timed out", "TimeoutError")), this.#timeoutMs);
    const signal = combineSignals(timeoutController.signal, options.signal);
    try {
      const response = await this.#fetch(`${this.#baseUrl}${path}`, {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "User-Agent": this.#userAgent,
        },
        body: JSON.stringify({ ...(input ?? {}), app_key: this.#appKey }),
        signal,
      });
      const bytes = await readLimitedResponse(response, binarySize && response.ok ? binarySize : MAX_RESPONSE_BYTES);
      if (binarySize && response.ok) return bytes;
      const raw = new TextDecoder().decode(bytes);
      let envelope;
      try {
        envelope = JSON.parse(raw);
      } catch (error) {
        throw new Error(`ynlicense: decode response (HTTP ${response.status}): ${error.message}`, { cause: error });
      }
      if (!response.ok || envelope?.success !== true) {
        throw new APIError({
          code: envelope?.error?.code || "HTTP_ERROR",
          message: envelope?.error?.message || response.statusText || "request failed",
          httpStatus: response.status,
          requestId: envelope?.request_id || "",
        });
      }
      return envelope.data;
    } catch (error) {
      if (error instanceof APIError || String(error?.message).startsWith("ynlicense:")) throw error;
      throw new Error(`ynlicense: request failed: ${error?.message || error}`, { cause: error });
    } finally {
      clearTimeout(timeout);
    }
  }
}

async function readLimitedResponse(response, limit) {
  const declaredLength = Number(response.headers.get("content-length"));
  if (Number.isFinite(declaredLength) && declaredLength > limit) {
    throw new Error(`ynlicense: response exceeds ${limit === MAX_RESPONSE_BYTES ? '2 MiB' : limit + ' bytes'} limit`);
  }
  if (!response.body) return new Uint8Array();
  const reader = response.body.getReader();
  const chunks = [];
  let total = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      total += value.byteLength;
      if (total > limit) {
        await reader.cancel();
        throw new Error(`ynlicense: response exceeds ${limit === MAX_RESPONSE_BYTES ? '2 MiB' : limit + ' bytes'} limit`);
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  const buffer = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    buffer.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return buffer;
}

function combineSignals(timeoutSignal, callerSignal) {
  if (!callerSignal) return timeoutSignal;
  if (typeof AbortSignal.any === "function") {
    return AbortSignal.any([timeoutSignal, callerSignal]);
  }
  const controller = new AbortController();
  const abort = (signal) => controller.abort(signal.reason);
  if (timeoutSignal.aborted) abort(timeoutSignal);
  else timeoutSignal.addEventListener("abort", () => abort(timeoutSignal), { once: true });
  if (callerSignal.aborted) abort(callerSignal);
  else callerSignal.addEventListener("abort", () => abort(callerSignal), { once: true });
  return controller.signal;
}

// offlineVerifierFromKeyRing 拉取产品公钥环并构造按 key_version 选钥的离线验签器，
// 让密钥轮换后的新 token 仍能在已部署客户端上通过验证。
export async function offlineVerifierFromKeyRing(client, { appKey, productCode, ...options } = {}) {
  const key = String(appKey ?? "").trim();
  if (!key) throw new TypeError("ynlicense: appKey is required");
  const ring = await client.productKeys(key, options);
  const publicKeys = {};
  for (const entry of ring?.keys ?? []) {
    if (entry?.public_key_pem) publicKeys[entry.key_version] = entry.public_key_pem;
  }
  if (Object.keys(publicKeys).length === 0) {
    throw new Error("ynlicense: product key ring is empty");
  }
  return new OfflineVerifier({ publicKeys, productCode: productCode ?? ring.product_code, ...options });
}
