<script setup lang="ts">
import { onMounted, ref } from "vue";
import { downloadText } from "../shared/api";
const props = defineProps<{
  api: (path: string, options?: RequestInit) => Promise<any>;
}>();
const rows = ref<any[]>([]),
  error = ref("");
onMounted(async () => {
  try {
    rows.value = await props.api("/v2/platform/authorization-archive");
  } catch (e: any) {
    error.value = e.message;
  }
});
</script>
<template>
  <section class="panel page-stack">
    <h2>历史授权档案</h2>
    <p>
      旧协议已经停用，历史数据原样保留。账号、代理余额、等级及平台配置继续使用。
    </p>
    <p>
      旧许可证与新协议的安装签名、席位和期限含义不同，因此不会自动转换为可用授权。请在应用授权中心建立应用与套餐，核对客户剩余使用和更新期限后重新签发，再让客户用新版接入激活。
    </p>
    <p>
      旧卡密不再提供明文恢复或导出，旧令牌不能调用新授权与更新接口。完整数据库及升级包可从“运行与容量”备份，重新签发前请保留备份。
    </p>
    <p v-if="error" role="alert">{{ error }}</p>
    <button
      class="btn secondary"
      @click="
        downloadText(
          JSON.stringify(
            { protocol: 'retired', automatic_conversion: false, items: rows },
            null,
            2,
          ),
          'legacy-authorization-inventory.json',
        )
      "
    >
      导出历史盘点报告
    </button>
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>历史产品</th>
            <th>原应用 ID</th>
            <th>许可证</th>
            <th>卡密</th>
            <th>版本文件</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.product_id">
            <td>{{ r.name }}</td>
            <td>{{ r.product_id }}</td>
            <td>{{ r.licenses }}</td>
            <td>{{ r.cards }}</td>
            <td>{{ r.releases }}</td>
          </tr>
          <tr v-if="!rows.length">
            <td colspan="5">没有历史授权数据，可直接开始创建应用。</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
