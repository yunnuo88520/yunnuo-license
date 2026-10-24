<script setup lang="ts">
import PagedRecords from "../shared/components/PagedRecords.vue";
import { onMounted, ref } from "vue";
const props = defineProps<{
  token: string;
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
}>();
const cases = ref<any[]>([]),
  appeals = ref<any[]>([]),
  selected = ref(""),
  reason = ref(""),
  message = ref(""),
  error = ref(""),
  busy = ref(false),
  refreshKey = ref(0);
async function load() {
  refreshKey.value++;
  [cases.value, appeals.value] = await Promise.all([
    props.api("/user/security/cases"),
    props.api("/user/security/appeals"),
  ]);
}
async function submit() {
  busy.value = true;
  error.value = "";
  try {
    await props.api(`/user/security/cases/${selected.value}/appeal`, {
      method: "POST",
      body: JSON.stringify({ reason: reason.value }),
    });
    message.value = "申诉已提交，管理员将核实处理。";
    selected.value = "";
    reason.value = "";
    await load();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
onMounted(() => load().catch((e: any) => (error.value = e.message)));
</script>
<template>
  <section v-if="cases.length || error" class="panel">
    <h2>授权安全记录与申诉</h2>
    <p>
      待核实线索不会自动改变授权；如对已确认处置有异议，可提交部署和权属说明。
    </p>
    <p v-if="message" role="status">{{ message }}</p>
    <p v-if="error" role="alert">{{ error }}</p>
    <article v-for="c in cases.slice(0, 10)" :key="c.id">
      <h3>
        {{
          c.signal === "binding_mismatch" ? "绑定异常线索" : "升级包完整性线索"
        }}
      </h3>
      <code>{{ c.id }}</code>
      <p>
        {{ c.license_no }} ·
        {{
          c.status === "open"
            ? "待核实"
            : c.status === "confirmed"
              ? "已确认处置"
              : "已解除"
        }}
      </p>
      <p v-if="appeals.find((a) => a.case_id === c.id)">
        申诉状态：{{
          appeals.find((a) => a.case_id === c.id)?.status === "open"
            ? "待管理员处理"
            : "已关闭"
        }}
      </p>
      <button
        v-if="
          c.status !== 'resolved' &&
          !appeals.some((a) => a.case_id === c.id && a.status === 'open')
        "
        class="btn secondary"
        @click="selected = c.id"
      >
        提交申诉
      </button>
    </article>
    <details>
      <summary>查询全部安全历史与申诉</summary>
      <PagedRecords
        :api="api"
        :token="token"
        :refresh="refreshKey"
        endpoint="/user/history/cases"
        kind="cases"
        selectable
        action-label="选择申诉记录"
        @select="
          (row: any) => {
            if (row.status !== 'resolved') selected = row.id;
          }
        "
      /><PagedRecords
        :api="api"
        :token="token"
        :refresh="refreshKey"
        endpoint="/user/history/appeals"
        kind="appeals"
      />
    </details>
    <form v-if="selected" @submit.prevent="submit">
      <label class="field"
        >申诉说明<textarea
          v-model="reason"
          required
          maxlength="1000"
          placeholder="说明应用部署、版本和授权来源"
        ></textarea></label
      ><button class="btn" :disabled="busy">发送申诉给管理员</button>
    </form>
  </section>
</template>
<style scoped>
.panel {
  padding: 26px;
  margin: 24px 0;
  border-radius: 12px;
}
article {
  padding: 20px 0;
  border-bottom: 1px solid #d9e1dc;
}
code {
  overflow-wrap: anywhere;
}
form {
  display: grid;
  gap: 16px;
  margin-top: 24px;
}
textarea {
  min-height: 100px;
}
</style>
