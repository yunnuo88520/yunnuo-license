<script setup lang="ts">
import { onMounted, ref } from "vue";
import { request } from "../api";
import { friendlyError } from "../errors";

const props = defineProps<{
  productId: string;
  prefix: string;
  token: string;
  initialSecret?: string;
}>();
const emit = defineEmits<{ consumed: [] }>();
const secret = ref(props.initialSecret || ""),
  visible = ref(false),
  status = ref<any>(),
  error = ref(""),
  message = ref(""),
  busy = ref(false),
  editing = ref(false),
  reason = ref(""),
  confirmed = ref(false);
const path = `${props.prefix}/products/${encodeURIComponent(props.productId)}/credentials`;
async function load() {
  try {
    status.value = await request(path, {}, props.token);
  } catch (e) {
    error.value = friendlyError(e);
  }
}
async function copySecret() {
  try {
    await navigator.clipboard.writeText(secret.value);
    message.value = "AppSecret 已复制，请保存到开发者服务端的安全配置中。";
  } catch {
    error.value = "复制失败，请显示后手动复制。";
  }
}
async function reset() {
  if (!confirmed.value || !status.value) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    const result = await request<any>(
      path + "/reset",
      {
        method: "POST",
        body: JSON.stringify({
          expected_version: status.value.version,
          reason: reason.value,
        }),
      },
      props.token,
    );
    secret.value = result.app_secret;
    visible.value = false;
    editing.value = false;
    confirmed.value = false;
    reason.value = "";
    await load();
    message.value =
      "AppSecret 已重置，旧密钥及旧访问令牌已失效。请保存本次生成的密钥。";
  } catch (e) {
    error.value = friendlyError(e);
    await load();
  } finally {
    busy.value = false;
  }
}
onMounted(() => {
  emit("consumed");
  void load();
});
</script>
<template>
  <section class="app-credentials" aria-label="应用服务端凭证">
    <h3>AppSecret · 开发者服务端密钥</h3>
    <p>
      与微信开发者凭证采用相同的使用方式：AppID 公开，AppSecret 保密。AppSecret
      为 32
      位随机十六进制字符，仅在创建或重置后当前页面展示，刷新后不可再次查看。
    </p>
    <p v-if="error" class="credential-error" role="alert">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>
    <template v-if="secret">
      <label class="field"
        >AppSecret<input
          :value="secret"
          :type="visible ? 'text' : 'password'"
          readonly
          autocomplete="off"
          spellcheck="false"
      /></label>
      <div class="credential-actions">
        <button
          class="btn secondary small"
          type="button"
          @click="visible = !visible"
        >
          {{ visible ? "隐藏 AppSecret" : "显示 AppSecret" }}
        </button>
        <button class="btn secondary small" type="button" @click="copySecret">
          复制 AppSecret
        </button>
      </div>
    </template>
    <p v-else-if="status">
      {{
        status.configured
          ? `已配置 · 第 ${status.version} 版；遗失后请重置。`
          : "尚未配置，请生成 AppSecret。"
      }}
    </p>
    <p>
      此密钥只用于开发者自有的可信服务端，换取 7200 秒有效的
      access_token。不放入网页、小程序前端或发给客户安装的产品，也不包含在下方客户端接入包中。
    </p>
    <button
      v-if="status && !editing"
      class="btn secondary small"
      type="button"
      @click="editing = true"
    >
      {{ status.configured ? "重置 AppSecret" : "生成 AppSecret" }}
    </button>
    <form v-if="editing" class="credential-reset" @submit.prevent="reset">
      <label class="field"
        >重置原因<textarea v-model="reason" required maxlength="500" rows="2" />
      </label>
      <label class="credential-confirm"
        ><input v-model="confirmed" type="checkbox" required />我确认：旧
        AppSecret
        和已签发的访问令牌会立即失效，需要更新开发者服务端配置。</label
      >
      <div class="credential-actions">
        <button class="btn" :disabled="busy || !confirmed">
          {{ busy ? "正在生成…" : "确认生成新密钥" }}</button
        ><button type="button" class="btn secondary" @click="editing = false">
          取消
        </button>
      </div>
    </form>
  </section>
</template>
<style scoped>
.app-credentials {
  padding: 18px;
  margin: 18px 0;
  border: 1px solid var(--line);
  border-radius: 9px;
  background: var(--surface);
  min-width: 0;
}
h3 {
  margin: 0 0 10px;
  font-size: 16px;
}
p {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.85;
  overflow-wrap: anywhere;
}
.credential-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 10px 0;
}
.credential-reset {
  margin-top: 16px;
}
.credential-confirm {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  font-size: 13px;
  line-height: 1.8;
  margin: 14px 0;
}
.credential-confirm input {
  width: auto;
  flex: none;
  margin-top: 5px;
}
.credential-error {
  color: var(--red);
}
input[readonly] {
  font-family: monospace;
}
</style>
