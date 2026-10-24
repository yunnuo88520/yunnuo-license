import { createServer } from "node:http";
import { readFile, realpath, mkdtemp, writeFile, rm } from "node:fs/promises";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { execFile } from "node:child_process";
import { LicenseClient } from "../../sdk/node/src/index.js";

const execute = promisify(execFile);
const config = JSON.parse(await readFile(process.env.YN_DEMO_CONFIG, "utf8"));
const dataDir = process.env.YN_DEMO_DATA_DIR;
if (!dataDir) throw new Error("YN_DEMO_DATA_DIR is required");
const port = Number(process.env.YN_DEMO_PORT || 18086);
const origin = `http://127.0.0.1:${port}`;
const installer = fileURLToPath(new URL("./installer.py", import.meta.url));
const client = new LicenseClient({ baseUrl: config.baseUrl, appKey: config.appKey, timeoutMs: 30_000 });
const events = [];
let updating = false;

async function install(action, ...args) {
  const result = await execute(process.env.YN_DEMO_PYTHON || "python3", [installer, action, dataDir, ...args], { timeout: 30_000 });
  return JSON.parse(result.stdout);
}

// Recover an incomplete transaction before exposing the demo again.
await install("rollback");

async function currentRelease() {
  const path = await realpath(join(dataDir, "current"));
  return { path, metadata: JSON.parse(await readFile(join(path, "release.json"), "utf8")) };
}

function json(response, status, body) {
  response.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
  response.end(JSON.stringify(body));
}

async function update() {
  const before = await currentRelease();
  events.push(`检查 v${before.metadata.version} 的更新与当前授权`);
  const result = await client.checkUpdate({ ...config.binding, current_version: before.metadata.version });
  if (!result.update_available) return { updated: false, message: "当前已是最新版本。" };
  const release = result.release;
  const bytes = await client.downloadUpdate(release, config.binding, { publicKeyPem: config.publicKeyPem, productId: config.productId });
  events.push(`v${release.version} 升级包签名、产品归属与 SHA-256 校验通过`);
  const staging = await mkdtemp(join(dataDir, "staging-"));
  let applied = false;
  try {
    const archive = join(staging, "package.zip");
    await writeFile(archive, bytes, { mode: 0o600, flag: "wx" });
    await install("apply", archive, release.version);
    applied = true;
    events.push(`已备份数据库并切换网站文件至 v${release.version}`);
    const health = await fetch(origin + "/api/health", { signal: AbortSignal.timeout(5_000) });
    const body = await health.json();
    if (!health.ok || body.version !== release.version || !body.healthy) throw new Error("新版健康检查未通过");
    await install("commit");
    events.push(`v${release.version} 健康检查通过，升级完成`);
    return { updated: true, message: `已成功升级至 v${release.version}，客户数据已保留。` };
  } catch (error) {
    if (applied) {
      await install("rollback");
      events.push(`新版检查失败，网站文件与数据库已回滚至 v${before.metadata.version}`);
    }
    throw error;
  } finally {
    await rm(staging, { recursive: true, force: true });
  }
}

const server = createServer(async (request, response) => {
  try {
    if (request.url === "/api/status" && request.method === "GET") {
      const { metadata } = await currentRelease();
      return json(response, 200, { version: metadata.version, ...(await install("status")), events });
    }
    if (request.url === "/api/health" && request.method === "GET") {
      const { metadata } = await currentRelease();
      const state = await install("status");
      const healthy = metadata.healthy === true && metadata.schema_version === state.schema_version;
      return json(response, healthy ? 200 : 503, { healthy, version: metadata.version });
    }
    if (request.url === "/api/update" && request.method === "POST") {
      if (request.headers.origin && request.headers.origin !== origin) return json(response, 403, { message: "来源不受信任" });
      if (request.headers["content-type"] !== "application/json") return json(response, 415, { message: "需要 JSON 请求" });
      if (updating) return json(response, 409, { message: "正在升级，请稍后重试" });
      request.resume();
      updating = true;
      try {
        return json(response, 200, await update());
      } catch (error) {
        events.push(`更新未完成：${error.code || error.message}`);
        return json(response, error.httpStatus || 500, { updated: false, code: error.code || "UPDATE_FAILED", message: `${error.code || error.message}；当前网站版本已保留。` });
      } finally {
        updating = false;
      }
    }
    if (request.url === "/" && request.method === "GET") {
      const { path } = await currentRelease();
      response.writeHead(200, { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" });
      return response.end(await readFile(join(path, "index.html")));
    }
    response.writeHead(404); response.end();
  } catch {
    json(response, 500, { message: "Demo 服务错误" });
  }
});
server.listen(port, "127.0.0.1", () => console.log("Ebox update demo is running on loopback"));
