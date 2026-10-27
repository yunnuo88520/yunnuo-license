<script setup lang="ts">
import { computed, watch } from "vue";
import { bindingNames, type AuthorizationTerms } from "../authorization";
const t = defineModel<AuthorizationTerms>({ required: true });
const forms: Record<string, string> = {
  web: "Web 网站",
  container: "Docker／Compose",
  cluster: "集群／Kubernetes",
  gui: "桌面 GUI",
  cli: "命令行",
  service: "后台服务",
  plugin: "插件／模块",
  mobile: "移动应用",
  iot: "IoT／工控",
  saas: "SaaS 租户",
};
const wildcard = computed(
  () =>
    t.value.bind_mode === "domain_wildcard" ||
    t.value.binding_kinds?.includes("domain_wildcard"),
);
const featureText = computed({
  get: () => (t.value.features || []).join(", "),
  set: (v: string) => {
    t.value.features = [
      ...new Set(
        v
          .split(/[,，\n]/)
          .map((x) => x.trim())
          .filter(Boolean),
      ),
    ];
  },
});
function bindingChanged() {
  const mode = t.value.bind_mode;
  t.value.binding_kinds = mode === "composite" ? ["domain", "device"] : [];
  if (mode === "instance" || mode === "cluster")
    t.value.require_instance = true;
}
watch(
  () => [t.value.max_devices, t.value.max_concurrent_seats],
  ([devices, seats]) => {
    if (devices > 0 || seats > 0) t.value.require_instance = true;
  },
);
</script>
<template>
  <div class="authorization-fields">
    <fieldset class="form-targets">
      <legend>产品与交付形态（可多选）</legend>
      <label v-for="(label, value) in forms" :key="value"
        ><input v-model="t.forms" type="checkbox" :value="value" />{{
          label
        }}</label
      >
    </fieldset>
    <div class="terms-grid">
      <label class="field"
        >卡密用途<select v-model="t.card_purpose">
          <option value="license">新授权／常规续费</option>
          <option value="renew_usage">仅续使用期限</option>
          <option value="renew_updates">仅续更新服务</option>
        </select></label
      >
      <label class="field"
        >使用授权模式<select v-model="t.license_mode">
          <option value="subscription">订阅使用</option>
          <option value="perpetual">永久使用、限期更新</option>
        </select></label
      >
      <label class="field"
        >授权绑定策略<select v-model="t.bind_mode" @change="bindingChanged">
          <option
            v-for="(label, value) in bindingNames"
            :key="value"
            :value="value"
          >
            {{ label }}
          </option>
        </select></label
      >
      <label class="field"
        >绑定规则额度<input
          v-model.number="t.max_bindings"
          type="number"
          min="1"
          max="1000"
          required
      /></label>
    </div>
    <fieldset v-if="t.bind_mode === 'composite'" class="form-targets">
      <legend>必须同时满足的绑定条件</legend>
      <label
        v-for="(label, value) in bindingNames"
        v-show="!['none', 'composite'].includes(value)"
        :key="value"
        ><input v-model="t.binding_kinds" type="checkbox" :value="value" />{{
          label
        }}</label
      >
    </fieldset>
    <div v-if="wildcard" class="terms-grid">
      <label class="field"
        >泛域名子域范围<select v-model="t.subdomain_depth">
          <option value="one">仅一级子域名</option>
          <option value="all">所有层级子域名</option>
        </select></label
      ><label class="check"
        ><input v-model="t.include_apex" type="checkbox" />同时包含根域名</label
      >
      <p>
        默认 *.example.com 只包含
        a.example.com；域名范围与安装实例额度分别计算。
      </p>
    </div>
    <label class="check"
      ><input
        v-model="t.require_instance"
        type="checkbox"
      />要求持久化安装实例凭证</label
    >
    <p>
      容器重建和合法重装应保留实例身份；不要使用临时容器 ID、Pod IP
      或镜像中的共享身份。
    </p>
    <div class="terms-grid">
      <label class="field"
        >安装实例额度<input
          v-model.number="t.max_deployments"
          type="number"
          min="1"
          max="10000"
          :disabled="!t.require_instance"
          required
      /></label>
      <label class="field"
        >设备额度（0 不额外限制）<input
          v-model.number="t.max_devices"
          type="number"
          min="0"
          max="10000"
          required
      /></label>
      <label class="field"
        >并发席位（0 不启用）<input
          v-model.number="t.max_concurrent_seats"
          type="number"
          min="0"
          max="10000"
          required
      /></label>
      <label v-if="t.max_concurrent_seats > 0" class="field"
        >席位租约秒数<input
          v-model.number="t.seat_lease_seconds"
          type="number"
          min="30"
          max="3600"
          required
      /></label>
      <label class="field"
        >更新服务天数（0 跟随卡密）<input
          v-model.number="t.update_duration_days"
          type="number"
          min="0"
          max="36500"
          required
      /></label>
      <label class="field"
        >使用到期宽限天数<input
          v-model.number="t.expire_grace_days"
          type="number"
          min="0"
          max="365"
          required
      /></label>
      <label class="field"
        >离线缓存／席位借用上限天数<input
          v-model.number="t.offline_grace_days"
          type="number"
          min="0"
          max="365"
          required
      /></label>
      <label class="field"
        >换绑次数（0 禁止，-1 不限）<input
          v-model.number="t.unbind_limit"
          type="number"
          min="-1"
          max="10000"
          required
      /></label>
      <label class="field"
        >换绑冷却小时<input
          v-model.number="t.unbind_cooldown_hours"
          type="number"
          min="0"
          max="87600"
          required
      /></label>
      <label class="field"
        >功能标识（逗号分隔）<input
          v-model="featureText"
          maxlength="6400"
          placeholder="articles, export, api"
      /></label>
    </div>
    <p>
      使用宽限不延长更新资格。离线借用仍占用席位，到期才自动回收；离线期间无法即时获知吊销。
    </p>
  </div>
</template>
<style scoped>
.authorization-fields {
  display: grid;
  gap: 16px;
  grid-column: 1/-1;
  min-width: 0;
}
.terms-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}
.form-targets {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 20px;
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 16px;
  min-width: 0;
}
.form-targets label,
.check {
  display: flex;
  gap: 8px;
  align-items: center;
}
.form-targets input,
.check input {
  width: auto;
}
.authorization-fields p {
  color: var(--muted);
  font-size: 13px;
  margin: 0;
  line-height: 1.7;
}
.terms-grid p {
  grid-column: 1/-1;
}
@media (max-width: 600px) {
  .terms-grid {
    grid-template-columns: 1fr;
  }
  .form-targets {
    gap: 12px;
  }
}
</style>
