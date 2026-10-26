<script setup lang="ts">
import { computed, ref, watch } from "vue";
import FormDialog from "../shared/components/FormDialog.vue";
import { friendlyError } from "../shared/errors";
const props = defineProps<{
  open: boolean;
  initialUserId?: string;
  products: any[];
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
}>();
const emit = defineEmits<{ close: []; opened: [result: any] }>();
const mode = ref("new"),
  users = ref<any[]>([]),
  catalog = ref<any>({ packages: [], tiers: [], groups: [] }),
  busy = ref(false),
  error = ref("");
const form = ref({
  user_id: "",
  username: "",
  password: "",
  display_name: "",
  email: "",
  name: "",
  tier_id: "",
  package_ids: [] as string[],
  group_ids: [] as string[],
  initial_balance: 0,
  reason: "",
  max_batch_quantity: 100,
  can_export: false,
});
const selectedUser = computed(() =>
  users.value.find((u) => u.id === form.value.user_id),
);
const pname = (id: string) =>
  props.products.find((p) => p.id === id)?.name || id;
watch(
  () => props.open,
  async (open) => {
    if (!open) return;
    mode.value = props.initialUserId ? "existing" : "new";
    form.value = {
      user_id: props.initialUserId || "",
      username: "",
      password: "",
      display_name: "",
      email: "",
      name: "",
      tier_id: "",
      package_ids: [],
      group_ids: [],
      initial_balance: 0,
      reason: "",
      max_batch_quantity: 100,
      can_export: false,
    };
    error.value = "";
    busy.value = true;
    try {
      [users.value, catalog.value] = await Promise.all([
        props.api<any[]>("/admin/agent-accounts/eligible-users"),
        props.api("/admin/commerce"),
      ]);
    } catch (e) {
      error.value = friendlyError(e);
    } finally {
      busy.value = false;
    }
  },
);
async function submit() {
  busy.value = true;
  error.value = "";
  try {
    const { user_id, username, password, display_name, email, ...business } =
      form.value;
    const identity =
      mode.value === "existing"
        ? { user_id }
        : { username, password, display_name, email };
    const result = await props.api("/admin/agent-accounts", {
      method: "POST",
      body: JSON.stringify({ ...business, ...identity }),
    });
    form.value.password = "";
    emit("opened", result);
    emit("close");
  } catch (e) {
    error.value = friendlyError(e);
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <FormDialog
    :open="open"
    title="开通代理账号"
    description="一次开通即可登录。用户中心与代理工作台共用账号，等级和经营余额归代理资格。"
    @close="emit('close')"
  >
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <form class="form-grid" @submit.prevent="submit">
      <label class="field wide"
        >账号来源<select v-model="mode">
          <option value="new">创建新用户并开通代理</option>
          <option value="existing">为已有用户开通代理资格</option>
        </select></label
      >
      <template v-if="mode === 'new'">
        <label class="field"
          >登录账号<input
            v-model="form.username"
            required
            maxlength="80"
            autocomplete="off"
        /></label>
        <label class="field"
          >初始密码<input
            v-model="form.password"
            type="password"
            required
            minlength="8"
            maxlength="256"
            autocomplete="new-password"
        /></label>
        <label class="field"
          >用户显示名称<input v-model="form.display_name" maxlength="160"
        /></label>
        <label class="field"
          >账号邮箱<input
            v-model="form.email"
            type="email"
            maxlength="254"
            placeholder="用于账号找回，不使用代理业务联系人邮箱"
        /></label>
      </template>
      <label v-else class="field wide"
        >选择已有用户<select v-model="form.user_id" required>
          <option value="" disabled>选择未开通代理的用户</option>
          <option v-for="u in users" :key="u.id" :value="u.id">
            {{ u.display_name || u.username }} · {{ u.username }}
          </option></select
        ><small v-if="selectedUser"
          >使用该用户原有密码和个人邮箱，不会重置账号。</small
        ></label
      >
      <label class="field"
        >代理显示名称<input
          v-model="form.name"
          maxlength="160"
          placeholder="留空使用用户显示名称或登录账号"
      /></label>
      <label class="field"
        >代理等级<select v-model="form.tier_id">
          <option value="">基础等级 · 按套餐基础价</option>
          <option v-for="t in catalog.tiers" :key="t.id" :value="t.id">
            {{ t.name }}
          </option>
        </select></label
      >
      <fieldset class="wide selection">
        <legend>允许销售的套餐</legend>
        <p>
          选择具体应用的套餐，或按分组授予当前已上架的套餐。也可开通后再配置。
        </p>
        <label
          v-for="p in catalog.packages.filter((p: any) => p.enabled)"
          :key="p.id"
          class="check"
          ><input v-model="form.package_ids" type="checkbox" :value="p.id" />{{
            pname(p.product_id)
          }}
          / {{ p.name }}</label
        >
        <p v-if="!catalog.packages.length">
          尚未建立套餐，可在“套餐与结算”中配置。
        </p>
        <label v-if="catalog.groups.length" class="field"
          >按应用分组批量授权<select v-model="form.group_ids" multiple>
            <option v-for="g in catalog.groups" :key="g.id" :value="g.id">
              {{ g.name }}
            </option>
          </select></label
        >
      </fieldset>
      <label class="field"
        >初始预存余额（分）<input
          v-model.number="form.initial_balance"
          type="number"
          min="0"
          max="1000000000000"
          required
        /><small>100 分 = 1 元；零余额可登录，发卡需有足够余额。</small></label
      >
      <label class="field"
        >余额登记原因<input
          v-model="form.reason"
          :required="form.initial_balance > 0"
          maxlength="500"
          placeholder="记录款项来源或赠送原因"
      /></label>
      <label class="field"
        >单批发卡上限<input
          v-model.number="form.max_batch_quantity"
          type="number"
          min="1"
          max="1000"
          required
      /></label>
      <label class="check"
        ><input
          v-model="form.can_export"
          type="checkbox"
        />允许批量导出完整卡密</label
      >
      <p class="wide">
        代理等级只决定成本，不会自动扩大可售应用范围。普通用户自己的授权归属不因开通代理而改变。
      </p>
      <div class="form-actions">
        <button class="btn" :disabled="busy">
          {{ busy ? "正在开通…" : "确认开通代理" }}
        </button>
      </div>
    </form>
  </FormDialog>
</template>
<style scoped>
.selection {
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 16px;
  min-width: 0;
}
.selection p,
.form-grid > p,
small {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.8;
}
.selection .check {
  display: flex;
  align-items: center;
  gap: 8px;
  line-height: 1.8;
  margin: 8px 0;
  overflow-wrap: anywhere;
}
.check input {
  width: auto;
  flex: none;
}
</style>
