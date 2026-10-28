<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { downloadText } from "../api";
const props = defineProps<{
  application: any;
  api: (path: string, options?: RequestInit) => Promise<any>;
}>();
const language = ref("typescript"),
  error = ref(""),
  kit = ref<any>(null),
  busy = ref(false),
  booting = ref(true);
const plans = ref<any[]>([]),
  planID = ref(""),
  bindingType = ref(""),
  form = ref("");
let revision = 0,
  applicationRevision = 0;
const selected = ref("overview");
const sections = [
  { id: "overview", name: "接入步骤" },
  { id: "public_key", name: "公钥与 JWKS" },
  { id: "activate", name: "激活" },
  { id: "verify", name: "在线校验" },
  { id: "local_verify", name: "本地验签" },
  { id: "deployment", name: "部署与离线" },
  { id: "updates", name: "产品更新" },
  { id: "sales", name: "销售回调" },
];
const languages = [
  { id: "go", name: "Go" },
  { id: "java", name: "Java" },
  { id: "python", name: "Python" },
  { id: "rust", name: "Rust" },
  { id: "typescript", name: "TypeScript / JavaScript" },
  { id: "http", name: "其他语言 / HTTP" },
];
const currentPlan = computed(() =>
  plans.value.find((p) => p.id === planID.value),
);
const bindings = computed<string[]>(
  () =>
    currentPlan.value?.terms.binding_types ||
    props.application.allowed_bindings,
);
const forms = computed<string[]>(
  () => currentPlan.value?.terms.forms || props.application.forms,
);
const bindingNames: Record<string, string> = {
  machine_id: "机器指纹",
  domain: "域名",
  ip: "IP",
  seat: "浮动席位",
  installation: "持久安装标识",
  account: "业务账号",
  tenant: "租户",
  cluster: "集群",
};
const jwks = computed(
  () =>
    `${location.origin}/v2/applications/${props.application.application_id}/.well-known/jwks.json`,
);
function resetChoices() {
  if (!bindings.value.includes(bindingType.value))
    bindingType.value = bindings.value[0] || "";
  if (!forms.value.includes(form.value)) form.value = forms.value[0] || "";
}
async function load() {
  if (booting.value) return;
  const ticket = ++revision;
  busy.value = true;
  error.value = "";
  try {
    const query = new URLSearchParams({
      language: language.value,
      plan_id: planID.value,
      binding_type: bindingType.value,
      form: form.value,
      base_url: location.origin,
    });
    const value = await props.api(
      `/v2/console/applications/${props.application.application_id}/integration?${query}`,
    );
    if (ticket === revision) kit.value = value;
  } catch (e: any) {
    if (ticket === revision) error.value = e.message;
  } finally {
    if (ticket === revision) busy.value = false;
  }
}
function download() {
  if (!kit.value) return;
  const bytes = Uint8Array.from(atob(kit.value.archive_base64), (c) =>
    c.charCodeAt(0),
  );
  const url = URL.createObjectURL(
    new Blob([bytes], { type: "application/zip" }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download =
    props.application.application_id +
    "-" +
    language.value +
    "-integration.zip";
  link.click();
  URL.revokeObjectURL(url);
}
watch(
  () => props.application.application_id,
  async () => {
    const ticket = ++applicationRevision;
    booting.value = true;
    kit.value = null;
    error.value = "";
    try {
      const value = await props.api(
        `/v2/console/applications/${props.application.application_id}/plans`,
      );
      if (ticket !== applicationRevision) return;
      plans.value = value;
      planID.value = plans.value.find((p) => p.enabled)?.id || "";
      resetChoices();
    } catch (e: any) {
      error.value = e.message;
    } finally {
      if (ticket === applicationRevision) {
        booting.value = false;
        await load();
      }
    }
  },
  { immediate: true },
);
watch(planID, () => {
  resetChoices();
  load();
});
watch([language, bindingType, form], load);
</script>
<template>
  <section class="panel integration-v2">
    <h3>产品接入文档</h3>
    <p>
      同一应用集成一次，客户使用相同发行包。授权卡密属于应用与套餐，不依赖产品语言或部署方式。
    </p>
    <div class="integration-meta">
      <label class="field"
        >公开 AppID<input :value="application.application_id" readonly /></label
      ><label class="field"
        >语言<select v-model="language" aria-label="语言">
          <option v-for="l in languages" :key="l.id" :value="l.id">
            {{ l.name }}
          </option>
        </select></label
      >
    </div>
    <div class="integration-meta">
      <label class="field"
        >接入套餐<select v-model="planID" aria-label="接入套餐">
          <option value="">按应用配置</option>
          <option v-for="p in plans" :key="p.id" :value="p.id">
            {{ p.name }}
          </option>
        </select></label
      ><label class="field"
        >授权绑定<select v-model="bindingType" aria-label="授权绑定">
          <option v-for="b in bindings" :key="b" :value="b">
            {{ bindingNames[b] || b }}
          </option>
        </select></label
      ><label class="field"
        >安装形态<select v-model="form" aria-label="安装形态">
          <option v-for="f in forms" :key="f" :value="f">{{ f }}</option>
        </select></label
      >
    </div>
    <p
      v-if="
        ['domain', 'ip', 'account', 'tenant', 'cluster', 'machine_id'].includes(
          bindingType,
        )
      "
      class="muted"
    >
      示例已选择 {{ bindingNames[bindingType] }} 绑定；客户安装时设置
      YN_BINDING_VALUE 为真实锚点。卡密、安装私钥和客户绑定值不写入通用发行包。
    </p>
    <p class="wrap">
      JWKS：<code>{{ jwks }}</code>
    </p>
    <p class="notice">
      AppSecret
      只供开发者自己控制的服务端使用。客户安装生成并持久保存独立安装私钥，发行包只包含
      AppID 和公钥来源。
    </p>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <template v-if="kit"
      ><label class="field"
        >安装依赖 / 运行命令<textarea
          :value="kit.install_command"
          readonly
          rows="3"
        ></textarea>
      </label>
      <nav class="code-tabs">
        <button
          v-for="s in sections"
          :key="s.id"
          :class="{ selected: selected === s.id }"
          @click="selected = s.id"
        >
          {{ s.name }}
        </button>
      </nav>
      <pre>{{ kit.snippets[selected] }}</pre>
      <div class="integration-actions">
        <button class="btn" :disabled="busy" @click="download">
          保存接入示例文件</button
        ><button
          class="btn secondary"
          @click="downloadText(kit.openapi, 'openapi-licensing-v2.json')"
        >
          下载 OpenAPI 契约
        </button>
      </div>
      <details>
        <summary>查看完整可运行示例</summary>
        <article v-for="f in kit.files" :key="f.name">
          <h4>{{ f.name }}</h4>
          <button
            class="btn secondary"
            @click="downloadText(f.content, f.name)"
          >
            保存文件
          </button>
          <pre>{{ f.content }}</pre>
        </article>
      </details></template
    >
  </section>
</template>
<style scoped>
.integration-v2 {
  min-width: 0;
  padding: 24px;
}
.integration-meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 16px;
}
.wrap {
  overflow-wrap: anywhere;
}
.code-tabs,
.integration-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 18px 0;
}
.code-tabs button {
  padding: 9px 12px;
  border: 1px solid var(--line);
  background: var(--surface, #fff);
  color: inherit;
  border-radius: 8px;
  cursor: pointer;
}
.code-tabs .selected {
  background: #087c64;
  color: #fff;
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font: 12px/1.8 monospace;
  background: var(--surface);
  padding: 18px;
  border-radius: 10px;
  max-height: 600px;
  overflow: auto;
}
textarea {
  font-family: monospace;
  width: 100%;
}
summary {
  cursor: pointer;
}
@media (max-width: 600px) {
  .integration-meta {
    grid-template-columns: 1fr;
  }
  .integration-v2 {
    padding: 16px;
  }
}
</style>
