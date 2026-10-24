<script setup lang="ts">
import PagedRecords from "../shared/components/PagedRecords.vue";
import { computed, onMounted, reactive, ref } from "vue";
import { formatTime } from "../shared/format";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  products: any[];
  agents: any[];
  canWrite: boolean;
  token: string;
}>();
const catalog = ref<any>({ groups: [], packages: [], tiers: [] }),
  account = ref<any>(),
  selected = ref(""),
  error = ref(""),
  message = ref(""),
  busy = ref(false),
  codes = ref<string[]>([]);
const group = reactive({ id: "", name: "", product_ids: [] as string[] });
const pkg = reactive({
  product_id: "",
  name: "",
  duration_days: 30,
  is_permanent: false,
  base_cost: 1000,
  use_quota: false,
});
const tierName = ref(""),
  tierID = ref("");
const price = reactive({ tier_id: "", package_id: "", unit_cost: 700 });
const grant = reactive({
  package_id: "",
  group_id: "",
  enabled: true,
  max_batch_quantity: 100,
  can_export: false,
  override_cost: "",
});
const credit = reactive({
  amount: 10000,
  reason: "",
  request_id: crypto.randomUUID(),
});
const issue = reactive({
  package_id: "",
  quantity: 1,
  request_id: crypto.randomUUID(),
});
const quote = computed(() =>
  account.value?.packages.find((q: any) => q.package.id === issue.package_id),
);
const newRequestID = () => crypto.randomUUID();
const money = (n: number) => `¥${(n / 100).toFixed(2)}`;
const pname = (id: string) =>
  props.products.find((p) => p.id === id)?.name || id;
async function load() {
  catalog.value = await props.api("/admin/commerce");
  if (!pkg.product_id) pkg.product_id = props.products[0]?.id || "";
  if (!selected.value) selected.value = props.agents[0]?.id || "";
  await loadAccount();
}
async function loadAccount() {
  if (!selected.value) {
    account.value = null;
    return;
  }
  account.value = await props.api(`/admin/agents/${selected.value}/account`);
  tierID.value = account.value.account.tier_id;
  if (
    !account.value.packages.some((q: any) => q.package.id === issue.package_id)
  )
    issue.package_id = account.value.packages[0]?.package.id || "";
}
async function run(fn: () => Promise<any>) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await fn();
    message.value = "保存成功";
    await load();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
const post = (path: string, body: any) =>
  props.api(path, { method: "POST", body: JSON.stringify(body) });
