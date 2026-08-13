import assert from "node:assert/strict";
import test from "node:test";

import { LicenseClient, isAPIErrorCode } from "../src/index.js";

test("online client injects app key and preserves API errors", async () => {
  const calls = [];
  const fetch = async (url, options) => {
    const body = JSON.parse(options.body);
    calls.push({ url, options, body });
    if (body.card_code === "bad") {
      return jsonResponse(
        { success: false, request_id: "req_error", error: { code: "CARD_INVALID", message: "card invalid" } },
        { status: 404 },
      );
    }
    return jsonResponse({
      success: true,
      request_id: "req_ok",
      data: { license_no: "lic_test", status: "active", server_time: "2026-08-12T03:00:00Z" },
    });
  };
  const client = new LicenseClient({ baseUrl: "https://license.example.com/", appKey: "app_test", fetch, userAgent: "product/2.0" });
  const result = await client.activate({ card_code: "YN-TEST", bind_mode: "device", bind_value: "machine-A", app_key: "cannot_override" });
  assert.equal(result.license_no, "lic_test");
  assert.equal(calls[0].url, "https://license.example.com/v1/licenses/activate");
  assert.equal(calls[0].body.app_key, "app_test");
  assert.equal(calls[0].options.headers["User-Agent"], "product/2.0");

  await assert.rejects(
    client.activate({ card_code: "bad", bind_mode: "device", bind_value: "machine-A" }),
    (error) => isAPIErrorCode(error, "CARD_INVALID") && error.httpStatus === 404 && error.requestId === "req_error",
  );
});

test("all lifecycle methods use their expected routes", async () => {
  const paths = [];
  const fetch = async (url, options) => {
    const path = new URL(url).pathname;
    const body = JSON.parse(options.body);
    assert.equal(body.app_key, "app_test");
    paths.push(path);
    const data = path.endsWith("heartbeat")
      ? { accepted: true, server_time: "2026-08-12T03:00:00Z" }
      : path.endsWith("unbind")
        ? { unbound: true, license_no: "lic_test", server_time: "2026-08-12T03:00:00Z" }
        : { license_no: "lic_test", status: "active", server_time: "2026-08-12T03:00:00Z" };
    return jsonResponse({ success: true, data });
  };
  const client = new LicenseClient({ baseUrl: "https://license.example.com", appKey: "app_test", fetch });
  const binding = { license_no: "lic_test", bind_mode: "device", bind_value: "machine-A" };
  await client.verify(binding);
  assert.equal((await client.heartbeat(binding)).accepted, true);
  await client.renew({ ...binding, renew_card_code: "YN-RENEW" });
  assert.equal((await client.unbind(binding)).unbound, true);
  assert.deepEqual(paths, [
    "/v1/licenses/verify",
    "/v1/licenses/heartbeat",
    "/v1/licenses/renew",
    "/v1/licenses/unbind",
  ]);
});

test("client enforces timeout, caller cancellation, and response limit", async () => {
  const hangingFetch = (_url, { signal }) => new Promise((_resolve, reject) => {
    if (signal.aborted) reject(signal.reason);
    else signal.addEventListener("abort", () => reject(signal.reason), { once: true });
  });
  const timeoutClient = new LicenseClient({ baseUrl: "https://license.example.com", appKey: "app_test", fetch: hangingFetch, timeoutMs: 10 });
  await assert.rejects(timeoutClient.heartbeat({}), /request failed/i);

  const abortController = new AbortController();
  abortController.abort(new DOMException("cancelled", "AbortError"));
  await assert.rejects(timeoutClient.verify({}, { signal: abortController.signal }), /request failed/i);

  const largeClient = new LicenseClient({
    baseUrl: "https://license.example.com",
    appKey: "app_test",
    fetch: async () => new Response("x", { headers: { "Content-Length": String(2 * 1024 * 1024 + 1) } }),
  });
  await assert.rejects(largeClient.verify({}), /exceeds 2 MiB/);
});

function jsonResponse(body, options = {}) {
  return new Response(JSON.stringify(body), {
    ...options,
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
  });
}
