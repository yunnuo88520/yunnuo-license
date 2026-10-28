<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { downloadText } from "../shared/api";
import {
  formatTime,
  licenseStateLabel,
  updatePeriodLabel,
} from "../shared/format";
import FormDialog from "../shared/components/FormDialog.vue";
const props = defineProps<{
  api: (path: string, options?: RequestInit) => Promise<any>;
}>();
const items = ref<any[]>([]),
  apps = ref<any[]>([]),
  busy = ref(false),
  error = ref(""),
  notice = ref("");
const claim = reactive({
  app_id: new URLSearchParams(location.search).get("app_id") || "",
  card_code: "",
});
const op = reactive({
  kind: "",
  item: null as any,
  activation_id: "",
  card_code: "",
  type: "domain",
  value: "",
  aliases: "",
  reason: "",
  hours: 24,
});
const invitation = ref(""),
  showInvitation = ref(false);
const choices = computed(() => {
  const result = new Map(apps.value.map((a) => [a.app_id, a]));
  for (const item of items.value)
    result.set(item.application.app_id, item.application);
  if (claim.app_id && !result.has(claim.app_id))
    result.set(claim.app_id, { app_id: claim.app_id, name: "当前产品" });
  return [...result.values()];
});
const post = (path: string, data: any) =>
  props.api(path, { method: "POST", body: JSON.stringify(data) });
async function run(fn: () => Promise<void>) {
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
  [items.value, apps.value] = await Promise.all([
    props.api("/v2/user/entitlements"),
    props.api("/v2/public/applications"),
  ]);
  if (!claim.app_id) claim.app_id = choices.value[0]?.app_id || "";
}
async function associate() {
  await post(`/v2/user/applications/${claim.app_id}/claim`, {
    card_code: claim.card_code,
  });
  claim.card_code = "";
  await refresh();
  notice.value =
    "授权已关联当前账号。请在实际产品中激活，或生成一次性安装邀请。";
}
function open(kind: string, item: any, activation = "") {
  Object.assign(op, {
    kind,
    item,
    activation_id: activation,
    card_code: "",
    type:
      item.license.terms.binding_types.find((t: string) => t !== "seat") ||
      "installation",
    value: "",
    aliases: "",
    reason: "",
    hours: 24,
  });
}
async function submit() {
  const id = op.item.license.id,
    app = op.item.application.app_id;
  let payload: any;
  if (op.kind === "appeal") payload = { reason: op.reason };
  else if (op.kind === "renew") payload = { card_code: op.card_code };
  else if (op.kind === "invite") payload = { hours: op.hours };
  else if (op.kind === "deactivate")
    payload = { activation_id: op.activation_id, reason: op.reason };
  else
    payload = {
      binding: {
        type: op.type,
        value: op.value,
        aliases: op.aliases
          .split(/[\n,]/)
          .map((v) => v.trim())
          .filter(Boolean),
      },
      reason: op.reason,
    };
  const result = await post(
    `/v2/user/applications/${app}/licenses/${id}/${op.kind}`,
    payload,
  );
  if (op.kind === "invite") {
    invitation.value = `AppID: ${app}\nInvitation ID: ${result.invitation.id}\nInvitation token: ${result.token}\n到期：${formatTime(result.invitation.expires_at)}`;
    showInvitation.value = true;
  }
  const appealed = op.kind === "appeal";
  op.kind = "";
  op.card_code = "";
  await refresh();
  notice.value = appealed
    ? `申诉已提交，编号 ${result.event_id}，请等待管理员核实。`
    : "操作已完成。";
}
const status = (value: string) =>
  ({
    active: "有效",
    suspended: "已暂停",
    revoked: "已吊销",
    expired: "已到期",
  })[value] || value;
