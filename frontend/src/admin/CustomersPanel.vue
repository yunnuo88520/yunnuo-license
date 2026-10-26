<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import FormDialog from "../shared/components/FormDialog.vue";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  canWrite: boolean;
}>();
const emit = defineEmits<{ openAgent: [userID: string] }>();
const users = ref<any[]>([]),
  error = ref(""),
  message = ref(""),
  busy = ref(false),
  license = ref<any>(),
  showCreate = ref(false);
const form = reactive({ username: "", password: "", display_name: "", email: "" }),
  owner = reactive({ license_no: "", user_id: "", reason: "" });
async function load() {
  users.value = await props.api("/admin/customers");
}
async function run(fn: () => Promise<unknown>) {
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
function create() {
  void run(async () => {
    await props.api("/admin/customers", {
      method: "POST",
      body: JSON.stringify(form),
    });
    form.password = "";
    form.username = "";
    form.display_name = "";
    form.email = "";
    await load();
    showCreate.value = false;
    message.value = "普通用户账号已创建。";
  });
}
function status(u: any) {
  void run(async () => {
    await props.api(
      `/admin/customers/${u.id}/${u.status === "active" ? "disable" : "enable"}`,
      { method: "POST" },
    );
    await load();
  });
}
function lookup() {
  void run(async () => {
    const result = await props.api(
      `/admin/licenses?q=${encodeURIComponent(owner.license_no)}`,
    );
    license.value = result.items.find(
      (l: any) => l.license_no === owner.license_no,
    );
    if (!license.value) throw Error("未找到该授权");
    owner.user_id = license.value.owner_user_id || "";
  });
}
function assign() {
  void run(async () => {
    await props.api(`/admin/licenses/${license.value.license_no}/owner`, {
      method: "POST",
      body: JSON.stringify({
        user_id: owner.user_id,
        expected_owner: license.value.owner_user_id || "",
        reason: owner.reason,
      }),
    });
    license.value = null;
    owner.reason = "";
    message.value = "授权归属已更新，操作已审计。";
  });
}
onMounted(() => run(load));
</script>
<template>
  <section class="panel">
    <div class="panel-head">
      <div>
        <h3>普通用户管理</h3>
        <p>
          用户管理自己的授权；开通代理资格后，同一账号可进入代理工作台。停用用户将同时停止该账号的代理访问。
        </p>
      </div>
      <button
        v-if="canWrite"
        class="btn small"
        type="button"
        @click="showCreate = true"
      >
        ＋ 新增用户
      </button>
    </div>
    <div class="panel-body">
      <p class="error" role="alert" v-if="error">{{ error }}</p>
      <p v-if="message" role="status">{{ message }}</p>
    </div>
    <FormDialog
      :open="showCreate"
      title="新增普通用户"
      description="用户首次登录后可自行管理名下授权。"
      @close="showCreate = false"
    >
      <form class="customer-form in-dialog" @submit.prevent="create">
        <label class="field"
          >用户登录名<input
            v-model="form.username"
            required
            maxlength="80"
            autocomplete="off" /></label
        ><label class="field"
          >显示名称<input v-model="form.display_name" maxlength="160" /></label
        ><label class="field"
          >邮箱<input
            v-model="form.email"
            type="email"
            maxlength="254"
            placeholder="用于自助找回密码与通知，建议填写" /></label
        ><label class="field"
          >初始密码<input
            v-model="form.password"
            type="password"
            required
            minlength="8"
            maxlength="256"
            autocomplete="new-password" /></label
        ><div class="form-actions">
          <button class="btn" :disabled="busy">创建用户</button>
        </div>
      </form>
    </FormDialog>
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>用户</th>
            <th>邮箱</th>
            <th>名称</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td>{{ u.username }}</td>
            <td><code>{{ u.email || "—" }}</code></td>
            <td>{{ u.display_name || "—" }}</td>
            <td>{{ u.status === "active" ? "正常" : "停用" }}</td>
            <td>
              <button
                v-if="canWrite"
                class="btn secondary small"
                :disabled="busy"
                @click="status(u)"
              >
                {{ u.status === "active" ? "停用账号" : "恢复账号" }}
              </button>
              <button v-if="canWrite && u.status === 'active' && !u.agent_id" class="btn secondary small" @click="emit('openAgent', u.id)">开通代理</button>
              <span v-if="u.agent_id">已开通代理</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="canWrite" class="panel-body">
      <h3>分配已有授权</h3>
      <p class="assign-hint">
        首次归属可由管理员分配。已有归属默认禁止转移，启用后仍需由管理员填写原因并执行。
      </p>
      <form class="customer-form" @submit.prevent="lookup">
        <label class="field"
          >待分配授权编号<input v-model="owner.license_no" required /></label
        ><button class="btn secondary" :disabled="busy">查找授权</button>
      </form>
      <form v-if="license" class="customer-form" @submit.prevent="assign">
        <p>
          当前归属：{{
            users.find((u: any) => u.id === license.owner_user_id)?.username ||
            license.owner_user_id ||
            "未分配"
          }}
        </p>
        <label class="field"
          >目标用户<select v-model="owner.user_id" required>
            <option
              v-for="u in users.filter((u: any) => u.status === 'active')"
              :key="u.id"
              :value="u.id"
            >
              {{ u.display_name || u.username }}
            </option>
          </select></label
        ><label class="field"
          >操作原因<input
            v-model="owner.reason"
            required
            maxlength="500" /></label
        ><button class="btn" :disabled="busy">确认分配归属</button>
      </form>
    </div>
  </section>
</template>
<style scoped>
.customer-form {
  display: grid;
  gap: 14px;
  max-width: 680px;
  margin: 0 0 18px;
}
.customer-form.in-dialog {
  margin: 0;
  max-width: 480px;
}
.panel-body h3 {
  margin: 0 0 6px;
  font-size: 15px;
}
.assign-hint {
  margin: 0 0 14px;
  color: var(--muted);
  font-size: 12px;
}
.error {
  color: var(--red);
}
</style>
