<script setup lang="ts">
import { onMounted, ref, computed } from "vue";
const props = defineProps<{
  api: (path: string, options?: RequestInit) => Promise<any>;
}>();
const rows = ref<any[]>([]),
  error = ref(""),
  loading = ref(false);
const totals = computed(() =>
  rows.value.reduce(
    (v, r) => ({
      applications: v.applications + 1,
      licenses: v.licenses + r.diagnostics.license_total,
      activations: v.activations + r.diagnostics.active_activations,
      expiring: v.expiring + r.diagnostics.expiring_licenses,
    }),
    { applications: 0, licenses: 0, activations: 0, expiring: 0 },
  ),
);
async function load() {
  loading.value = true;
  error.value = "";
  try {
    const apps = await props.api("/v2/console/applications");
    const next = [];
    for (const app of apps) {
      next.push({
        app,
        diagnostics: await props.api(
          `/v2/console/applications/${app.application_id}/diagnostics`,
        ),
      });
    }
    rows.value = next;
  } catch (e: any) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}
onMounted(load);
</script>
<template>
  <div class="page-stack">
    <div class="page-head">
      <h2>授权业务概览</h2>
      <p>统计来自当前应用授权核心，历史档案不计入有效业务。</p>
    </div>
    <p v-if="error" role="alert">{{ error }}</p>
    <div class="stat-grid">
      <div
        v-for="(label, key) in {
          applications: '应用数量',
          licenses: '许可证总数',
          activations: '活跃安装 / 租约',
          expiring: '30 天内到期',
        }"
        :key="key"
        class="stat-card"
      >
        <span class="stat-body"
          ><small>{{ label }}</small
          ><strong>{{ totals[key] }}</strong></span
        >
      </div>
    </div>
    <section class="panel">
      <div class="panel-head">
        <h3>应用运行情况</h3>
        <button class="btn secondary" :disabled="loading" @click="load">
          刷新统计
        </button>
      </div>
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>应用</th>
              <th>分组</th>
              <th>许可证</th>
              <th>活跃安装</th>
              <th>即将到期</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in rows" :key="r.app.application_id">
              <td>
                {{ r.app.name }}<br /><small>{{ r.app.application_id }}</small>
              </td>
              <td>{{ r.app.group || "未分组" }}</td>
              <td>{{ r.diagnostics.license_total }}</td>
              <td>{{ r.diagnostics.active_activations }}</td>
              <td>{{ r.diagnostics.expiring_licenses }}</td>
            </tr>
            <tr v-if="!rows.length">
              <td colspan="5">请先在应用授权中心创建应用、套餐与许可证。</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <section class="panel">
      <h3>开始交付产品</h3>
      <p>
        创建应用 → 配置套餐 → 确认收款并发卡或预录域名 / IP → 下载接入代码 →
        客户激活。相同发行包可交付多个客户。
      </p>
      <a class="btn" href="#applications">进入应用授权中心</a>
    </section>
  </div>
</template>
