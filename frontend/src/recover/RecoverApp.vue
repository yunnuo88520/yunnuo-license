<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { request } from "../shared/api";
import { entries } from "../shared/entries";
import { branding, loadBranding } from "../shared/branding";
const form = reactive({ code: "", password: "", confirm: "" }),
  busy = ref(false),
  error = ref(""),
  done = ref(false);
onMounted(() => loadBranding("密码恢复").catch(() => {}));
async function submit() {
  error.value = "";
  if (form.password !== form.confirm) {
    error.value = "两次输入的密码不一致。";
    return;
  }
  busy.value = true;
  try {
    await request("/v1/accounts/recovery", {
      method: "POST",
      body: JSON.stringify({ code: form.code, password: form.password }),
    });
    form.code = "";
    form.password = "";
    form.confirm = "";
    done.value = true;
  } catch (e: any) {
    error.value =
      e.code === "RECOVERY_INVALID"
        ? "恢复码无效、已过期或已使用，请联系管理员重新核实。"
        : e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <main>
    <a href="/" class="brand">{{ branding.site_name }}</a>
    <section class="panel">
      <span class="eyebrow">账号恢复</span>
      <h1>{{ done ? "密码已重置" : "重新设置登录密码" }}</h1>
      <p v-if="!done">
        请先联系平台管理员核实身份并获取一次性恢复码。恢复码仅用于设置新密码。
      </p>
      <p v-else>原登录会话已失效，请使用新密码重新登录。</p>
      <p v-if="error" role="alert" class="error">{{ error }}</p>
      <form v-if="!done" @submit.prevent="submit">
        <label class="field"
          >一次性恢复码<input
            v-model="form.code"
            type="password"
            autocomplete="off"
            required
            maxlength="100" /></label
        ><label class="field"
          >新密码<input
            v-model="form.password"
            type="password"
            autocomplete="new-password"
            required
            minlength="8"
            maxlength="256" /></label
        ><label class="field"
          >再次输入新密码<input
            v-model="form.confirm"
            type="password"
            autocomplete="new-password"
            required
            minlength="8"
            maxlength="256" /></label
        ><button class="btn" :disabled="busy">
          {{ busy ? "正在重置…" : "确认重置密码" }}
        </button>
      </form>
      <nav v-if="done">
        <a :href="entries.admin">管理后台</a
        ><a :href="entries.agent">代理后台</a
        ><a :href="entries.user">用户中心</a
        ><a :href="entries.developer">开发者登录</a>
      </nav>
    </section>
  </main>
</template>
<style scoped>
main {
  max-width: 620px;
  margin: 60px auto;
  padding: 24px;
}
.brand {
  font-size: 19px;
  font-weight: 600;
}
.panel {
  padding: 32px;
  margin-top: 28px;
  border-radius: 16px;
}
h1 {
  font-size: 27px;
  margin: 12px 0;
}
p {
  color: #72816c;
  line-height: 1.8;
}
form {
  display: grid;
  gap: 20px;
  margin: 26px 0;
}
nav {
  display: flex;
  gap: 20px;
  flex-wrap: wrap;
  margin-top: 30px;
}
.error {
  color: #a5392d;
  background: #fff0eb;
  padding: 14px;
}
</style>
