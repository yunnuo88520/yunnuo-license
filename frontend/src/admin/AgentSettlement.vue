<script setup lang="ts">
import { onMounted, ref, reactive } from "vue";
import { costRateLabel, formatTime } from "../shared/format";
import FormDialog from "../shared/components/FormDialog.vue";
import ConfirmDialog from "../shared/components/ConfirmDialog.vue";
const props = defineProps<{
  api: (path: string, options?: RequestInit) => Promise<any>;
  agents: any[];
  canWrite: boolean;
  agentId?: string;
}>();
const tiers = ref<any[]>([]),
  selected = ref(props.agentId || ""),
  account = ref<any>(),
  tierID = ref(""),
  error = ref(""),
  notice = ref(""),
  busy = ref(false),
  tierOpen = ref(false),
  deleting = ref<any>();
const tier = reactive({ id: "", name: "", rate: 100, sort_order: 100 });
const balance = reactive({
  amount: 0,
  reason: "",
  request_id: crypto.randomUUID(),
});
const money = (v: number) => `¥${((v || 0) / 100).toFixed(2)}`;
async function run(fn: () => Promise<any>) {
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
async function loadAccount() {
  if (!selected.value) return;
  account.value = await props.api(`/admin/agents/${selected.value}/account`);
  const records = await props.api(
    `/v2/platform/agents/${selected.value}/records`,
  );
  account.value.ledgers = [
    ...(account.value.ledgers || []),
    ...records.flatMap((r: any) =>
      r.money_events.map((e: any) => ({
        ...e,
        amount: e.amount_cents,
        remark: `${r.application.name} · ${e.kind === "refund" ? "退款退回" : "发卡扣款"}`,
      })),
    ),
  ].sort((a: any, b: any) => b.created_at.localeCompare(a.created_at));

  tierID.value = account.value.account.tier_id || "";
}
async function load() {
  const c = await props.api("/admin/commerce");
  tiers.value = c.tiers || [];
  if (!selected.value) selected.value = props.agents[0]?.id || "";
  await loadAccount();
}
const post = (p: string, b: any) =>
  props.api(p, { method: "POST", body: JSON.stringify(b) });
function edit(t?: any) {
  Object.assign(
    tier,
    t
      ? { ...t, rate: t.cost_rate_bps / 100 }
      : { id: "", name: "", rate: 100, sort_order: 100 },
  );
  tierOpen.value = true;
}
async function saveTier() {
  await post(tier.id ? `/admin/agent-tiers/${tier.id}` : "/admin/agent-tiers", {
    name: tier.name,
    cost_rate_bps: Math.round(tier.rate * 100),
    sort_order: tier.sort_order,
  });
  tierOpen.value = false;
  await load();
  notice.value = "代理等级已保存";
}
async function deleteTier() {
  await props.api(`/admin/agent-tiers/${deleting.value.id}`, {
    method: "DELETE",
  });
  deleting.value = undefined;
  await load();
}
async function adjust() {
  await post(`/admin/agents/${selected.value}/balance`, {
    amount: Math.round(balance.amount * 100),
    reason: balance.reason,
    request_id: balance.request_id,
  });
  balance.request_id = crypto.randomUUID();
  balance.amount = 0;
  balance.reason = "";
  await loadAccount();
  notice.value = "余额调整已记账";
}
onMounted(() => run(load));
</script>
<template>
  <div class="page-stack">
    <div class="page-head">
      <h2>代理等级与结算</h2>
      <p>
        应用授权中心配置可售应用、套餐和代理专属价；这里管理平台等级和人工充值。
      </p>
    </div>
    <p v-if="error" role="alert" class="notice error">{{ error }}</p>
    <p v-if="notice" role="status" class="notice">{{ notice }}</p>
    <section v-if="!agentId" class="panel">
      <div class="panel-head">
        <h3>代理等级与价格</h3>
        <button v-if="canWrite" class="btn" @click="edit()">创建等级</button>
      </div>
      <p>
        默认普通、高级、金牌三个等级。结算成本依次采用代理专属价、套餐基础成本 ×
        等级比例；不足 1 分向上取整。
      </p>
      <div class="table-wrap">
        <table class="table" aria-label="代理等级列表">
          <thead>
            <tr>
              <th>等级名称</th>
              <th>成本比例</th>
              <th>排序</th>
              <th>使用代理数</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="t in tiers" :key="t.id">
              <td>{{ t.name }}</td>
              <td>{{ costRateLabel(t.cost_rate_bps) }}</td>
              <td>{{ t.sort_order }}</td>
              <td>{{ t.agent_count }}</td>
              <td>
                <div v-if="canWrite" class="row-actions">
                  <button class="btn secondary small" @click="edit(t)">
                    编辑等级</button
                  ><button
                    class="btn danger small"
                    :disabled="t.agent_count > 0"
                    @click="deleting = t"
                  >
                    删除等级
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <section class="panel">
      <h3>代理资金与等级</h3>
      <label v-if="!agentId" class="field"
        >当前代理<select
          v-model="selected"
          aria-label="当前代理"
          @change="run(loadAccount)"
        >
          <option value="" disabled>选择代理</option>
          <option v-for="a in agents" :key="a.id" :value="a.id">
            {{ a.name }}
          </option>
        </select></label
      ><template v-if="account"
        ><p>
          可用余额 <strong>{{ money(account.account.balance) }}</strong>
        </p>
        <fieldset :disabled="busy || !canWrite">
          <form
            class="form-grid"
            @submit.prevent="
              run(async () => {
                await post(`/admin/agents/${selected}/tier`, {
                  tier_id: tierID,
                });
                await loadAccount();
                notice = '代理等级已保存';
              })
            "
          >
            <label class="field"
              >分配等级<select v-model="tierID" aria-label="分配等级">
                <option value="">未分配（套餐基础价）</option>
                <option v-for="t in tiers" :value="t.id">
                  {{ t.name }} · {{ costRateLabel(t.cost_rate_bps) }}
                </option>
              </select></label
            >
            <div class="form-actions">
              <button class="btn">保存代理等级</button>
            </div>
          </form>
          <form class="form-grid" @submit.prevent="run(adjust)">
            <label class="field"
              >调整金额（元；负数扣减）<input
                v-model.number="balance.amount"
                type="number"
                step="0.01"
                required /></label
            ><label class="field"
              >入账原因<input v-model="balance.reason" required maxlength="500"
            /></label>
            <div class="form-actions">
              <button class="btn">确认人工入账</button>
            </div>
          </form>
        </fieldset>
        <h4>资金流水</h4>
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>时间</th>
                <th>金额</th>
                <th>余额</th>
                <th>原因</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="l in account.ledgers" :key="l.id">
                <td>{{ formatTime(l.created_at) }}</td>
                <td>{{ money(l.amount) }}</td>
                <td>{{ money(l.balance_after) }}</td>
                <td>{{ l.remark || l.reason }}</td>
              </tr>
            </tbody>
          </table>
        </div></template
      >
    </section>
    <FormDialog
      :open="tierOpen"
      :title="tier.id ? '编辑代理等级' : '创建代理等级'"
      @close="tierOpen = false"
      ><form class="form-grid" @submit.prevent="run(saveTier)">
        <label class="field"
          >等级名称<input v-model="tier.name" required maxlength="80" /></label
        ><label class="field"
          >成本比例（%）<input
            v-model.number="tier.rate"
            type="number"
            min="0"
            max="10000"
            step="0.01"
            required /></label
        ><label class="field"
          >排序<input
            v-model.number="tier.sort_order"
            type="number"
            required /></label
        ><button class="btn" :disabled="busy">保存等级</button>
      </form></FormDialog
    ><ConfirmDialog
      :open="!!deleting"
      title="删除代理等级"
      :description="`确认删除 ${deleting?.name || ''}？正在使用的等级不能删除。`"
      @close="deleting = undefined"
      @confirm="run(deleteTier)"
    />
  </div>
</template>
