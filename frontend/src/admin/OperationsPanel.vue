<script setup lang="ts">
import { onMounted, ref } from "vue";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  canRetry?: boolean;
}>();
const data = ref<any>(),
  error = ref(""),
  busy = ref(false);
const bytes = (n: number) => `${(n / 1024 / 1024 / 1024).toFixed(2)} GiB`;
const names: Record<string, string> = {
  activation: "授权激活",
  verification: "授权验证",
  heartbeat: "心跳",
  renewal: "续费",
  update_check: "检查更新",
  update_download: "下载更新",
  management: "管理后台",
  user: "用户中心",
  developer: "开发者",
  public: "公开接口",
  other: "其他",
};
async function load() {
  busy.value = true;
  error.value = "";
  try {
    data.value = await props.api("/admin/operations");
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function retry() {
  busy.value = true;
  try {
    await props.api("/admin/operations/notifications/retry", {
      method: "POST",
      body: "{}",
    });
    await load();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
onMounted(load);
</script>
<template>
  <section class="panel operations">
    <div class="panel-head">
      <div>
        <h3>运行与容量</h3>
        <p>检查授权站就绪状态、升级包容量和业务请求，及时处理服务异常。</p>
      </div>
      <button class="btn secondary" :disabled="busy" @click="load">
        刷新运行状态
      </button>
    </div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <template v-if="data">
      <div class="metrics">
        <div class="metric">
          <strong>{{ data.readiness.database ? "正常" : "异常" }}</strong
          ><small>数据库与审计写入</small>
        </div>
        <div class="metric">
          <strong>{{ data.readiness.storage ? "正常" : "异常" }}</strong
          ><small>升级包存储</small>
        </div>
        <div class="metric">
          <strong>{{ bytes(data.storage.usage.total_bytes) }}</strong
          ><small
            >升级包已用容量 /
            {{ bytes(data.storage.limits.total_bytes) }}</small
          >
        </div>
        <div class="metric">
          <strong>{{ data.storage.usage.total_versions }}</strong
          ><small>保留的升级版本</small>
        </div>
      </div>
      <h4>容量保护</h4>
      <p>
        每个应用 {{ bytes(data.storage.limits.product_bytes) }} /
        {{ data.storage.limits.product_versions }} 个版本；每个开发者
        {{ bytes(data.storage.limits.developer_bytes) }}；同时上传上限
        {{ data.storage.limits.concurrent_uploads }}、下载上限
        {{
          data.storage.limits.concurrent_downloads
        }}。容量参数由部署配置管理，修改后重启生效。
      </p>
      <p>
        发布中的版本不能删除；先下架，再删除文件可释放额度。历史发卡与资金流水不会因容量清理删除。
      </p>
      <h4>通知投递与存储</h4>
      <p>
        磁盘剩余 {{ bytes(data.readiness.disk_free_bytes) }}。待发送
        {{ data.notifications.pending }}；重试中
        {{ data.notifications.retry }}；发送中
        {{ data.notifications.sending }}；已发送
        {{ data.notifications.sent }}；最终失败
        {{ data.notifications.dead }}。失败通知最多重试 12
        次，可在修复邮箱配置后重新投递。
      </p>
      <div v-if="canRetry && data.notifications.dead" class="retry-row">
        <button class="btn secondary" :disabled="busy" @click="retry">
          重试失败通知
        </button>
      </div>
      <h4>业务请求</h4>
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>业务</th>
              <th>请求数</th>
              <th>服务异常</th>
              <th>业务拒绝</th>
              <th>平均响应</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="m in data.metrics" :key="m.operation">
              <td>{{ names[m.operation] || m.operation }}</td>
              <td>{{ m.requests }}</td>
              <td>{{ m.errors }}</td>
              <td>{{ m.denials }}</td>
              <td>
                {{
                  m.requests
                    ? ((m.duration_seconds / m.requests) * 1000).toFixed(1)
                    : "0"
                }}
                ms
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p>
        {{
          data.external_metrics_enabled
            ? "已启用受令牌保护的外部监控指标。"
            : "外部监控尚未启用，请由运维配置指标令牌与告警接收端。"
        }}计数自进程启动累积，长期趋势应由外部监控保存。
      </p>
    </template>
  </section>
</template>
<style scoped>
.operations h4,
.operations > p {
  padding: 0 24px;
}
.operations > p {
  color: var(--muted);
  line-height: 1.8;
}
.operations .metrics {
  padding: 20px;
}
.operations {
  min-width: 0;
  max-width: 100%;
}
.operations .table-wrap {
  margin: 20px;
  width: calc(100% - 40px);
  min-width: 0;
}
.operations .metric {
  min-width: 0;
}
.retry-row {
  padding: 0 24px;
}
</style>
