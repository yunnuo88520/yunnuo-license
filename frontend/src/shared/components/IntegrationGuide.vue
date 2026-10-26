<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { BookOpen, Copy, Download, Terminal } from "lucide-vue-next";
import AppCredentials from "./AppCredentials.vue";

const props = withDefaults(
  defineProps<{
    product: any;
    prefix: "/admin" | "/developer";
    token: string;
    initialSecret?: string;
    canManageCredentials?: boolean;
  }>(),
  { canManageCredentials: true },
);
const emit = defineEmits<{ secretConsumed: [] }>();
const language = ref("node"),
  tab = ref("start"),
  baseURL = ref(location.origin),
  binding = ref(""),
  busy = ref(false),
  error = ref(""),
  message = ref("");
const languages: Record<string, string> = {
  node: "Node.js",
  go: "Go",
  java: "Java",
};
const bindingLabels: Record<string, string> = {
  domain: "域名",
  ip: "IP",
  device: "设备标识",
  account: "应用账号",
  none: "无绑定",
};
watch(
  () => props.product.id,
  () => {
    binding.value =
      (
        {
          domain: "app.example.com",
          ip: "203.0.113.10",
          device: "demo-device-001",
          account: "demo-account-001",
          none: "",
        } as Record<string, string>
      )[props.product.bind_mode] || "";
    error.value = "";
    message.value = "";
    tab.value = "start";
  },
  { immediate: true },
);
const name = computed(() => languages[language.value]);
function command(args: string) {
  return language.value === "java"
    ? `mvn -q compile exec:java -Dexec.args="${args}"`
    : (language.value === "node" ? "node app.mjs " : "go run . ") + args;
}
const commands = computed(() =>
  ["activate CARD_CODE", "verify", "heartbeat", "serve"]
    .map(command)
    .join("\n"),
);
const activate = computed(() => ({
  app_key: props.product.id,
  card_code: "替换为本应用的未使用卡密",
  bind_mode: props.product.bind_mode,
  bind_value: binding.value,
}));
const verify = computed(() => ({
  app_key: props.product.id,
  license_no: "替换为激活返回的完整授权号",
  bind_mode: props.product.bind_mode,
  bind_value: binding.value,
}));
const pretty = (value: any) => JSON.stringify(value, null, 2);
function curl(path: string, file: string) {
  let origin: string;
  try {
    origin = validatedURL();
  } catch {
    origin = location.origin;
  }
  const quoted = "'" + (origin + path).replaceAll("'", "'\"'\"'") + "'";
  return `curl --fail-with-body --silent --show-error ${quoted} -H 'Content-Type: application/json' --data-binary @${file}`;
}
function validatedURL() {
  const url = new URL(baseURL.value);
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    !["", "/"].includes(url.pathname)
  )
    throw Error(
      "授权站地址只填写根地址，例如 https://license.example.com，不含 /admin 或查询参数。",
    );
  return url.origin;
}
async function download() {
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    const params = new URLSearchParams({
      language: language.value,
      base_url: validatedURL(),
      bind_value: binding.value.trim(),
    });
    const response = await fetch(
      `${props.prefix}/products/${encodeURIComponent(props.product.id)}/integration-kit?${params}`,
      { headers: { Authorization: `Bearer ${props.token}` } },
    );
    if (!response.ok) {
      const result = await response.json();
      throw Error(
        result.error?.code === "INTEGRATION_ASSETS_UNAVAILABLE"
          ? "服务器缺少 SDK 接入资源，请联系管理员检查 YN_SDK_DIR。"
          : `接入包下载失败：${result.error?.code || response.status}`,
      );
    }
    const url = URL.createObjectURL(await response.blob());
    const a = document.createElement("a");
    a.href = url;
    a.download = `${props.product.id}-${language.value}-integration.zip`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    message.value = "接入包已开始下载，解压后按下方命令运行。";
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function copy(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    message.value = "已复制。";
  } catch {
    error.value = "复制失败，可直接选中文本复制。";
  }
}
</script>
<template>
  <section class="integration-guide" aria-label="应用授权接入指南">
    <header class="guide-heading">
      <BookOpen :size="24" />
      <div>
        <h2>接入授权 · {{ product.name }}</h2>
        <p>从创建应用到受保护接口，按当前应用生成可运行的接入包。</p>
      </div>
    </header>
    <div class="guide-credentials">
      <div>
        <small>AppID（系统自动生成，标准 18 位）</small
        ><code>{{ product.id }}</code
        ><button
          type="button"
          class="btn secondary small"
          @click="copy(product.id)"
        >
          <Copy :size="13" />复制应用 ID
        </button>
        <p>
          格式为 yn + 16 位十六进制字符，例如
          yn0123456789abcdef。后台选择应用会自动带入，用户激活由卡密识别。
        </p>
      </div>
      <div>
        <small>授权规则</small
        ><strong
          >{{ bindingLabels[product.bind_mode] }}绑定 ·
          {{
            product.license_mode === "perpetual"
              ? "永久使用、限期更新"
              : "订阅使用"
          }}</strong
        >
      </div>
    </div>
    <AppCredentials
      v-if="canManageCredentials !== false"
      :key="product.id"
      :product-id="product.id"
      :prefix="prefix"
      :token="token"
      :initial-secret="initialSecret"
      @consumed="emit('secretConsumed')"
    />
    <nav class="tabbar" aria-label="接入文档分类">
      <button
        v-for="item in [
          { id: 'start', label: '快速接入' },
          { id: 'http', label: 'HTTP 接口' },
          { id: 'policy', label: '上线与到期处理' },
          { id: 'updates', label: '可选在线升级' },
          { id: 'server', label: 'AppSecret 服务端接入' },
        ]"
        type="button"
        :class="{ active: tab === item.id }"
        @click="tab = item.id"
      >
        {{ item.label }}
      </button>
    </nav>
    <p v-if="error" class="guide-error" role="alert">{{ error }}</p>
    <p v-if="message" class="guide-message" role="status">{{ message }}</p>
    <template v-if="tab === 'start'">
      <div class="guide-setup">
        <label class="field"
          >接入语言<select v-model="language">
            <option value="node">Node.js 18+</option>
            <option value="go">Go 1.22+</option>
            <option value="java">Java 17+ / Maven 3</option>
          </select></label
        ><label class="field"
          >授权站根地址<input
            v-model="baseURL"
            type="url"
            placeholder="https://license.example.com" /></label
        ><label v-if="product.bind_mode !== 'none'" class="field"
          >客户端实际绑定值<input
            v-model="binding"
            maxlength="500"
            :placeholder="bindingLabels[product.bind_mode]"
        /></label>
      </div>
      <p>
        上面的示例绑定值只用于联调，请改为真实部署值。域名不含协议和路径；设备标识应稳定；应用账号由产品服务端确定。所有请求必须使用同一绑定值。
      </p>
      <ol class="guide-steps">
        <li>
          <h3>下载当前应用的接入包</h3>
          <p>
            包内包含配置、SDK 源码、运行说明及带授权检查的 demo，不依赖本 SDK
            已发布到公共包仓库。Java 首次编译需联网下载 Maven 插件和 Jackson
            依赖。
          </p>
          <button
            class="btn"
            type="button"
            :disabled="busy || !token"
            @click="download"
          >
            <Download :size="16" />{{
              busy ? "正在生成接入包…" : `下载 ${name} 接入包`
            }}
          </button>
        </li>
        <li>
          <h3>生成一张本应用的测试卡</h3>
          <p v-if="prefix === '/developer'">
            在本页下方“授权发卡”生成测试卡，复制本次返回的完整卡密。
          </p>
          <p v-else>
            在“卡密 → 新增批次”选择
            {{ product.name }} 生成一张测试卡。接入包不会代你发卡或激活。
          </p>
          <p>
            若客户已在用户中心激活，可使用
            {{
              command("attach LICENSE_NO")
            }}，先校验其完整授权号与当前绑定。客户端激活不会自动认领用户中心的授权归属。
          </p>
        </li>
        <li>
          <h3>解压后，在接入包目录运行</h3>
          <p>
            将 CARD_CODE
            换成测试卡密。首次激活会创建实际授权，结果写入本机私有的
            .license.json；verify 校验使用资格，heartbeat 仅上报活跃状态。
          </p>
          <div class="guide-code-head">
            <Terminal :size="16" /><span>{{ name }} 命令</span
            ><button
              class="btn secondary small"
              type="button"
              @click="copy(commands)"
            >
              复制命令
            </button>
          </div>
          <pre data-example="commands"><code>{{commands}}</code></pre>
        </li>
        <li>
          <h3>验证业务入口确实受保护</h3>
          <p>
            serve 启动后，打开
            <code>http://127.0.0.1:3000/protected</code
            >。每次进入该接口都重新验证，仅在 can_use=true
            时返回业务内容。未激活、吊销、错误绑定和使用到期返回
            403；授权站不可用返回 503。
          </p>
          <p>
            把同样的检查接到真实应用的服务端中间件、后台入口或核心功能；测试“允许”和“拒绝”两个方向。Node.js
            示例同时接入可配置的授权拦截页。
          </p>
        </li>
      </ol>
      <details>
        <summary>产品验签公钥</summary>
        <pre><code>{{product.public_key_pem}}</code></pre>
        <p>接入包 config.json 包含当前及历史公钥。它不包含任何产品私钥。</p>
      </details>
    </template>
    <template v-else-if="tab === 'http'">
      <h3>请求地址和返回结构</h3>
      <p>
        产品客户端调用以下公开 API，不需要管理员或开发者登录 token。请求中的
        app_key 填上方的应用 ID；接入包已经自动配置。响应外层为
        success、data、request_id；SDK 已处理 HTTP 错误和外层解包。
      </p>
      <h4>1. 首次激活：POST /v1/licenses/activate</h4>
      <p>
        将以下内容保存为 activate.json，替换卡密后执行命令。macOS/Linux 使用
        curl，Windows PowerShell 请使用 curl.exe。
      </p>
      <p>这些联调文件含卡密或授权信息，使用后删除，不要提交版本库。</p>
      <pre data-example="activate"><code>{{pretty(activate)}}</code></pre>
      <pre
        data-example="activate-command"
      ><code>{{curl('/v1/licenses/activate','activate.json')}}</code></pre>
      <h4>2. 校验使用资格：POST /v1/licenses/verify</h4>
      <pre data-example="verify"><code>{{pretty(verify)}}</code></pre>
      <p>将上面的内容保存为 verify.json，填写激活返回的完整授权号后执行：</p>
      <pre
        data-example="verify-command"
      ><code>{{curl('/v1/licenses/verify','verify.json')}}</code></pre>
      <p>
        保存激活返回的完整
        license_no。公开验真页面的编号已脱敏，不能用于客户端校验。不要仅判断
        HTTP 200，业务必须确认 data.can_use === true。
      </p>
      <h4>成功返回的关键字段</h4>
      <pre><code>{{pretty({success:true,data:{license_no:'lic_…',license_mode:product.license_mode,can_use:true,can_update:true,usage_expired_at:product.license_mode==='perpetual'?null:'UTC 时间或 null',updates_expired_at:'UTC 时间或 null',license_token:'敏感凭证，请妥善保存',offline_token:'签名缓存凭证，非永久离线许可'},request_id:'排障请求号'})}}</code></pre>
      <h4>3. 心跳与续费</h4>
      <p>
        POST /v1/licenses/heartbeat 使用与 verify 相同的 app_key、license_no
        和绑定字段；accepted=true 只表示心跳被接收，不能替代 can_use 判断。POST
        /v1/licenses/renew 在这些字段基础上增加 renew_card_code。
      </p>
      <h4>4. 公钥环：GET /v1/products/keys?app_key=本应用ID</h4>
      <p>
        按 token 的 key_version 选择 keys 中对应的
        public_key_pem，轮换后同时保留历史版本。只从可信的授权站获取公钥；生产使用
        HTTPS。
      </p>
    </template>
    <template v-else-if="tab === 'policy'">
      <h3>使用和更新分别判断</h3>
      <table class="guide-table">
        <thead>
          <tr>
            <th>场景</th>
            <th>应用端处理</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>can_use=true，can_update=false</td>
            <td>允许使用当前版本，关闭升级入口。</td>
          </tr>
          <tr>
            <td>can_use=false，或验证明确拒绝</td>
            <td>拒绝受保护业务，引导续费或核实授权。</td>
          </tr>
          <tr>
            <td>网络超时、授权站不可达或 5xx</td>
            <td>接入包默认返回 503；不能把故障当作永久有效授权。</td>
          </tr>
        </tbody>
      </table>
      <h3>上线前实际测一遍</h3>
      <ul>
        <li>
          正确卡密激活、错误绑定拒绝、续费后恢复、旧绑定在合法换绑后失效。
        </li>
        <li>
          订阅到期停止使用；永久使用且更新到期仍可运行，但不能检查或下载新版本。宽限期不能延长更新资格。
        </li>
        <li>
          后台吊销/停用授权后，真实业务入口应拒绝访问。遇到
          LICENSE_EXPIRED、LICENSE_REVOKED、BINDING_MISMATCH 等 4xx
          时不得继续执行业务。
        </li>
        <li>
          授权站断网、超时和 5xx
          时按明确的可用性策略处理；按应用负载设置短期校验缓存和心跳，遵守接口限流与
          Retry-After。
        </li>
      </ul>
      <h3>离线不是自动放行</h3>
      <p>
        SDK
        返回离线缓存凭证，但不会自动持久化或启用离线容错。需要离线使用时，必须验签并检查产品、绑定、授权期限和
        offline_until，配合安全存储与时间高水位；未知 key_version
        时刷新可信公钥环。完整离线文件只适用设备绑定，并受有效期限制。离线时无法即时获知吊销、换绑及安全处置。
      </p>
      <h3>凭证与排障</h3>
      <p>
        应用 ID 和公钥可以随产品分发；管理员/开发者登录
        token、产品私钥和数据库密码不能放进产品。不要在日志或 URL
        中记录卡密、license_token、offline_token。SDK 错误保留 code 和
        request_id，可与平台操作日志核对。直接激活仅建立部署绑定，授权用户归属由用户中心激活或管理员分配。
      </p>
    </template>
    <template v-else-if="tab === 'updates'">
      <h3>授权接入不要求开启升级服务</h3>
      <p>
        需要在线更新时，先为当前应用启用升级服务，上传升级包并发布高于客户端的版本。客户端每次检查和下载都需要有效的更新资格。
      </p>
      <pre><code>node app.mjs updates 1.0.0
