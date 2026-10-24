<script setup lang="ts">
import PagedRecords from "../shared/components/PagedRecords.vue";
import { computed, onMounted, reactive, ref, watch } from "vue";
import { renderLicenseBlockPage } from "../../../sdk/node/src/web.js";
import { formatTime } from "../shared/format";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  products: any[];
  canWrite: boolean;
  token: string;
}>();
const product = ref(""),
  selected = ref<any>(),
  error = ref(""),
  message = ref(""),
  busy = ref(false);
const preview = ref(false),
  refreshKey = ref(0);
const actions: Record<string, string> = {
  observe: "仅记录观察",
  deny_updates: "禁止更新，保留使用",
  deny_use: "禁止使用与更新",
  block_page: "应用端显示拦截页",
};
const policy = reactive({
  enabled: false,
  detect_mismatch: false,
  accept_reports: false,
  action: "observe",
  message: "此应用的授权需要核实，请联系服务提供方。",
  support_url: "",
});
const review = reactive({ status: "confirmed", action: "observe", reason: "" });
const previewHTML = computed(() =>
  renderLicenseBlockPage({
    action: "block_page",
    message: policy.message,
    support_url: policy.support_url,
  }),
);
async function run(fn: () => Promise<any>) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await fn();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function load() {
  refreshKey.value++;
  if (!product.value) product.value = props.products[0]?.id || "";
  if (product.value)
    Object.assign(
      policy,
      await props.api(`/admin/products/${product.value}/security-policy`),
    );
}
async function save() {
  await props.api(`/admin/products/${product.value}/security-policy`, {
    method: "POST",
    body: JSON.stringify(policy),
  });
  message.value = "应用安全策略已保存。";
}
function choose(c: any) {
  selected.value = c;
  Object.assign(review, {
    status: c.status === "resolved" ? "resolved" : "confirmed",
    action: c.status === "open" ? policy.action : c.action,
    reason: "",
  });
}
async function submit() {
  await props.api(`/admin/security/cases/${selected.value.id}/review`, {
    method: "POST",
    body: JSON.stringify(review),
  });
  selected.value = null;
  await load();
  message.value = "安全处置已更新，操作已审计。";
}
watch(
  () => props.products,
  () => {
    if (!product.value) void run(load);
  },
);
onMounted(() => run(load));
</script>
<template>
  <div class="security-panels">
    <section class="section-intro">
      <div>
        <h2>盗版线索与安全处置</h2>
        <p>
          绑定异常和客户端完整性报告先进入待核实记录。确认前不改变授权权益；过期、断网不作为盗版线索。
        </p>
      </div>
      <button class="btn secondary" :disabled="busy" @click="run(load)">
        刷新安全记录
      </button>
    </section>
    <p v-if="error" class="error notice" role="alert">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>
    <section class="panel">
      <h3>应用安全策略</h3>
      <label class="field"
        >安全策略应用<select v-model="product" @change="run(load)">
          <option v-for="p in products" :value="p.id">{{ p.name }}</option>
        </select></label
      >
      <fieldset :disabled="!canWrite || busy || !product">
        <form class="form-grid" @submit.prevent="run(save)">
          <label class="check"
            ><input
              v-model="policy.enabled"
              type="checkbox"
            />启用检测与已确认处置</label
          ><label class="check"
            ><input
              v-model="policy.detect_mismatch"
              type="checkbox"
            />记录服务端绑定不匹配</label
          ><label class="check"
            ><input
              v-model="policy.accept_reports"
              type="checkbox"
            />接收客户端升级包完整性报告</label
          ><label class="field"
            >确认后的建议动作<select v-model="policy.action">
              <option v-for="(name, id) in actions" :value="id">
                {{ name }}
              </option>
            </select></label
          ><label class="field announcement-field"
            ><span id="security-announcement-label">打击盗版公告</span
            ><textarea
              v-model="policy.message"
              aria-labelledby="security-announcement-label"
              aria-describedby="security-announcement-help"
              maxlength="1000"
              rows="4"
              placeholder="例如：站点未授权，联系 QQ 531023977。"
              required
            ></textarea
            ><small id="security-announcement-help"
              >公告显示在拦截页的醒目区域，支持多行文字，可填写联系 QQ
              等处理说明。</small
            ></label
          ><label class="field"
            >服务联系链接<input
              v-model="policy.support_url"
              type="url"
              maxlength="500"
              placeholder="https://support.example.com"
          /></label>
          <p>
            建议动作仍须管理员逐条确认后生效。关闭策略后暂停该应用的检测及已确认处置；记录保留。拦截页需应用接入服务端中间件。
          </p>
          <div class="form-actions">
            <button class="btn">保存安全策略</button>
            <button
              type="button"
              class="btn secondary"
              :aria-expanded="preview"
              @click="preview = !preview"
            >
              {{ preview ? "收起拦截页预览" : "预览拦截页" }}
            </button>
          </div>
        </form>
      </fieldset>
      <section v-if="preview" class="announcement-preview">
        <h4>拦截页公告预览</h4>
        <p>公告即时预览，保存后应用于该产品的拦截页。</p>
        <iframe
          title="拦截页公告预览"
          :srcdoc="previewHTML"
          sandbox=""
        ></iframe>
      </section>
    </section>
    <section class="panel">
      <h3>线索与处置记录</h3>
      <PagedRecords
        :api="api"
        :token="token"
        endpoint="/admin/history/cases"
        kind="cases"
        :params="{ product_id: product }"
        :refresh="refreshKey"
        selectable
        @select="choose"
      />
      <div v-if="selected" class="review">
        <h4>核实记录 {{ selected.id }}</h4>
        <p class="evidence">{{ selected.evidence }}</p>
        <p v-if="selected.review_reason">
          上次说明：{{ selected.review_reason }}
        </p>
        <fieldset :disabled="!canWrite || busy">
          <form class="form-grid" @submit.prevent="run(submit)">
            <label class="field"
              >核实结论<select v-model="review.status">
                <option value="confirmed">确认并处置</option>
                <option value="resolved">解除 / 排除误报</option>
              </select></label
            ><label class="field"
              >本次处置动作<select
                v-model="review.action"
                :disabled="review.status === 'resolved'"
              >
                <option v-for="(name, id) in actions" :value="id">
                  {{ name }}
                </option>
              </select></label
            ><label class="field"
              >核实与处置原因<textarea
                v-model="review.reason"
                required
                maxlength="1000"
              ></textarea>
            </label>
            <p>
              客户端报告可能被伪造，请核对版本、真实部署和权属后再执行限制。
            </p>
            <div class="form-actions">
              <button class="btn">提交核实结论</button>
            </div>
          </form>
        </fieldset>
      </div>
    </section>
    <section class="panel">
      <h3>用户申诉</h3>
      <PagedRecords
        :api="api"
        :token="token"
        endpoint="/admin/history/appeals"
        kind="appeals"
        :params="{ product_id: product }"
        :refresh="refreshKey"
      />
    </section>
  </div>
