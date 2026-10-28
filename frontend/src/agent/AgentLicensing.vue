<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { downloadText } from "../shared/api";
import {
  formatTime,
  licenseStateLabel,
  updatePeriodLabel,
} from "../shared/format";
const props = defineProps<{
  api: (path: string, options?: RequestInit) => Promise<any>;
  section: string;
  canManage: boolean;
  profile: any;
}>();
const catalog = ref<any[]>([]),
  history = ref<any[]>([]),
  ledger = ref<any[]>([]),
  account = ref<any>(null),
  codes = ref<any[]>([]),
  error = ref(""),
  notice = ref(""),
  busy = ref(false),
  appID = ref("");
const form = reactive({
  plan_id: "",
  quantity: 1,
  customer_ref: "",
  purpose: "activate",
  request_id: crypto.randomUUID(),
});
const applications = computed(() => [
  ...new Map(
    catalog.value.map((q) => [q.application.application_id, q.application]),
  ).values(),
]);
const plans = computed(() =>
  catalog.value.filter((q) => q.application.application_id === appID.value),
);
const quote = computed(() =>
  plans.value.find((q) => q.plan.id === form.plan_id),
);
const orders = computed(() =>
  history.value.flatMap((h) =>
    h.orders.map((o: any) => ({ ...o, application_name: h.application.name })),
  ),
);
const licenses = computed(() =>
  history.value.flatMap((h) =>
    h.licenses.map((l: any) => ({
      ...l,
      application_name: h.application.name,
    })),
  ),
);
const transactions = computed(() =>
  [
    ...ledger.value,
    ...history.value.flatMap((h) =>
      h.money_events.map((e: any) => ({
        ...e,
        amount: e.amount_cents,
        balance_after: e.balance_after,
        remark: h.application.name,
      })),
    ),
  ].sort((a, b) => b.created_at.localeCompare(a.created_at)),
);
const money = (v: number) => `¥${((v || 0) / 100).toFixed(2)}`;
async function run(fn: () => Promise<void>) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await fn();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function refresh() {
  [catalog.value, history.value, ledger.value, account.value] =
    await Promise.all([
      props.api("/v2/agent/catalog"),
      props.api("/v2/agent/records"),
      props.api("/agent/money-ledgers"),
      props.api("/agent/account"),
    ]);
  if (!applications.value.some((a) => a.application_id === appID.value))
    appID.value = applications.value[0]?.application_id || "";
  if (!plans.value.some((q) => q.plan.id === form.plan_id))
    form.plan_id = plans.value[0]?.plan.id || "";
}
watch(appID, () => {
  form.plan_id = plans.value[0]?.plan.id || "";
  codes.value = [];
});
watch(
  () => [
    appID.value,
    form.plan_id,
    form.quantity,
    form.customer_ref,
    form.purpose,
  ],
  () => {
    form.request_id = crypto.randomUUID();
  },
);
async function issue() {
  if (!quote.value) return;
  const result = await props.api(
    `/v2/agent/applications/${appID.value}/orders`,
    {
      method: "POST",
      body: JSON.stringify({
        ...form,
        expected_unit_cost_cents: quote.value.unit_cost_cents,
      }),
    },
  );
  codes.value = result.cards || [];
  await refresh();
  notice.value = result.replayed
    ? "此订单已处理，未重复扣款。卡密原文只在首次成功时显示。"
    : "发卡成功，已按当前代理成本扣款。请立即导出本次卡密。";
  form.request_id = crypto.randomUUID();
}
function exportCodes() {
  downloadText(
    [
      "card_id,license_id,code",
      ...codes.value.map((c) => `${c.card_id},${c.license_id},${c.code}`),
    ].join("\n"),
    `${appID.value}-agent-cards.csv`,
  );
}
watch(
  () => props.section,
  () => {
    codes.value = [];
  },
);
defineExpose({ refresh: () => run(refresh) });
onMounted(() => run(refresh));
</script>
<template>
  <div class="agent-licensing">
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <div class="panel agent-summary">
      <div>
        <small>公开代理编号</small><strong>{{ profile.agent_no }}</strong
        ><a href="/" target="_blank" rel="noopener">前台代理验真</a>
      </div>
      <div>
        <small>可用余额</small><strong>{{ money(account?.balance) }}</strong
        ><small>充值入账由管理员确认</small>
      </div>
      <div>
        <small>可代理应用</small><strong>{{ applications.length }}</strong
        ><small>{{ catalog.length }} 个已获授权的套餐</small>
      </div>
      <button class="btn secondary" :disabled="busy" @click="run(refresh)">
        刷新业务记录
      </button>
    </div>
    <section v-if="section === 'overview'" class="panel">
      <h2>我的代理应用</h2>
      <p>
        使用现有用户账号的代理资格销售应用。可售套餐、单批数量与成本均由平台授予；无需维护另一套登录账号。
      </p>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>应用 / 套餐</th>
              <th>授权规格</th>
              <th>每份成本</th>
              <th>单批上限</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="q in catalog" :key="q.grant.id">
              <td>
                {{ q.application.name }}<small>{{ q.plan.name }}</small>
              </td>
              <td>
                {{
                  q.plan.terms.perpetual
                    ? "永久使用"
                    : q.plan.terms.duration_days + " 天"
                }}<small
                  >更新 {{ q.plan.terms.update_duration_days }} 天 ·
                  {{ q.plan.terms.model }}</small
                >
              </td>
              <td>{{ money(q.unit_cost_cents) }}</td>
              <td>{{ q.grant.max_batch_quantity }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="!catalog.length">
        暂无可销售套餐，请联系管理员开通应用代理权限。
      </p>
    </section>
    <section v-if="section === 'generate'" class="panel">
      <h2>生成应用卡密</h2>
      <p>
        同一卡密适用于套餐允许的 Web、Docker、GUI
        等形态。请确认成本后生成，余额扣款与授权签发在同一事务完成。
      </p>
      <form v-if="canManage" @submit.prevent="run(issue)">
        <div class="issue-grid">
          <label class="field"
            >选择应用<select v-model="appID" required aria-label="选择代理应用">
              <option value="" disabled>请选择应用</option>
              <option
                v-for="a in applications"
                :key="a.application_id"
                :value="a.application_id"
              >
                {{ a.name }}
              </option>
            </select></label
          ><label class="field"
            >可售套餐<select
              v-model="form.plan_id"
              required
              aria-label="可售套餐"
            >
              <option value="" disabled>请选择套餐</option>
              <option v-for="q in plans" :key="q.plan.id" :value="q.plan.id">
                {{ q.plan.name }} · {{ money(q.unit_cost_cents) }} / 份
              </option>
            </select></label
          ><label class="field"
            >发卡数量<input
              v-model.number="form.quantity"
              type="number"
              min="1"
              :max="quote?.grant.max_batch_quantity || 1"
              required /></label
          ><label class="field"
            >客户参考号<input
              v-model="form.customer_ref"
              maxlength="190"
              placeholder="客户或订单编号" /></label
          ><label class="field"
            >卡密用途<select v-model="form.purpose" aria-label="卡密用途">
              <option value="activate">新授权激活</option>
              <option value="renew_usage">仅续使用时间</option>
              <option value="renew_updates">仅续更新时间</option>
            </select></label
          >
        </div>
        <p v-if="quote">
          本次成本
          <strong>{{ money(quote.unit_cost_cents * form.quantity) }}</strong> ·
          当前余额 {{ money(account?.balance) }}
        </p>
        <button
          class="btn"
          :disabled="
            busy ||
            !quote ||
            form.quantity * quote.unit_cost_cents > (account?.balance || 0)
          "
        >
          确认成本并生成卡密
        </button>
      </form>
      <p v-else>当前代理成员没有发卡权限。</p>
      <div v-if="codes.length" class="codes">
        <h3>本次卡密（只显示一次）</h3>
        <p>原文不保存在服务器。刷新或离开本页后不能再次导出，请立即保存。</p>
        <textarea
          :value="codes.map((c) => c.code).join('\n')"
          readonly
          rows="5"
          aria-label="代理本次卡密"
        ></textarea>
        <div class="actions">
          <button class="btn" @click="exportCodes">导出本次卡密 CSV</button
          ><button class="btn secondary" @click="codes = []">
            已保存，隐藏卡密
          </button>
        </div>
      </div>
    </section>
    <section v-if="section === 'batches'" class="panel">
      <h2>销售与卡密记录</h2>
      <p>历史记录保留许可证及签发信息，不支持恢复卡密原文。</p>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>应用 / 客户</th>
              <th>数量</th>
              <th>成本</th>
              <th>状态</th>
              <th>签发时间</th>
              <th>许可证</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="o in orders" :key="o.id">
              <td>
                {{ o.application_name
                }}<small>{{ o.customer_ref || "未填写客户参考号" }}</small>
              </td>
              <td>{{ o.quantity }}</td>
              <td>{{ money(o.cost_cents) }}</td>
              <td>{{ o.status === "confirmed" ? "已签发" : o.status }}</td>
              <td>{{ formatTime(o.created_at) }}</td>
              <td class="wrap">{{ o.license_ids.join("\n") }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <section v-if="section === 'licenses'" class="panel">
      <h2>我签发的授权</h2>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>应用 / 客户</th>
              <th>许可证</th>
              <th>状态</th>
              <th>使用期限</th>
              <th>更新期限</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="l in licenses" :key="l.id">
              <td>
                {{ l.application_name }}<small>{{ l.customer_ref }}</small>
              </td>
              <td class="wrap">{{ l.id }}</td>
              <td>
                {{ licenseStateLabel(l) }}
              </td>
              <td>
                {{
                  l.terms.perpetual
                    ? "永久"
                    : l.expires_at
                      ? formatTime(l.expires_at)
                      : "首次激活起算"
                }}
              </td>
              <td>
                {{ updatePeriodLabel(l) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <section
      v-if="!['overview', 'generate', 'batches', 'licenses'].includes(section)"
      class="panel"
    >
      <h2>余额与资金流水</h2>
      <p>
        余额是当前用户代理资格下的统一资金账户，不同应用的发卡成本均从此扣除。
      </p>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>时间</th>
              <th>业务</th>
              <th>金额</th>
              <th>余额</th>
              <th>备注</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="e in transactions" :key="e.id">
              <td>{{ formatTime(e.created_at) }}</td>
              <td>
                {{
                  e.kind === "issue"
                    ? "发卡扣款"
                    : e.kind === "refund"
                      ? "退款退回"
                      : e.kind === "topup"
                        ? "管理员入账"
                        : e.kind
                }}
              </td>
              <td>{{ money(e.amount) }}</td>
              <td>{{ money(e.balance_after) }}</td>
              <td>{{ e.remark }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>
<style scoped>
.panel {
  padding: 24px;
  margin-bottom: 22px;
}
.agent-licensing {
  min-width: 0;
}
.agent-summary > div {
  min-width: 0;
}
.agent-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 18px;
  align-items: center;
}
.agent-summary small,
.agent-summary strong,
.agent-summary a {
  display: block;
  margin: 6px 0;
}
.agent-summary strong {
  font-size: 24px;
  overflow-wrap: anywhere;
}
.agent-summary small {
  color: var(--muted);
}
form {
  display: grid;
  gap: 16px;
  margin-top: 20px;
}
.issue-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}
.table-wrap {
  overflow: auto;
  margin-top: 20px;
}
table {
  width: 100%;
  min-width: 600px;
  border-collapse: collapse;
}
td,
th {
  padding: 14px 10px;
  text-align: left;
  border-bottom: 1px solid var(--line);
  vertical-align: top;
}
td small {
  display: block;
  color: var(--muted);
  margin-top: 5px;
}
.wrap {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  max-width: 350px;
}
.codes {
  padding: 20px;
  border: 1px solid var(--amber);
  border-radius: 10px;
  margin: 24px 0;
}
.codes textarea {
  width: 100%;
  font-family: monospace;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 14px;
}
@media (max-width: 600px) {
  .panel {
    padding: 16px;
  }
  .issue-grid {
    grid-template-columns: 1fr;
  }
  .agent-summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .agent-summary strong {
    font-size: 19px;
  }
}
</style>
