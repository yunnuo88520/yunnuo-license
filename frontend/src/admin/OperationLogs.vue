<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { formatTime } from "../shared/format";
import { downloadText } from "../shared/api";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  token: string;
}>();
const labels: Record<string, string> = {
  account: "账号安全",
  users: "用户管理",
  applications: "应用管理",
  agents: "代理管理",
  cards: "卡密发放",
  licenses: "授权管理",
  binding: "换绑规则",
  ownership: "授权归属",
  renewal: "续费",
  updates: "版本更新",
  security: "安全处置",
  audit: "日志审计",
  platform: "平台设置",
  packages: "套餐与成本",
  finance: "资金流水",
};
const actorLabels: Record<string, string> = {
  customer: "普通用户",
  admin: "管理员",
  agent: "代理",
  developer: "开发者",
  application: "应用服务端",
  anonymous: "未登录",
};
const resultLabels: Record<string, string> = {
  success: "成功",
  failed: "失败 / 已拒绝",
  pending: "处理中",
};
const filters = reactive({
    category: "",
    actor: "",
    result: "",
    q: "",
    from: "",
    to: "",
  }),
  page = ref(1),
  data = ref<any>({ items: [], total: 0, page_size: 30 }),
  error = ref(""),
  busy = ref(false);
function query() {
  const q = new URLSearchParams({ page: String(page.value), page_size: "30" });
  for (const [k, v] of Object.entries(filters)) {
    if (v) q.set(k, k === "from" || k === "to" ? new Date(v).toISOString() : v);
  }
  return q.toString();
}
async function load() {
  busy.value = true;
  error.value = "";
  try {
    data.value = await props.api("/admin/operation-logs?" + query());
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function exportLogs() {
  error.value = "";
  try {
    const response = await fetch("/admin/operation-logs/export?" + query(), {
      headers: { Authorization: "Bearer " + props.token },
    });
    if (!response.ok) throw Error("导出失败");
    downloadText("\uFEFF" + (await response.text()), "operation-logs.csv");
  } catch (e: any) {
    error.value = e.message;
  }
}
onMounted(load);
</script>
<template>
  <section>
    <div class="head">
      <div>
        <h2>全操作记录</h2>
        <p>
          记录业务接口的成功、失败和越权请求。日志不保存请求中的密码、卡密或访问令牌。
        </p>
      </div>
      <button class="btn secondary" :disabled="busy" @click="exportLogs">
        导出筛选结果（最多 5000 条）
      </button>
    </div>
    <form
      class="filters"
      @submit.prevent="
        page = 1;
        load();
      "
    >
      <label class="field"
        >分类标签<select v-model="filters.category">
          <option value="">全部分类</option>
          <option v-for="(name, key) in labels" :key="key" :value="key">
            {{ name }}
          </option>
        </select></label
      ><label class="field"
        >操作者<select v-model="filters.actor">
          <option value="">全部</option>
          <option value="admin">管理员</option>
          <option value="agent">代理</option>
          <option value="customer">普通用户</option>
          <option value="developer">开发者</option>
          <option value="application">应用服务端</option>
          <option value="anonymous">未登录</option>
        </select></label
      ><label class="field"
        >结果<select v-model="filters.result">
          <option value="">全部</option>
          <option value="success">成功</option>
          <option value="failed">失败 / 拒绝</option>
          <option value="pending">处理中 / 待核对</option>
        </select></label
      ><label class="field"
        >操作者 / 授权编号 / 路由 / 错误码<input
          v-model="filters.q"
          maxlength="100" /></label
      ><label class="field"
        >开始时间<input v-model="filters.from" type="datetime-local" /></label
      ><label class="field"
        >结束时间<input v-model="filters.to" type="datetime-local" /></label
      ><button class="btn" :disabled="busy">查询操作日志</button>
    </form>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>时间</th>
            <th>分类</th>
            <th>操作者</th>
            <th>动作</th>
            <th>作用对象</th>
            <th>结果</th>
            <th>错误码</th>
            <th>请求编号</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="l in data.items" :key="l.id">
            <td>{{ formatTime(l.created_at) }}</td>
            <td>
              <span class="tag">{{ labels[l.category] || l.category }}</span>
            </td>
            <td>
              {{ actorLabels[l.actor_type] || l.actor_type }}<br /><small>{{
                l.actor_id || "—"
              }}</small>
            </td>
            <td>
              {{ l.method }}<br /><code>{{ l.route }}</code>
            </td>
            <td>
              <small>{{ l.object_id || "—" }}</small>
            </td>
            <td :class="l.result === 'failed' ? 'error' : ''">
              {{ resultLabels[l.result] }}
              · {{ l.http_status || "—" }}
            </td>
            <td>{{ l.error_code || "—" }}</td>
            <td>
              <small>{{ l.request_id }}</small>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div class="head">
      <span>共 {{ data.total }} 条 · 第 {{ page }} 页</span>
      <div>
        <button
          class="btn secondary small"
          :disabled="busy || page <= 1"
          @click="
            page--;
            load();
          "
        >
          上一页</button
        ><button
          class="btn secondary small"
          :disabled="busy || page * data.page_size >= data.total"
          @click="
            page++;
            load();
          "
        >
          下一页
        </button>
      </div>
    </div>
    <p class="hint">
      请求开始前先写入日志；若结束记录失败，该条保留“处理中 /
      待核对”，不会静默丢失。关键归属、换绑、用户续费操作还在业务事务内写入审计。
    </p>
  </section>
</template>
<style scoped>
section {
  padding: 4px 20px 20px;
}
.head {
  display: flex;
  justify-content: space-between;
  gap: 14px;
  flex-wrap: wrap;
  align-items: center;
  margin: 12px 0 16px;
}
.head h2 {
  margin: 0;
  font-size: 16px;
}
.head p {
  margin: 5px 0 0;
  color: var(--muted);
  font-size: 12px;
}
.filters {
  display: grid;
  grid-template-columns: repeat(3, minmax(140px, 1fr));
  gap: 14px;
  margin-bottom: 20px;
}
.tag {
  display: inline-block;
  border-radius: 16px;
  padding: 4px 9px;
  background: var(--green-soft);
  white-space: nowrap;
}
.error {
  color: var(--red);
}
small {
  font-size: 11px;
  overflow-wrap: anywhere;
}
.hint {
  color: var(--muted);
  font-size: 13px;
}
code {
  font-size: 12px;
}
@media (max-width: 700px) {
  .filters {
    grid-template-columns: 1fr;
  }
}
</style>
