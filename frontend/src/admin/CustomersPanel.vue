<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  canWrite: boolean;
}>();
const users = ref<any[]>([]),
  error = ref(""),
  message = ref(""),
  busy = ref(false),
  license = ref<any>();
const form = reactive({ username: "", password: "", display_name: "" }),
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
    await load();
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
    <h2>普通用户管理</h2>
    <p>
      用户只能管理归属于自己的授权。用户账号与管理员、代理、开发者账号相互隔离。
    </p>
    <p class="error" role="alert" v-if="error">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>
    <form v-if="canWrite" class="customer-form" @submit.prevent="create">
      <label class="field"
        >用户登录名<input
          v-model="form.username"
          required
          maxlength="80"
          autocomplete="off" /></label
      ><label class="field"
        >显示名称<input v-model="form.display_name" maxlength="160" /></label
      ><label class="field"
        >初始密码<input
          v-model="form.password"
          type="password"
          required
          minlength="8"
          autocomplete="new-password" /></label
      ><button class="btn" :disabled="busy">创建普通用户</button>
    </form>
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>用户</th>
            <th>名称</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td>{{ u.username }}</td>
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
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <template v-if="canWrite"
      ><h3>分配已有授权</h3>
      <p>
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
      </form></template
    >
  </section>
</template>
<style scoped>
.customer-form {
  display: grid;
  gap: 14px;
  max-width: 680px;
  margin: 22px 0;
}
.error {
  color: #a23535;
}
</style>
