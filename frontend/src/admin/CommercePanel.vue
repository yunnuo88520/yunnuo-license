<script setup lang="ts">
import AuthorizationTermsFields from "../shared/components/AuthorizationTermsFields.vue";
import { authorizationDefaults, termsSummary } from "../shared/authorization";
import PagedRecords from "../shared/components/PagedRecords.vue";
import FormDialog from "../shared/components/FormDialog.vue";
import ConfirmDialog from "../shared/components/ConfirmDialog.vue";
import { computed, onMounted, reactive, ref, watch } from "vue";
import { Plus } from "lucide-vue-next";
import { formatTime, costRateLabel } from "../shared/format";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  products: any[];
  agents: any[];
  canWrite: boolean;
  token: string;
  agentId?: string;
}>();
const catalog = ref<any>({ groups: [], packages: [], tiers: [] }),
  account = ref<any>(),
  selected = ref(props.agentId || ""),
  catalogTab = ref(props.agentId ? "agents" : "catalog"),
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
const customTerms = ref(false),
  packageTerms = ref(authorizationDefaults());
watch(
  () => pkg.product_id,
  () => {
    packageTerms.value = authorizationDefaults(
      props.products.find((p) => p.id === pkg.product_id),
    );
  },
);
const tierID = ref("");
const tierForm = reactive({
  id: "",
  name: "",
  cost_rate_percent: 100,
  sort_order: 100,
});
const deleteTier = ref<any>();
function editTier(t?: any) {
  Object.assign(
    tierForm,
    t
      ? {
          id: t.id,
          name: t.name,
          cost_rate_percent: t.cost_rate_bps / 100,
          sort_order: t.sort_order,
        }
      : { id: "", name: "", cost_rate_percent: 100, sort_order: 100 },
  );
  show.tier = true;
}
async function saveTier() {
  await post(
    tierForm.id ? `/admin/agent-tiers/${tierForm.id}` : "/admin/agent-tiers",
    {
      name: tierForm.name,
      cost_rate_bps: Math.round(tierForm.cost_rate_percent * 100),
      sort_order: tierForm.sort_order,
    },
  );
  show.tier = false;
}
async function removeTier() {
  await props.api(`/admin/agent-tiers/${deleteTier.value.id}`, {
    method: "DELETE",
  });
  deleteTier.value = undefined;
}
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
const show = reactive({ group: false, pkg: false, tier: false, price: false });
function editGroup(g: any) {
  Object.assign(group, { ...g, product_ids: [...g.product_ids] });
  show.group = true;
}
async function saveGroup() {
  await post("/admin/product-groups", group);
  Object.assign(group, { id: "", name: "", product_ids: [] });
  show.group = false;
}
async function savePackage() {
  await post("/admin/packages", {
    ...pkg,
    authorization_terms: customTerms.value ? packageTerms.value : undefined,
  });
  pkg.name = "";
  show.pkg = false;
}
async function saveGrant() {
  await post(`/admin/agents/${selected.value}/packages`, {
    ...grant,
    override_cost:
      grant.override_cost === "" ? null : Number(grant.override_cost),
  });
}
async function adjust() {
  if (Number(credit.amount) === 0) {
    error.value = "调整金额不能为 0。";
    return;
  }
  if (!credit.reason.trim()) {
    error.value = "请填写调整原因。";
    return;
  }
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
    <section v-if="!agentId" class="section-intro">
      <div>
        <h2>应用分组与代理套餐</h2>
        <p>
          管理员统一定义规格与成本。代理仅可生成已授予的套餐，费用以人民币分结算。
        </p>
      </div>
    </section>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="message" class="notice" role="status">{{ message }}</p>
    <div
      v-if="!agentId"
      class="tabbar plain"
      role="tablist"
      aria-label="套餐与结算视图"
    >
      <button
        type="button"
        role="tab"
        :aria-selected="catalogTab === 'catalog'"
        :class="{ active: catalogTab === 'catalog' }"
        @click="catalogTab = 'catalog'"
      >
        套餐与价格
      </button>
      <button
        type="button"
        role="tab"
        :aria-selected="catalogTab === 'agents'"
        :class="{ active: catalogTab === 'agents' }"
        @click="catalogTab = 'agents'"
      >
        代理运营
      </button>
    </div>
    <fieldset v-if="catalogTab === 'catalog'" :disabled="busy || !canWrite">
      <section class="panel">
        <div class="panel-title">
          <h3>应用分组</h3>
          <button
            class="btn small"
            type="button"
            :disabled="busy || !canWrite"
            @click="
              Object.assign(group, { id: '', name: '', product_ids: [] });
              show.group = true;
            "
          >
            <Plus :size="14" />{{ group.id ? "继续编辑分组" : "新增分组" }}
          </button>
        </div>
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
      <FormDialog
        :open="show.group"
        :title="group.id ? '编辑分组' : '新增分组'"
        :description="
          group.id
            ? '调整分组包含的应用后保存。'
            : '分组用于按批向代理授予套餐。'
        "
        @close="show.group = false"
      >
        <p v-if="error" class="modal-error" role="alert">{{ error }}</p>
        <form class="form-grid" @submit.prevent="run(saveGroup)">
          <label class="field"
            >分组名称<input v-model="group.name" required maxlength="160"
          /></label>
          <div class="choices wide">
            <label v-for="p in products" :key="p.id" class="check"
              ><input
                type="checkbox"
                v-model="group.product_ids"
                :value="p.id"
              />{{ p.name }}</label
            >
          </div>
          <div class="form-actions">
            <button class="btn" :disabled="busy">
              {{ group.id ? "更新分组" : "创建分组" }}
            </button>
          </div>
        </form>
      </FormDialog>
      <section class="panel">
        <div class="panel-title">
          <h3>统一卡密套餐</h3>
          <button
            class="btn small"
            type="button"
            :disabled="busy || !canWrite"
            @click="show.pkg = true"
          >
            <Plus :size="14" />建立套餐
          </button>
        </div>
        <p>
          套餐规格创建后固定；调整时长请新建套餐。使用与更新期限遵循应用规则；更新天数为
          0 时跟随套餐天数，永久卡的更新服务也受应用固定更新天数限制。
        </p>
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
                  }}{{ p.use_quota ? " + 张数额度" : ""
                  }}<small class="package-terms">{{
                    termsSummary(p.authorization_terms)
                  }}</small>
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
                    @click="
                      run(() =>
                        post(`/admin/packages/${p.id}`, {
                          enabled: p.enabled,
                          base_cost: Number(p.base_cost),
                        }),
                      )
                    "
                  >
                    保存成本</button
                  ><button
                    class="btn small secondary"
                    @click="
                      run(() =>
                        post(`/admin/packages/${p.id}`, {
                          enabled: !p.enabled,
                          base_cost: Number(p.base_cost),
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
      <FormDialog
        :open="show.pkg"
        title="建立套餐"
        description="套餐规格创建后固定，调整时长需另建套餐。"
        @close="show.pkg = false"
      >
        <p v-if="error" class="modal-error" role="alert">{{ error }}</p>
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
          <label class="check"
            ><input
              v-model="customTerms"
              type="checkbox"
            />自定义套餐授权条款</label
          >
          <AuthorizationTermsFields v-if="customTerms" v-model="packageTerms" />
          <p v-else>
            创建时保存当前应用的默认条款；之后修改应用不会改变本套餐或已生成的卡密。
          </p>
          <div class="form-actions">
            <button class="btn" :disabled="busy">建立套餐</button>
          </div>
        </form>
      </FormDialog>
      <section class="panel">
        <div class="panel-title">
          <h3>代理等级与价格</h3>
          <div class="row-actions">
            <button
              class="btn small"
              type="button"
              :disabled="busy || !canWrite"
              @click="editTier()"
            >
              <Plus :size="14" />创建等级
            </button>
            <button
              class="btn small secondary"
              type="button"
              :disabled="busy || !canWrite"
              @click="show.price = true"
            >
              等级定价
            </button>
          </div>
        </div>
        <p>
          默认普通、高级、金牌三个等级，可调整名称与成本比例。成本按套餐基础价 ×
          等级比例计算，不足 1 分向上取整。
        </p>
        <p>
          价格优先级：代理专属价 → 等级套餐专属价 → 等级比例 →
          未分配等级时的套餐基础价。修改比例只影响后续发卡，历史成本与流水保持原值。
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
              <tr v-for="t in catalog.tiers" :key="t.id">
                <td>{{ t.name }}</td>
                <td>{{ costRateLabel(t.cost_rate_bps) }}</td>
                <td>{{ t.sort_order }}</td>
                <td>{{ t.agent_count }}</td>
                <td>
                  <div class="row-actions">
                    <button
                      class="btn secondary small"
                      type="button"
                      :disabled="busy || !canWrite"
                      @click="editTier(t)"
                    >
                      编辑等级</button
                    ><button
                      class="btn danger small"
                      type="button"
                      :disabled="busy || !canWrite || t.agent_count > 0"
                      :title="
                        t.agent_count > 0
                          ? '请先为使用此等级的代理分配其他等级'
                          : '删除未使用的等级'
                      "
                      @click="deleteTier = t"
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
      <FormDialog
        :open="show.tier"
        :title="tierForm.id ? '编辑代理等级' : '创建代理等级'"
        description="比例 80% 表示按套餐基础成本的八折结算，支持两位小数。等级不会扩大可售应用范围。"
        @close="show.tier = false"
      >
        <p v-if="error" class="modal-error" role="alert">{{ error }}</p>
        <form class="form-grid" @submit.prevent="run(saveTier)">
          <label class="field"
            >等级名称<input v-model="tierForm.name" required maxlength="160"
          /></label>
          <label class="field"
            >成本比例（%）<input
              v-model.number="tierForm.cost_rate_percent"
              type="number"
              min="0.01"
              max="100"
              step="0.01"
              required
          /></label>
          <label class="field"
            >显示排序<input
              v-model.number="tierForm.sort_order"
              type="number"
              min="0"
              max="10000"
              step="1"
              required
            /><small>数值越小越靠前。</small></label
          >
          <div class="form-actions">
            <button class="btn" :disabled="busy">
              {{ tierForm.id ? "保存等级" : "创建等级" }}
            </button>
          </div>
        </form>
      </FormDialog>
      <FormDialog
        :open="show.price"
        title="等级定价"
        description="为等级的指定套餐设置专属成本；删除专属价后恢复按等级比例计算。"
        @close="show.price = false"
      >
        <p v-if="error" class="modal-error" role="alert">{{ error }}</p>
        <form
          class="form-grid"
          @submit.prevent="
            run(async () => {
              await post(`/admin/agent-tiers/${price.tier_id}/prices`, {
                package_id: price.package_id,
                unit_cost: Number(price.unit_cost),
              });
              show.price = false;
            })
          "
        >
          <label class="field"
            >代理等级<select v-model="price.tier_id" required>
              <option value="" disabled>选择等级</option>
              <option v-for="t in catalog.tiers" :value="t.id">
                {{ t.name }} · {{ costRateLabel(t.cost_rate_bps) }}
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
            <button class="btn" :disabled="busy">保存等级价格</button
            ><button
              class="btn secondary"
              type="button"
              :disabled="!price.tier_id || !price.package_id"
              @click="
                run(async () => {
                  await post(`/admin/agent-tiers/${price.tier_id}/prices`, {
                    package_id: price.package_id,
                    unit_cost: null,
                  });
                  show.price = false;
                })
              "
            >
              恢复按等级比例
            </button>
          </div>
        </form>
      </FormDialog>
    </fieldset>
    <section v-if="catalogTab === 'agents'" class="panel">
      <h3>代理授权与资金</h3>
      <label v-if="!agentId" class="field"
        >当前代理<select v-model="selected" @change="run(loadAccount)">
          <option v-for="a in agents" :value="a.id">{{ a.name }}</option>
        </select></label
      >
      <p v-if="!agents.length">请先在代理管理中一次开通代理账号。</p>
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
                  {{ t.name }} · {{ costRateLabel(t.cost_rate_bps) }}
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
                        tier_ratio: "等级比例",
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
    <ConfirmDialog
      :open="!!deleteTier"
      title="删除代理等级"
      :description="`删除 ${deleteTier?.name || ''} 及其套餐专属价。已被代理使用的等级不能删除，历史发卡成本不会改变。`"
      confirm-text="确认删除等级"
      @close="deleteTier = undefined"
      @confirm="run(removeTier)"
    />
  </div>
</template>
<style scoped>
.package-terms {
  display: block;
  margin-top: 6px;
  max-width: 340px;
  color: var(--muted);
  white-space: normal;
}
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
.panel-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.panel-title h3 {
  margin: 0;
}
.modal-error {
  margin: 0;
  padding: 12px 20px 0;
  color: var(--red);
  font-size: 12px;
}
.choices.wide {
  grid-column: span 2;
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
  background: var(--green-soft);
}
.error {
  color: var(--red);
  background: var(--red-soft);
}
.balance {
  padding: 20px;
  background: var(--green-soft);
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
