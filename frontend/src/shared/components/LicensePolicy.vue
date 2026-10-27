<script setup lang="ts">
import { ref, watch } from "vue";
import AuthorizationTermsFields from "./AuthorizationTermsFields.vue";
import { authorizationDefaults } from "../authorization";
const props = defineProps<{
  productId: string;
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  prefix: "/admin" | "/developer";
  canWrite: boolean;
}>();
const emit = defineEmits<{ saved: [] }>();
const terms = ref(authorizationDefaults()),
  busy = ref(false),
  error = ref(""),
  message = ref("");
let generation = 0;
const path = () =>
  `${props.prefix}/products/${encodeURIComponent(props.productId)}/authorization-terms`;
watch(
  () => props.productId,
  async () => {
    const current = ++generation;
    error.value = "";
    message.value = "";
    try {
      const result = await props.api(path());
      if (current === generation)
        terms.value = { ...authorizationDefaults(), ...result };
    } catch (e: any) {
      if (current === generation) error.value = e.message;
    }
  },
  { immediate: true },
);
async function save() {
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    terms.value = await props.api(path(), {
      method: "POST",
      body: JSON.stringify(terms.value),
    });
    message.value = "默认条款已保存。库存卡和已有客户授权保持原条款。";
    emit("saved");
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section v-if="productId" class="policy-panel" aria-label="默认授权条款">
    <h3>默认授权条款</h3>
    <p>
      用于之后按应用默认规则生成的卡密。不同销售规格请在“套餐与结算”建立独立套餐，所有套餐共用此应用的
      AppID 和版本库。
    </p>
    <form @submit.prevent="save">
      <fieldset :disabled="busy || !canWrite" class="terms-fieldset">
        <AuthorizationTermsFields v-model="terms" /><button
          v-if="canWrite"
          class="btn secondary"
        >
          保存默认授权条款
        </button>
      </fieldset>
    </form>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>
  </section>
</template>
<style scoped>
.policy-panel {
  border-top: 1px solid var(--line);
  padding: 20px 0;
  margin-top: 24px;
}
.terms-fieldset {
  border: 0;
  padding: 0;
  display: grid;
  gap: 20px;
  min-width: 0;
}
.policy-panel > p {
  color: var(--muted);
  line-height: 1.7;
}
.terms-fieldset > .btn {
  justify-self: start;
}
</style>