</template>
<style scoped>
.security-panels {
  display: grid;
  gap: 24px;
}
.panel {
  padding: 26px;
}
fieldset {
  border: 0;
  padding: 0;
  margin: 0;
  min-width: 0;
}
form {
  margin: 24px 0;
}
.review {
  margin-top: 28px;
  border-top: 1px solid #d9e1dc;
  padding-top: 20px;
}
.evidence {
  overflow-wrap: anywhere;
  background: #f2f5f1;
  padding: 18px;
}
.notice {
  padding: 18px;
  background: #fae9e9;
}
.error {
  color: #9e3030;
}
textarea {
  min-height: 90px;
}
.table td {
  max-width: 420px;
  overflow-wrap: anywhere;
}
.announcement-field {
  grid-column: 1 / -1;
}
.announcement-field small,
.announcement-preview > p {
  color: #71806e;
  font-size: 12px;
  font-weight: 400;
}
.announcement-preview {
  margin-top: 24px;
  padding-top: 24px;
  border-top: 1px solid #e0e6dc;
}
.announcement-preview h4 {
  margin: 0 0 8px;
}
.announcement-preview iframe {
  display: block;
  width: 100%;
  height: 780px;
  margin-top: 18px;
  border: 1px solid #dce1d8;
  border-radius: 14px;
  background: #f4f5f1;
}
@media (max-width: 600px) {
  .announcement-preview iframe {
    height: 960px;
  }
}
</style>
