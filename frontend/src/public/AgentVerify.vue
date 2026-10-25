<script setup lang="ts">
import { reactive, ref } from "vue";
import { request } from "../shared/api";
import { formatTime } from "../shared/format";
const form = reactive({ agent_no: "", application_code: "" }),
  result = ref<any>(),
  error = ref(""),
  busy = ref(false);
async function submit() {
  busy.value = true;
  error.value = "";
  result.value = null;
  try {
    result.value = await request("/v1/agents/query", {
      method: "POST",
      body: JSON.stringify(form),
    });
  } catch (e: any) {
    error.value =
      e.code === "AGENT_NOT_FOUND"
        ? "未找到该公开代理编号，请核对后重试。"
        : "查询失败，请稍后重试。";
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <div class="agent-verify">
    <form @submit.prevent="submit">
      <label class="field"
        >公开代理编号<input
          v-model="form.agent_no"
          placeholder="AGT_ 开头的公开编号"
          required
          minlength="8"
          maxlength="80"
          autocomplete="off" /></label
      ><label class="field"
        >应用代码（可选）<input
          v-model="form.application_code"
          placeholder="例如 EBOX，核对该应用的代理权限"
          maxlength="32" /></label
      ><button class="btn" :disabled="busy">
        {{ busy ? "查询中" : "核验代理身份" }}
      </button>
      <p>
        请核对交易对象提供的公开编号及当前可售应用。核验结果以本次查询时间为准。
      </p>
    </form>
    <p class="error" v-if="error" role="alert">{{ error }}</p>
    <section v-if="result" class="agent-result" aria-live="polite">
      <span class="tag" :class="{ denied: !result.is_authorized }">{{
        result.is_authorized
          ? "当前代理权限有效"
          : result.status === "active"
            ? "当前无匹配应用的代理权限"
            : "代理权限已暂停或停用"
      }}</span>
      <h2>{{ result.name }}</h2>
      <code>{{ result.agent_no }}</code>
      <p>
        平台登记状态：{{
          result.status === "active"
            ? "正常"
            : result.status === "suspended"
              ? "已暂停"
              : "已停用"
        }}
      </p>
      <h3>本次核验可售应用</h3>
      <ul v-if="result.applications.length">
        <li v-for="p in result.applications" :key="p.code">
          {{ p.name }} · {{ p.code }}
        </li>
      </ul>
      <p v-else>未找到可售应用，请先联系平台核实。</p>
      <small>查询时间 {{ formatTime(result.server_time) }}</small>
    </section>
  </div>
</template>
<style scoped>
.agent-verify {
  max-width: 640px;
  margin-top: 32px;
}
form {
  display: grid;
  gap: 18px;
}
form p {
  color: var(--muted);
  line-height: 1.7;
}
.agent-result {
  background: var(--paper);
  border: 1px solid var(--line);
  border-radius: 14px;
  padding: 28px;
  margin-top: 28px;
}
.tag {
  display: inline-block;
  padding: 8px 14px;
  border-radius: 20px;
  background: var(--green-soft);
  color: var(--green);
}
.denied {
  background: var(--red-soft);
  color: var(--red);
}
li {
  margin: 12px 0;
}
h2 {
  margin: 22px 0 10px;
}
small {
  color: var(--muted);
}
</style>