node app.mjs download 1.0.0</code></pre>
      <p>
        以上命令使用 Node.js 接入包。download 根据 release.key_version
        选择公钥，校验签名和 SHA-256 后保存
        update.zip。未知版本的公钥必须从可信授权站更新，不得跳过验证。
      </p>
      <h4>其他语言使用 HTTP 接口</h4>
      <p>
        POST /v1/updates/check 在校验字段基础上增加 current_version，读取
        enabled、update_available 和 release。下载使用 POST
        /v1/updates/{releaseID}/download，提交同一应用、授权号和绑定；不支持匿名
        GET。Go/Java
        快速接入包目前提供授权能力，升级下载、验签和安装需按这些接口集成。
      </p>
      <p>
        保存升级包不等于在线安装完成。真实应用还需要备份、文件切换、数据迁移、健康检查和失败回滚。可参考仓库
        <a
          href="https://github.com/yunnuo88520/yunnuo-license/tree/main/examples/update-demo"
          target="_blank"
          rel="noopener"
          >Ebox 升级 demo</a
        >。升级服务到期后，即使旧版本允许永久使用，下载也必须拒绝。
      </p>
    </template>
    <template v-else>
      <h3>AppID + AppSecret 换取访问令牌</h3>
      <p>
        仅在开发者自己的可信服务端执行。AppSecret
        不替代终端用户的卡密、授权号或绑定信息，也不能用于登录管理员、代理或用户后台。
      </p>
      <p>
        将以下内容保存为 server-token.json，把 secret 替换成本次保存的
        AppSecret，再执行命令。凭证文件应设置为仅当前账号可读，联调后删除，不要提交版本库。
      </p>
      <pre
        data-example="server-token"
      ><code>{{pretty({grant_type:'client_credential',appid:product.id,secret:'替换为32位AppSecret'})}}</code></pre>
      <pre
        data-example="server-token-command"
      ><code>{{curl('/v1/app/token','server-token.json')}}</code></pre>
      <p>
        返回
        data.access_token、token_type=Bearer、expires_in=7200。服务端缓存令牌并在到期前刷新；使用
        Authorization: Bearer ACCESS_TOKEN 请求 GET /v1/app/profile
        检查当前应用身份。
      </p>
      <h3>使用令牌校验本应用授权</h3>
      <p>
        POST /v1/app/licenses/verify 携带同一 Bearer 令牌，请求体为
        license_no、bind_mode、bind_value；应用身份由令牌确定，无需再填写
        app_key。仍须判断返回的 can_use 与 can_update。
      </p>
      <p>
        令牌只用于该应用的服务端接口，不能访问其他应用或平台后台。过期、重置
        AppSecret
        或停用应用后旧令牌失效。密钥放在服务端环境变量或安全配置中，不写进终端客户端、URL
        或日志。
      </p>
    </template>
  </section>
