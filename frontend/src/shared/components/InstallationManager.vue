<script setup lang="ts">
import { ref } from "vue";
import { formatTime } from "../format";
const props = withDefaults(
  defineProps<{
    api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
    licenseNo: string;
    productId?: string;
    prefix: "/admin" | "/developer" | "/user";
    canDisable?: boolean;
  }>(),
  { canDisable: false },
);
const opened = ref(false),
  items = ref<any[]>([]),
  error = ref(""),
  reason = ref(""),
  selected = ref(""),
  busy = ref(false);
const path = () =>
  props.prefix === "/user"
    ? `/user/licenses/${encodeURIComponent(props.licenseNo)}/installations`
    : `${props.prefix}/products/${encodeURIComponent(props.productId || "")}/licenses/${encodeURIComponent(props.licenseNo)}/installations`;
async function load() {
  busy.value = true;
  error.value = "";
  try {
    items.value = await props.api(path());
    opened.value = true;
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function disable() {
  busy.value = true;
  error.value = "";
  try {
    await props.api(`${path()}/${encodeURIComponent(selected.value)}/disable`, {
      method: "POST",
      body: JSON.stringify({ reason: reason.value }),
    });
    reason.value = "";
    selected.value = "";
    await load();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section class="installation-manager">
    <button
      class="btn secondary small"
      type="button"
      :disabled="busy"
      @click="
        opened = !opened;
        if (opened) load();
      "
    >
      {{ opened ? "收起安装实例" : "查看安装实例" }}
    </button>
    <div v-if="opened">
      <p v-if="!items.length">
        没有登记安装实例；旧接入方式可能仅使用部署绑定。
      </p>
      <article v-for="item in items" :key="item.id">
        <strong>{{ item.label || item.id }}</strong
        ><small
          >{{ item.status === "active" ? "已登记" : "已停用" }} · 最近核验
          {{ formatTime(item.last_seen_at) }}</small
        ><button
          v-if="canDisable && item.status === 'active'"
          class="btn secondary small"
          type="button"
          @click="selected = item.id"
        >
          停用实例
        </button>
      </article>
      <form v-if="selected" @submit.prevent="disable">
        <label class="field"
          >停用实例原因<input v-model="reason" required maxlength="500"
        /></label>
        <p>
          停用后此实例不能继续在线校验；客户自助操作受换绑次数和冷却限制。离线借出的席位仍保留到原截止时间。
        </p>
        <button class="btn" :disabled="busy">确认停用实例</button
        ><button class="btn secondary" type="button" @click="selected = ''">
          取消
        </button>
      </form>
    </div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
  </section>
</template>
<style scoped>
.installation-manager {
  margin: 16px 0;
  min-width: 0;
}
.installation-manager article {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
  border-bottom: 1px solid var(--line);
  padding: 14px 0;
  overflow-wrap: anywhere;
}
.installation-manager small {
  color: var(--muted);
}
.installation-manager form {
  display: grid;
  gap: 12px;
  margin-top: 16px;
}
.installation-manager form p {
  font-size: 13px;
  color: var(--muted);
}
</style>
