<script setup lang="ts">
import { onMounted, reactive, ref, watch } from "vue";
import { formatTime } from "../shared/format";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  products: any[];
  canWrite: boolean;
}>();
const product = ref(""),
  cases = ref<any[]>([]),
  appeals = ref<any[]>([]),
  selected = ref<any>(),
  error = ref(""),
  message = ref(""),
  busy = ref(false),
  filter = ref("");
const actions: Record<string, string> = {
  observe: "仅记录观察",
  deny_updates: "禁止更新，保留使用",
  deny_use: "禁止使用与更新",
  block_page: "应用端显示拦截页",
};
const statuses: Record<string, string> = {
  open: "待核实",
  confirmed: "已确认处置",
  resolved: "已解除",
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
  const [c, a] = await Promise.all([
    props.api<any[]>("/admin/security/cases"),
    props.api<any[]>("/admin/security/appeals"),
  ]);
  cases.value = c;
  appeals.value = a;
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
          ><label class="field"
            >拦截提示文字<textarea
              v-model="policy.message"
              maxlength="1000"
              required
            ></textarea></label
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
          </div>
        </form>
      </fieldset>
    </section>
    <section class="panel">
      <h3>线索与处置记录</h3>
      <label class="field"
        >线索状态<select v-model="filter">
          <option value="">全部</option>
          <option v-for="(name, id) in statuses" :value="id">{{ name }}</option>
        </select></label
      >
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>应用 / 授权号</th>
              <th>线索</th>
              <th>状态 / 动作</th>
              <th>次数 / 最近时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="c in cases.filter(
                (c) =>
                  (!product || c.product_id === product) &&
                  (!filter || c.status === filter),
              )"
              :key="c.id"
            >
              <td>
                {{ products.find((p) => p.id === c.product_id)?.name
                }}<code>{{ c.license_no }}</code>
              </td>
              <td>
                {{
                  c.signal === "binding_mismatch"
                    ? "绑定不匹配"
                    : "客户端完整性报告"
                }}<code>{{ c.subject }}</code>
              </td>
              <td>{{ statuses[c.status] }}<br />{{ actions[c.action] }}</td>
              <td>{{ c.occurrences }} 次<br />{{ formatTime(c.last_seen) }}</td>
              <td>
                <button class="btn secondary small" @click="choose(c)">
                  查看与核实
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="!cases.length">暂无检测线索。</p>
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
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>处置编号</th>
              <th>用户 / 时间</th>
              <th>申诉内容</th>
              <th>状态</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="a in appeals" :key="a.id">
              <td>
                <code>{{ a.case_id }}</code>
              </td>
              <td>{{ a.user_id }}<br />{{ formatTime(a.created_at) }}</td>
              <td>{{ a.reason }}</td>
              <td>{{ a.status === "open" ? "待处理" : "已随处置解除关闭" }}</td>
            </tr>
          </tbody>
        </table>
      </div>
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
</style>