function editGroup(g: any) {
  Object.assign(group, { ...g, product_ids: [...g.product_ids] });
}
async function saveGroup() {
  await post("/admin/product-groups", group);
  Object.assign(group, { id: "", name: "", product_ids: [] });
}
async function savePackage() {
  await post("/admin/packages", pkg);
  pkg.name = "";
}
async function saveGrant() {
  await post(`/admin/agents/${selected.value}/packages`, {
    ...grant,
    override_cost:
      grant.override_cost === "" ? null : Number(grant.override_cost),
  });
}
async function adjust() {
  await post(`/admin/agents/${selected.value}/balance`, credit);
  credit.request_id = crypto.randomUUID();
  credit.reason = "";
}
async function generate() {
  const r = await post(`/admin/agents/${selected.value}/card-batches`, {
    ...issue,
    name: "管理员代理套餐发卡",
    expected_unit_cost: quote.value?.unit_cost,
  });
  codes.value = r.codes;
  issue.request_id = crypto.randomUUID();
}
onMounted(() => run(load));
</script>
<template>
  <div class="commerce">
    <section class="section-intro">
      <div>
        <h2>应用分组与代理套餐</h2>
        <p>
          管理员统一定义规格与成本。代理仅可生成已授予的套餐，费用以人民币分结算。
        </p>
      </div>
    </section>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="message" class="notice" role="status">{{ message }}</p>
    <fieldset :disabled="busy || !canWrite">
      <section class="panel">
        <h3>应用分组</h3>
        <form @submit.prevent="run(saveGroup)">
          <label class="field"
            >分组名称<input v-model="group.name" required maxlength="160"
          /></label>
          <div class="choices">
            <label v-for="p in products" :key="p.id" class="check"
              ><input
                type="checkbox"
                v-model="group.product_ids"
                :value="p.id"
              />{{ p.name }}</label
            >
          </div>
          <div class="row-actions">
            <button class="btn">{{ group.id ? "更新分组" : "创建分组" }}</button
            ><button
              type="button"
              class="btn secondary"
              @click="
                Object.assign(group, { id: '', name: '', product_ids: [] })
              "
            >
              清空
            </button>
          </div>
        </form>
        <div class="groups">
          <button
            v-for="g in catalog.groups"
            :key="g.id"
            type="button"
            class="btn secondary"
            @click="editGroup(g)"
          >
            {{ g.name }} · {{ g.product_ids.length }} 个应用
          </button>
        </div>
      </section>
      <section class="panel">
        <h3>统一卡密套餐</h3>
        <p>
          套餐规格创建后固定；调整时长请新建套餐。使用与更新期限遵循应用规则；更新天数为
          0 时跟随套餐天数，永久卡的更新服务也受应用固定更新天数限制。
        </p>
        <form class="form-grid" @submit.prevent="run(savePackage)">
          <label class="field"
            >所属应用<select v-model="pkg.product_id" required>
              <option v-for="p in products" :value="p.id">{{ p.name }}</option>
            </select></label
          ><label class="field"
            >套餐名称<input
              v-model="pkg.name"
              required
              maxlength="160" /></label
          ><label class="field"
            >套餐天数<input
              v-model.number="pkg.duration_days"
              type="number"
              min="1"
              max="36500"
              :disabled="pkg.is_permanent"
              required /></label
          ><label class="field"
            >基础成本（分）<input
              v-model.number="pkg.base_cost"
              type="number"
              min="0"
              max="1000000000000"
              required /></label
          ><label class="check"
            ><input v-model="pkg.is_permanent" type="checkbox" />永久卡</label
          ><label class="check"
            ><input
              v-model="pkg.use_quota"
              type="checkbox"
            />同时扣除已发放的张数额度</label
          >
          <div class="form-actions"><button class="btn">建立套餐</button></div>
        </form>
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>应用 / 套餐</th>
                <th>规格</th>
                <th>基础成本（分）</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in catalog.packages" :key="p.id">
                <td>{{ pname(p.product_id) }} / {{ p.name }}</td>
                <td>
                  {{ p.is_permanent ? "永久" : `${p.duration_days} 天`
                  }}{{ p.use_quota ? " + 张数额度" : "" }}
                </td>
                <td>
                  <input
                    :aria-label="`${p.name}基础成本`"
                    v-model.number="p.base_cost"
                    type="number"
                    min="0"
                    max="1000000000000"
                  />
                </td>
                <td>{{ p.enabled ? "上架" : "停用" }}</td>
                <td class="row-actions">
                  <button
                    class="btn small secondary"
                    @click="run(() => post(`/admin/packages/${p.id}`, p))"
                  >
                    保存成本</button
                  ><button
                    class="btn small secondary"
                    @click="
                      run(() =>
                        post(`/admin/packages/${p.id}`, {
                          ...p,
                          enabled: !p.enabled,
                        }),
                      )
                    "
                  >
                    {{ p.enabled ? "停用套餐" : "上架套餐" }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section class="panel">
        <h3>代理等级与价格</h3>
        <form
          @submit.prevent="
            run(async () => {
              await post('/admin/agent-tiers', { name: tierName });
              tierName = '';
            })
          "
        >
          <label class="field"
            >等级名称<input
              v-model="tierName"
              required
              maxlength="160" /></label
          ><button class="btn">创建等级</button>
        </form>
        <form
          class="form-grid"
          @submit.prevent="
            run(() => post(`/admin/agent-tiers/${price.tier_id}/prices`, price))
          "
        >
          <label class="field"
            >代理等级<select v-model="price.tier_id" required>
              <option value="" disabled>选择等级</option>
              <option v-for="t in catalog.tiers" :value="t.id">
                {{ t.name }}
              </option>
            </select></label
          ><label class="field"
            >定价套餐<select v-model="price.package_id" required>
              <option value="" disabled>选择套餐</option>
              <option v-for="p in catalog.packages" :value="p.id">
                {{ pname(p.product_id) }} / {{ p.name }}
              </option>
            </select></label
          ><label class="field"
            >等级成本（分）<input
              v-model.number="price.unit_cost"
              type="number"
              min="0"
              max="1000000000000"
              required
          /></label>
          <div class="form-actions">
            <button class="btn">保存等级价格</button
            ><button
              class="btn secondary"
              type="button"
              :disabled="!price.tier_id || !price.package_id"
              @click="
                run(() =>
                  post(`/admin/agent-tiers/${price.tier_id}/prices`, {
                    package_id: price.package_id,
                    unit_cost: null,
                  }),
                )
              "
            >
              恢复套餐基础价
            </button>
          </div>
        </form>
      </section>
    </fieldset>
    <section class="panel">
      <h3>代理授权与资金</h3>
      <label class="field"
        >当前代理<select v-model="selected" @change="run(loadAccount)">
          <option v-for="a in agents" :value="a.id">{{ a.name }}</option>
        </select></label
      >
      <p v-if="!agents.length">请先在代理管理中建立代理及登录账号。</p>
      <template v-if="account"
        ><p class="balance">
          可用余额 <strong>{{ money(account.account.balance) }}</strong>
        </p>
        <fieldset :disabled="busy || !canWrite">
          <form
            class="form-grid"
            @submit.prevent="
              run(() =>
                post(`/admin/agents/${selected}/tier`, { tier_id: tierID }),
              )
            "
          >
            <label class="field"
              >分配等级<select v-model="tierID">
                <option value="">未分配（套餐基础价）</option>
                <option v-for="t in catalog.tiers" :value="t.id">
                  {{ t.name }}
                </option>
              </select></label
            >
            <div class="form-actions">
              <button class="btn">保存代理等级</button>
            </div>
          </form>
          <form class="form-grid" @submit.prevent="run(saveGrant)">
            <label class="field"
              >授权套餐<select
                v-model="grant.package_id"
                :disabled="!!grant.group_id"
              >
                <option value="">选择套餐</option>
                <option v-for="p in catalog.packages" :value="p.id">
                  {{ pname(p.product_id) }} / {{ p.name }}
                </option>
              </select></label
            ><label class="field"
              >或按分组批量授权<select
                v-model="grant.group_id"
                @change="grant.package_id = ''"
              >
                <option value="">不使用分组</option>
                <option v-for="g in catalog.groups" :value="g.id">
                  {{ g.name }}
                </option>
              </select></label
            ><label class="field"
              >代理专属成本（分，可留空）<input
                v-model="grant.override_cost"
                type="number"
                min="0"
                max="1000000000000"
                placeholder="留空使用等级价 / 基础价" /></label
            ><label class="field"
              >单批上限<input
                v-model.number="grant.max_batch_quantity"
                type="number"
                min="1"
                max="1000"
                required /></label
            ><label class="check"
              ><input
                v-model="grant.enabled"
                type="checkbox"
              />授予套餐（取消勾选即撤销）</label
            ><label class="check"
              ><input
                v-model="grant.can_export"
                type="checkbox"
              />允许明文导出</label
            >
            <p>分组授权仅覆盖当前已建立且上架的套餐，后续新增需重新授予。</p>
            <div class="form-actions">
              <button
                class="btn"
                :disabled="!grant.package_id && !grant.group_id"
              >
                保存套餐授权
              </button>
            </div>
          </form>
          <form class="form-grid" @submit.prevent="run(adjust)">
            <label class="field"
              >余额调整（分，可为负数）<input
                v-model.number="credit.amount"
                type="number"
                min="-1000000000000"
                max="1000000000000"
                required /></label
            ><label class="field"
              >调整原因<input v-model="credit.reason" required maxlength="500"
            /></label>
            <div class="form-actions">
              <button class="btn">登记余额调整</button>
            </div>
          </form>
          <form class="form-grid" @submit.prevent="run(generate)">
            <label class="field"
              >代生成套餐<select
                v-model="issue.package_id"
                @change="issue.request_id = newRequestID()"
                required
              >
                <option v-for="q in account.packages" :value="q.package.id">
                  {{ q.package.name }} · {{ money(q.unit_cost) }} / 张
                </option>
              </select></label
            ><label class="field"
              >发卡数量<input
                v-model.number="issue.quantity"
                @change="issue.request_id = newRequestID()"
                type="number"
                min="1"
                max="1000"
                required
            /></label>
            <p>
              本次成本 {{ money((quote?.unit_cost || 0) * issue.quantity) }}
            </p>
            <div class="form-actions">
              <button class="btn" :disabled="!quote">按代理价格代生成</button>
            </div>
          </form>
          <details v-if="codes.length">
            <summary>查看本次明文卡密（请妥善保存）</summary>
            <pre>{{ codes.join("\n") }}</pre>
          </details>
        </fieldset>
        <h4>当前可售套餐与实际成本</h4>
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>应用 / 套餐</th>
                <th>成本</th>
                <th>价格来源</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="q in account.packages" :key="q.package.id">
                <td>
                  {{ pname(q.package.product_id) }} / {{ q.package.name }}
                </td>
                <td>{{ money(q.unit_cost) }}</td>
                <td>
                  {{
                    (
                      {
                        agent: "代理专属价",
                        tier: "等级价",
                        package: "套餐基础价",
                      } as any
                    )[q.price_source]
                  }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <h4>资金流水</h4>
        <PagedRecords
          :key="`${selected}-${account.account.balance}-${account.ledgers.length}`"
          :api="api"
          :token="token"
          endpoint="/admin/history/money"
          kind="money"
          :params="{ agent_id: selected }"
        />
      </template>
    </section>
  </div>
</template>
<style scoped>
.commerce {
  display: grid;
  gap: 24px;
}
fieldset {
  border: 0;
  padding: 0;
  margin: 0;
  min-width: 0;
  display: grid;
  gap: 24px;
}
.panel {
  padding: 26px;
}
form {
  display: grid;
  gap: 16px;
  margin: 20px 0;
}
input,
select {
  max-width: 100%;
}
.choices,
.groups {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}
.notice {
  padding: 18px;
  background: #e9f3ec;
}
.error {
  color: #a13434;
  background: #faeaea;
}
.balance {
  padding: 20px;
  background: #edf5ef;
}
.balance strong {
  font-size: 26px;
  margin-left: 20px;
}
.table td input {
  width: 150px;
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
