<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { formatTime } from "../format";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  token?: string;
  endpoint: string;
  kind: "money" | "cases" | "appeals";
  params?: Record<string, string>;
  refresh?: number;
  selectable?: boolean;
  actionLabel?: string;
}>();
const emit = defineEmits<{ select: [row: any] }>();
const filters = reactive({ q: "", status: "", kind: "", from: "", to: "" }),
  page = ref(1),
  data = ref<any>({ items: [], total: 0, page_size: 20 }),
  busy = ref(false),
  error = ref("");
const columns = computed(() =>
  props.kind === "money"
    ? [
        ["created_at", "时间"],
        ["agent_id", "代理ID"],
        ["kind", "业务"],
        ["amount", "变动金额"],
        ["balance_after", "结余"],
        ["quantity", "数量"],
        ["unit_cost", "成本单价"],
        ["remark", "说明 / 套餐"],
        ["batch_id", "批次"],
      ]
    : props.kind === "cases"
      ? [
          ["license_no", "授权号"],
          ["signal", "线索"],
          ["subject", "对象"],
          ["status", "状态"],
          ["action", "处置动作"],
          ["occurrences", "次数"],
          ["last_seen", "最近时间"],
        ]
      : [
          ["case_id", "处置编号"],
          ["user_id", "用户"],
          ["reason", "申诉说明"],
          ["status", "状态"],
          ["created_at", "时间"],
        ],
);
const labels: Record<string, string> = {
  adjust: "余额调整",
  issue: "套餐发卡",
  open: "待处理",
  confirmed: "已确认处置",
  resolved: "已解除",
  closed: "已关闭",
  integrity_mismatch: "客户端完整性报告",
  binding_mismatch: "绑定不匹配",
  observe: "仅观察",
  deny_updates: "禁止更新",
  deny_use: "禁止使用",
  block_page: "显示拦截页",
};
function cell(row: any, key: string) {
  const v = row[key];
  if (["created_at", "last_seen"].includes(key)) return formatTime(v);
  if (["amount", "balance_after", "unit_cost"].includes(key))
    return `¥${(v / 100).toFixed(2)}`;
  if (["kind", "status", "signal", "action"].includes(key))
    return labels[v] || v;
  return v ?? "—";
}
function query() {
  const p = new URLSearchParams({
    ...props.params,
    page: String(page.value),
    page_size: "20",
  });
  for (const [k, v] of Object.entries(filters)) {
    if (v) p.set(k, k === "from" || k === "to" ? new Date(v).toISOString() : v);
  }
  return p.toString();
}
async function load(reset = false) {
  if (reset) page.value = 1;
  busy.value = true;
  error.value = "";
  try {
    data.value = await props.api(`${props.endpoint}?${query()}`);
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function download() {
  busy.value = true;
  error.value = "";
  try {
    const response = await fetch(`${props.endpoint}/export?${query()}`, {
      headers: { Authorization: `Bearer ${props.token || ""}` },
    });
    if (!response.ok) throw Error("导出失败，请确认登录状态与筛选条件。");
    const url = URL.createObjectURL(await response.blob());
    const a = document.createElement("a");
    a.href = url;
    a.download = `${props.kind}-history.csv`;
    a.click();
    URL.revokeObjectURL(url);
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
watch(
  () => [props.endpoint, JSON.stringify(props.params)],
  () => load(true),
  { immediate: true },
);
watch(
  () => props.refresh,
  () => load(),
);
</script>
<template>
  <div class="paged-records">
    <form class="history-filters" @submit.prevent="load(true)">
      <label class="field"
        >搜索历史记录<input
          v-model="filters.q"
          placeholder="编号、授权号或说明"
          maxlength="100" /></label
      ><label v-if="kind !== 'money'" class="field"
        >记录状态<select v-model="filters.status">
          <option value="">全部</option>
          <option value="open">待处理</option>
          <template v-if="kind === 'cases'"
            ><option value="confirmed">已确认处置</option>
            <option value="resolved">已解除</option></template
          >
          <option v-else value="closed">已关闭</option>
        </select></label
      ><label v-if="kind !== 'appeals'" class="field"
        >记录类型<select v-model="filters.kind">
          <option value="">全部</option>
          <template v-if="kind === 'money'"
            ><option value="adjust">余额调整</option>
            <option value="issue">套餐发卡</option></template
          ><template v-else
            ><option value="binding_mismatch">绑定不匹配</option>
            <option value="integrity_mismatch">完整性报告</option></template
          >
        </select></label
      ><label class="field"
        >开始时间<input v-model="filters.from" type="datetime-local" /></label
      ><label class="field"
        >结束时间（不含）<input v-model="filters.to" type="datetime-local"
      /></label>
      <div class="buttons">
        <button class="btn secondary small" :disabled="busy">
          查询历史记录</button
        ><button
          v-if="token"
          type="button"
          class="btn secondary small"
          :disabled="busy"
          @click="download"
        >
          导出全部筛选结果
        </button>
      </div>
    </form>
    <p v-if="error" role="alert" class="error">{{ error }}</p>
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th v-for="c in columns" :key="c[0]">{{ c[1] }}</th>
            <th v-if="selectable">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in data.items" :key="row.id">
            <td v-for="c in columns" :key="c[0]">{{ cell(row, c[0]) }}</td>
            <td v-if="selectable">
              <button
                type="button"
                class="btn secondary small"
                @click="emit('select', row)"
              >
                {{ actionLabel || "查看与核实" }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="!data.items.length" class="empty">未找到符合条件的历史记录。</p>
    </div>
    <footer>
      <span
        >共 {{ data.total }} 条 · 第 {{ page }} /
        {{ Math.max(1, Math.ceil(data.total / data.page_size)) }} 页</span
      >
      <div class="buttons">
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
    </footer>
  </div>
</template>
<style scoped>
.history-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  align-items: end;
  margin: 20px 0;
}
.field {
  min-width: 150px;
  flex: 1;
}
.buttons {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}
.table td {
  max-width: 310px;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  font-size: 12px;
}
footer {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: center;
  flex-wrap: wrap;
  margin-top: 20px;
  color: var(--muted);
  font-size: 12px;
}
.error {
  color: var(--red);
}
.paged-records {
  min-width: 0;
}
</style>
