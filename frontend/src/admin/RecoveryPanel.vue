<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { formatTime } from "../shared/format";
const props = defineProps<{
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  agents: any[];
}>();
const kind = ref("customer"),
  agent = ref(""),
  accounts = ref<any[]>([]),
  selected = ref(""),
  error = ref(""),
  message = ref(""),
  result = ref<any>(),
  busy = ref(false);
const form = reactive({ reason: "", minutes: 30 }),
  password = ref("");
const selectedAccount = computed(() =>
  accounts.value.find((u: any) => u.id === selected.value),
);
async function load() {
  error.value = "";
  result.value = null;
  try {
    if (kind.value === "agent" && !agent.value)
      agent.value = props.agents[0]?.id || "";
    if (kind.value === "agent" && !agent.value) {
      accounts.value = [];
      selected.value = "";
      return;
    }
    accounts.value = await props.api(
      `/admin/recovery/accounts?kind=${kind.value}&agent_id=${agent.value}`,
    );
    selected.value = accounts.value[0]?.id || "";
  } catch (e: any) {
    error.value = e.message;
  }
}
async function submit(direct = false) {
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    if (direct) {
      await props.api(
        `/admin/recovery/${kind.value}/${selected.value}/password`,
        {
          method: "POST",
          body: JSON.stringify({
            password: password.value,
            reason: form.reason,
          }),
        },
      );
      password.value = "";
      result.value = null;
      message.value = "密码已重置，原登录会话已失效。";
    } else {
      result.value = await props.api(
        `/admin/recovery/${kind.value}/${selected.value}`,
        { method: "POST", body: JSON.stringify(form) },
      );
      message.value = "一次性恢复码已创建，请核实身份后交给账号持有人。";
    }
  } catch (e: any) {
    error.value = e.message;
    } finally {
      busy.value = false;
    }
  }
  async function submitByEmail() {
    busy.value = true;
    error.value = "";
    message.value = "";
    try {
      const r = await props.api(
        `/admin/recovery/${kind.value}/${selected.value}/email`,
        { method: "POST", body: JSON.stringify(form) },
      );
      result.value = null;
      message.value = `6 位验证码已发送至 ${r.sent_to}，${formatTime(r.expires_at)} 前有效；对方在密码恢复页输入登录账号和验证码即可重置。`;
    } catch (e: any) {
      error.value = e.message;
    } finally {
      busy.value = false;
    }
  }
  onMounted(load);
</script>
<template>
  <section class="panel recovery-panel">
    <h2>账号密码恢复</h2>
    <p>
      管理员核实账号持有人身份后，可创建一次性恢复码或直接重置密码。操作会记录原因，密码重置后原登录会话会失效。
    </p>
    <p v-if="error" role="alert" class="error">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>
    <form class="form-grid" @submit.prevent="submit()">
      <label class="field"
        >恢复账号类型<select v-model="kind" @change="load">
          <option value="customer">普通用户</option>
          <option value="agent">代理账号</option>
          <option value="admin">管理员与开发者</option>
        </select></label
      ><label v-if="kind === 'agent'" class="field"
        >所属代理<select v-model="agent" @change="load">
          <option v-for="a in agents" :value="a.id">{{ a.name }}</option>
        </select></label
      ><label class="field"
        >待恢复账号<select
          aria-label="待恢复账号"
          v-model="selected"
          @change="result = null"
          required
        >
          <option v-for="u in accounts" :value="u.id">
            {{ u.username }} · {{ u.display_name || u.role }}
          </option>
        </select></label
      ><label class="field"
        >恢复码有效分钟<input
          v-model.number="form.minutes"
          type="number"
          min="5"
          max="1440"
          required /></label
      ><label class="field wide"
        >身份核实与操作原因<textarea
          v-model="form.reason"
          required
          maxlength="500"
        ></textarea>
      </label>
      <div class="form-actions">
        <button class="btn" :disabled="busy || !selected">
          创建一次性恢复码
        </button>
        <button
          v-if="selectedAccount?.email"
          type="button"
          class="btn secondary"
          :disabled="busy || !selected"
          @click="submitByEmail"
        >
          创建并发送到账号邮箱
        </button>
      </div>
    </form>
    <div v-if="result" class="result">
      <p>
        有效至
        {{
          formatTime(result.expires_at)
        }}，每个恢复码只能使用一次，新建恢复码会使此前的恢复码失效。
      </p>
      <details>
        <summary>查看一次性恢复码</summary>
        <code>{{ result.code }}</code>
      </details>
      <a :href="result.recovery_url" target="_blank" rel="noopener"
        >打开密码恢复页</a
      >
    </div>
    <details class="direct">
      <summary>直接设置新密码</summary>
      <form @submit.prevent="submit(true)">
        <label class="field"
          >设置账号新密码<input
            v-model="password"
            type="password"
            required
            minlength="8"
            maxlength="256"
            autocomplete="new-password"
        /></label>
        <p>使用当前选择的账号和上方填写的核实原因。</p>
        <button
          class="btn secondary"
          :disabled="busy || !selected || !form.reason.trim()"
        >
          重置所选账号密码
        </button>
      </form>
    </details>
  </section>
</template>
<style scoped>
.recovery-panel {
  padding: 28px;
}
form {
  margin: 24px 0;
}
p {
  color: var(--muted);
}
.wide {
  grid-column: 1/-1;
}
.error {
  color: var(--red);
}
.result {
  padding: 18px;
  background: var(--green-soft);
  border-radius: 9px;
  margin: 20px 0;
}
code {
  display: block;
  overflow-wrap: anywhere;
  margin: 14px 0;
}
.result a {
  display: inline-block;
  margin-top: 15px;
}
.direct {
  border-top: 1px solid var(--line);
  padding-top: 24px;
}
.direct form {
  max-width: 580px;
  display: grid;
  gap: 16px;
}
</style>
