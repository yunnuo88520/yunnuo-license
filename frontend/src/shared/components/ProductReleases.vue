<script setup lang="ts">
import { ref, watch } from "vue";
import { formatTime } from "../format";

interface Release {
  id: string;
  version: string;
  notes: string;
  status: string;
  filename: string;
  size: number;
  sha256: string;
  created_at: string;
}
const props = defineProps<{
  productId: string;
  api: <T = any>(path: string, options?: RequestInit) => Promise<T>;
  prefix: "/admin" | "/developer";
  canWrite: boolean;
}>();
const enabled = ref(false),
  releases = ref<Release[]>([]),
  busy = ref(false),
  error = ref("");
const version = ref(""),
  notes = ref(""),
  message = ref("");
const base = () =>
  `${props.prefix}/products/${encodeURIComponent(props.productId)}`;
let generation = 0;
async function load() {
  const current = ++generation;
  enabled.value = false;
  releases.value = [];
  error.value = "";
  message.value = "";
  if (!props.productId) return;
  const path = base();
  try {
    const [settings, items] = await Promise.all([
      props.api(`${path}/updates`),
      props.api<Release[]>(`${path}/releases`),
    ]);
    if (current !== generation) return;
    enabled.value = settings.enabled;
    releases.value = items;
  } catch (e: any) {
    if (current === generation) error.value = e.message;
  }
}
watch(
  () => props.productId,
  () => {
    version.value = "";
    notes.value = "";
    void load();
  },
  { immediate: true },
);
async function run(action: () => Promise<unknown>) {
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await action();
    await load();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
function toggle() {
  void run(() =>
    props.api(`${base()}/updates`, {
      method: "POST",
      body: JSON.stringify({ enabled: !enabled.value }),
    }),
  );
}
async function upload(event: Event) {
  const form = event.currentTarget as HTMLFormElement;
  const body = new FormData(form);
  await run(async () => {
    await props.api(`${base()}/releases`, { method: "POST", body });
    version.value = "";
    notes.value = "";
    form.reset();
  });
  if (!error.value) message.value = "升级包已保存为草稿，发布后客户才能获取。";
}
function changeStatus(id: string, action: string) {
  void run(() =>
    props.api(`${base()}/releases/${id}/${action}`, { method: "POST" }),
  );
}
</script>

<template>
  <section v-if="productId" class="release-panel">
    <header class="release-heading">
      <div>
        <h2>产品升级服务</h2>
        <p>按产品选择是否提供升级包。关闭后仍可正常使用授权服务。</p>
      </div>
      <button
        v-if="canWrite"
        class="btn secondary"
        :disabled="busy"
        @click="toggle"
      >
        {{ enabled ? "关闭升级服务" : "启用升级服务" }}
      </button>
    </header>
    <p>
      当前状态：<strong>{{ enabled ? "已启用" : "未启用" }}</strong> ·
      仅更新资格有效的授权可检查和下载更新
    </p>
    <form v-if="canWrite" class="release-upload" @submit.prevent="upload">
      <label class="field"
        >版本号<input
          v-model="version"
          name="version"
          placeholder="例如 1.2.0"
          pattern="(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})"
          required
          :disabled="busy"
      /></label>
      <label class="field"
        >更新说明<textarea
          v-model="notes"
          name="notes"
          maxlength="16000"
          rows="3"
          :disabled="busy"
        ></textarea>
      </label>
      <label class="field"
        >升级包（ZIP / TAR.GZ / TGZ，最大 256 MiB）<input
          name="package"
          type="file"
          accept=".zip,.tar.gz,.tgz"
          required
          :disabled="busy"
      /></label>
      <button class="btn" :disabled="busy">
        {{ busy ? "处理中…" : "上传为草稿" }}
      </button>
    </form>
    <p v-if="error" role="alert" class="error">{{ error }}</p>
    <p v-if="message" role="status">{{ message }}</p>
    <p v-if="!releases.length">暂无版本，可继续仅使用授权服务。</p>
    <article v-for="release in releases" :key="release.id" class="release-item">
      <div class="release-heading">
        <strong
          >v{{ release.version }} ·
          {{
            { draft: "草稿", published: "已发布", withdrawn: "已下架" }[
              release.status
            ] || release.status
          }}</strong
        ><span>{{ formatTime(release.created_at) }}</span>
      </div>
      <p class="release-notes">{{ release.notes || "暂无更新说明" }}</p>
      <p>
        {{ release.filename }} ·
        {{ (release.size / 1024 / 1024).toFixed(2) }} MiB
      </p>
      <small class="release-hash">SHA-256：{{ release.sha256 }}</small>
      <div v-if="canWrite" class="release-actions">
        <button
          v-if="release.status !== 'published'"
          class="btn small"
          :disabled="busy"
          @click="changeStatus(release.id, 'publish')"
        >
          发布版本
        </button>
        <button
          v-else
          class="btn secondary small"
          :disabled="busy"
          @click="changeStatus(release.id, 'withdraw')"
        >
          下架版本
        </button>
      </div>
    </article>
  </section>
</template>

<style scoped>
.release-panel {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid #dfe5e0;
}
.release-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}
.release-upload {
  display: grid;
  gap: 14px;
  margin: 20px 0;
  max-width: 700px;
}
.release-item {
  padding: 18px 0;
  border-top: 1px solid #e4e8e5;
}
.release-notes {
  white-space: pre-wrap;
}
.release-hash {
  overflow-wrap: anywhere;
}
.release-actions {
  margin-top: 12px;
}
</style>