</template>
<style scoped>
.integration-guide {
  min-width: 0;
  padding: 24px;
  border: 1px solid var(--line);
  border-radius: var(--radius-panel, 12px);
  background: var(--paper);
  color: var(--ink);
  margin-top: 24px;
  scroll-margin-top: 95px;
}
.guide-heading {
  display: flex;
  gap: 14px;
  align-items: flex-start;
  margin-bottom: 22px;
}
.guide-heading > svg {
  flex: none;
  color: var(--green);
  margin-top: 5px;
}
h2 {
  margin: 0;
  font-size: 23px;
}
p,
li {
  line-height: 1.85;
  color: var(--muted);
  font-size: 13px;
}
p {
  margin: 10px 0;
}
h3 {
  font-size: 17px;
  margin: 24px 0 8px;
}
h4 {
  font-size: 14px;
  margin: 22px 0 10px;
}
.guide-credentials {
  display: grid;
  gap: 15px;
  padding: 18px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 9px;
  margin-bottom: 22px;
}
.guide-credentials small {
  display: block;
  color: var(--muted);
  margin-bottom: 6px;
}
.guide-credentials code,
.guide-credentials strong {
  display: block;
  font-size: 13px;
  overflow-wrap: anywhere;
}
.guide-credentials button {
  margin-top: 9px;
}
.guide-setup {
  display: grid;
  grid-template-columns: 1fr 2fr 2fr;
  gap: 16px;
  margin: 22px 0;
}
.guide-steps {
  padding-left: 22px;
}
.guide-steps > li {
  padding-left: 6px;
  margin: 22px 0;
}
.guide-steps h3 {
  color: var(--ink);
}
.guide-code-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-bottom: 0;
  border-radius: 8px 8px 0 0;
  color: var(--ink);
}
.guide-code-head button {
  margin-left: auto;
}
pre {
  max-width: 100%;
  min-width: 0;
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  padding: 17px;
  background: var(--field);
  border: 1px solid var(--line);
  border-radius: 8px;
  font-size: 12px;
  color: var(--ink);
  line-height: 1.85;
  margin: 0 0 15px;
}
.guide-code-head + pre {
  border-radius: 0 0 8px 8px;
}
code {
  overflow-wrap: anywhere;
}
.guide-message {
  background: var(--green-soft);
  color: var(--green);
  padding: 12px;
}
.guide-error {
  background: var(--red-soft);
  color: var(--red);
  padding: 12px;
}
.guide-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: 12px;
}
.guide-table th,
.guide-table td {
  border: 1px solid var(--line);
  padding: 12px;
  text-align: left;
  overflow-wrap: anywhere;
}
details {
  margin: 20px 0;
}
details pre {
  margin-top: 15px;
}
a {
  text-decoration: underline;
  color: var(--green);
}
@media (max-width: 700px) {
  .integration-guide {
    padding: 16px;
  }
  .guide-setup {
    grid-template-columns: minmax(0, 1fr);
  }
  h2 {
    font-size: 20px;
  }
  .guide-steps {
    padding-left: 18px;
  }
  .guide-table th,
  .guide-table td {
    padding: 8px;
  }
  .guide-code-head {
    flex-wrap: wrap;
  }
}
</style>