const title = computed(
  () =>
    ({
      renew: "卡密续费",
      appeal: "授权申诉",
      invite: "生成安装邀请",
      rebind: "更换部署绑定",
      deactivate: "停用旧安装",
    })[op.kind] || "操作",
);
onMounted(() => run(refresh));
</script>
<template>
  <div class="customer-licenses">
    <p v-if="error" role="alert" class="error">{{ error }}</p>
    <p v-if="notice" role="status" class="notice">{{ notice }}</p>
    <section class="panel">
      <h2>关联购买的授权</h2>
      <p>
        选择应用并填写购买的激活卡密。关联账号不会伪造设备激活；客户安装仍通过产品里的激活页面或一次性邀请完成关联。
      </p>
      <form class="claim-grid" @submit.prevent="run(associate)">
        <label class="field"
          >选择应用<select
            v-model="claim.app_id"
            required
            aria-label="选择应用"
          >
            <option value="" disabled>请选择应用</option>
            <option v-for="a in choices" :key="a.app_id" :value="a.app_id">
              {{ a.name }}
            </option>
          </select></label
        ><label class="field"
          >购买的激活卡密<input
            v-model="claim.card_code"
            type="password"
            autocomplete="off"
            required /></label
        ><button class="btn" :disabled="busy">关联到我的账号</button>
      </form>
    </section>
    <div class="heading">
      <h2>我的应用授权</h2>
      <button class="btn secondary" :disabled="busy" @click="run(refresh)">
        刷新授权
      </button>
    </div>
    <p v-if="!items.length" class="muted">
      暂未关联授权。可填写购买的卡密，或联系管理员分配到当前账号。
    </p>
    <section
      v-for="item in items"
      :key="item.license.id"
      class="panel entitlement"
    >
      <div class="heading">
        <div>
          <h2>{{ item.application.name }}</h2>
          <p class="wrap">{{ item.license.id }}</p>
        </div>
        <span class="badge">{{
          licenseStateLabel(item.license, item.application_status)
        }}</span>
      </div>
      <div class="facts">
        <article>
          <small>使用期限</small
          ><strong>{{
            item.license.terms.perpetual
              ? "永久使用"
              : item.license.expires_at
                ? formatTime(item.license.expires_at)
                : "首次激活起算 " + item.license.terms.duration_days + " 天"
          }}</strong>
        </article>
        <article>
          <small>更新资格</small
          ><strong>{{
            updatePeriodLabel(item.license, item.updates_enabled)
          }}</strong>
        </article>
        <article>
          <small>额度</small
          ><strong>{{
            item.license.terms.model === "floating"
              ? item.license.terms.max_seats + " 个同时在线席位"
              : item.license.terms.max_activations + " 份活动安装"
          }}</strong
          ><small v-if="item.license.terms.model === 'metered'"
            >计量 {{ item.license.used_units }} /
            {{ item.license.terms.usage_quota }}
            {{ item.license.terms.usage_unit }}</small
          >
        </article>
        <article>
          <small>自助换绑</small
          ><strong>{{
            item.license.terms.rebind_limit === 0
              ? "未开放"
              : item.license.terms.rebind_limit === -1
                ? "不限次数"
                : "已用 " +
                  item.license.rebinds_used +
                  " / " +
                  item.license.terms.rebind_limit +
                  " 次"
          }}</strong>
        </article>
      </div>
      <p v-if="item.application_status !== 'active'" class="error">
        应用已暂停服务，请联系开发者。
      </p>
      <p v-if="item.license.terms.features?.length">
        已购买功能：{{ item.license.terms.features.join("、") }}
      </p>
      <p class="muted">
        永久使用不代表永久更新。授权归属与账号绑定，用户不能自行转让。
      </p>
      <div class="actions">
        <button
          class="btn"
          :disabled="busy || item.license.status === 'revoked'"
          @click="open('renew', item)"
        >
          卡密续费</button
        ><button
          class="btn secondary"
          :disabled="busy || item.license.status !== 'active'"
          @click="open('invite', item)"
        >
          生成安装邀请</button
        ><button
          class="btn secondary"
          :disabled="
            busy ||
            item.license.terms.rebind_limit === 0 ||
            item.license.status !== 'active'
          "
          @click="open('rebind', item)"
        >
          更换部署绑定
        </button>
      </div>
      <button
        class="btn secondary"
        :disabled="busy"
        @click="open('appeal', item)"
      >
        提交授权申诉
      </button>
      <details>
        <summary>安装与席位记录（{{ item.activations.length }}）</summary>
        <p v-if="!item.activations.length">
          还没有安装。首次激活时才会建立安装记录。
        </p>
        <article v-for="a in item.activations" :key="a.id" class="installation">
          <p class="wrap">
            {{ a.label || a.id }} · {{ a.binding_type }} ·
            {{
              a.deactivated_at
                ? "已停用"
                : a.lease_expires_at
                  ? "租约至 " + formatTime(a.lease_expires_at)
                  : "节点锁定"
            }}
          </p>
          <small
            >最近验证：{{ formatTime(a.last_verified_at) }}
            {{ a.borrowed ? "· 离线借用尚未归还" : "" }}</small
          ><button
            v-if="!a.deactivated_at"
            class="btn secondary"
            :disabled="busy"
            @click="open('deactivate', item, a.id)"
          >
            停用此安装
          </button>
        </article>
      </details>
    </section>
    <FormDialog :open="!!op.kind" :title="title" @close="op.kind = ''"
      ><form class="operation" @submit.prevent="run(submit)">
        <label v-if="op.kind === 'renew'" class="field"
          >续费卡密<input
            v-model="op.card_code"
            type="password"
            autocomplete="off"
            required
        /></label>
        <p v-if="op.kind === 'renew'">
          卡密分别对应使用时长或更新时长，系统按购买用途续费，不重置已有功能、席位或计量记录。
        </p>
        <label v-if="op.kind === 'invite'" class="field"
          >邀请有效小时数<input
            v-model.number="op.hours"
            type="number"
            min="1"
            max="168"
            required /></label
        ><template v-if="op.kind === 'rebind'"
          ><label class="field"
            >绑定方式<select v-model="op.type" aria-label="绑定方式">
              <option
                v-for="kind in op.item.license.terms.binding_types.filter(
                  (k: string) => k !== 'seat',
                )"
                :key="kind"
                :value="kind"
              >
                {{ kind }}
              </option>
            </select></label
          ><label class="field"
            >新绑定值<input v-model="op.value" required /></label
          ><label v-if="op.type === 'domain'" class="field"
            >同组域名别名（须含新绑定值）<textarea
              v-model="op.aliases"
              placeholder="example.com&#10;www.example.com"
            ></textarea>
          </label>
          <p>
            换绑后全部旧安装失效，请在产品内重新激活。已有离线凭证仍受其自身到期限制；离线借用未到期时暂不能换绑。
          </p></template
        ><label
          v-if="['rebind', 'deactivate', 'appeal'].includes(op.kind)"
          class="field"
          >操作原因<input v-model="op.reason" required maxlength="500"
        /></label>
        <p v-if="error" class="error">{{ error }}</p>
        <button class="btn" :disabled="busy">确认{{ title }}</button>
      </form></FormDialog
    ><FormDialog
      :open="showInvitation"
      title="保存一次性安装邀请"
      @close="
        showInvitation = false;
        invitation = '';
      "
      ><p>
        将邀请信息填入实际产品的激活页。只显示本次；每张邀请只能关联一份安装公钥。
      </p>
      <textarea
        :value="invitation"
        readonly
        rows="7"
        aria-label="安装邀请"
      ></textarea
      ><button
        class="btn"
        @click="downloadText(invitation, 'installation-invitation.txt')"
      >
        下载保存邀请
      </button></FormDialog
    >
  </div>
</template>
<style scoped>
.panel {
  padding: 24px;
  margin: 22px 0;
}
.claim-grid,
.facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}
.claim-grid {
  align-items: end;
  margin-top: 20px;
}
.heading,
.actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.actions {
  justify-content: flex-start;
  margin: 20px 0;
}
.facts {
  margin: 24px 0;
}
.facts small,
.facts strong {
  display: block;
}
.facts small {
  color: var(--muted);
  margin-bottom: 7px;
}
.facts strong {
  font-size: 15px;
}
.wrap {
  overflow-wrap: anywhere;
}
.operation {
  display: grid;
  gap: 16px;
}
textarea {
  width: 100%;
}
.installation {
  padding: 16px 0;
  border-bottom: 1px solid var(--line);
}
.installation button {
  margin: 10px;
}
.entitlement {
  min-width: 0;
}
summary {
  cursor: pointer;
}
@media (max-width: 600px) {
  .panel {
    padding: 16px;
  }
  .claim-grid,
  .facts {
    grid-template-columns: 1fr;
  }
  .heading h2 {
    font-size: 19px;
  }
}
</style>
