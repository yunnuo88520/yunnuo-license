<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import { formatTime } from "../format";
import ConfirmDialog from "./ConfirmDialog.vue";
const deleting = ref<Release>();

interface Release {
  target?: {
    os: string;
    arch: string;
    runtime: string;
    channel: string;
    kind: string;
  };
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
const targeted = ref(false),
  target = reactive({
    os: "any",
    arch: "any",
    runtime: "any",
    channel: "stable",
    kind: "archive",
    min_version: "",
    max_version: "",
    image_tag: "",
    image_digest: "",
  });
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
  const notesText = String(body.get("notes") || "");
  if (targeted.value) body.set("target", JSON.stringify(target));
  if (new Blob([notesText]).size > 16000) {
    error.value =
      "版本说明过长（含中文按字节计算不能超过 16000），请精简后重试。";
    return;
  }
  await run(async () => {
    await props.api(`${base()}/releases`, { method: "POST", body });
    version.value = "";
    notes.value = "";
    const fileInput = form.elements.namedItem(
      "package",
    ) as HTMLInputElement | null;
    if (fileInput) fileInput.value = "";
  });
  if (!error.value) message.value = "升级包已保存为草稿，发布后客户才能获取。";
}
function changeStatus(id: string, action: string) {
  void run(() =>
    props.api(`${base()}/releases/${id}/${action}`, { method: "POST" }),
  );
}
function removeRelease() {
  const item = deleting.value;
  if (!item) return;
  deleting.value = undefined;
  void run(() =>
    props.api(`${base()}/releases/${item.id}`, { method: "DELETE" }),
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
      <label class="check"
        ><input
          v-model="targeted"
          type="checkbox"
        />按平台、渠道和升级路径发布</label
      >
      <div v-if="targeted" class="target-fields">
        <label class="field"
          >操作系统<select v-model="target.os">
            <option
              v-for="v in [
                'any',
                'linux',
                'windows',
                'darwin',
                'android',
                'ios',
              ]"
              :value="v"
            >
              {{ v }}
            </option>
          </select></label
        >
        <label class="field"
          >CPU 架构<select v-model="target.arch">
            <option
              v-for="v in ['any', 'amd64', 'arm64', '386', 'arm']"
              :value="v"
            >
              {{ v }}
            </option>
          </select></label
        >
        <label class="field"
          >运行时<input
            v-model="target.runtime"
            pattern="[a-z0-9][a-z0-9._-]{0,31}"
            required
        /></label>
        <label class="field"
          >发布渠道<input
            v-model="target.channel"
            pattern="[a-z0-9][a-z0-9._-]{0,31}"
            required
        /></label>
        <label class="field"
          >更新交付类型<select v-model="target.kind">
            <option value="archive">文件升级包</option>
            <option value="installer">原生安装器归档</option>
            <option value="container_archive">Docker 镜像归档</option>
          </select></label
        >
        <label class="field"
          >最低来源版本<input
            v-model="target.min_version"
            placeholder="例如 1.0.0，可留空"
        /></label>
        <label class="field"
          >最高来源版本<input
            v-model="target.max_version"
            placeholder="例如 1.9.9，可留空"
        /></label>
        <template v-if="target.kind === 'container_archive'"
          ><label class="field"
            >归档中的镜像标签<input
              v-model="target.image_tag"
              placeholder="vendor/ebox:2.0.0"
              required /></label
          ><label class="field"
            >镜像 ID<input
              v-model="target.image_digest"
              placeholder="sha256:…"
              required
          /></label>
          <p>
            上传压缩后的 docker save
            归档。客户端验签和复核授权后导入，并核对镜像
            ID，再按产品自己的部署流程切换。
          </p></template
        >
      </div>
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
      <p v-if="release.target">
        {{ release.target.os }} / {{ release.target.arch }} ·
        {{ release.target.runtime }} · {{ release.target.channel }} ·
        {{ release.target.kind }}
      </p>
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
        <button
          v-if="release.status !== 'published'"
          class="btn danger small"
          :disabled="busy"
          @click="deleting = release"
        >
          删除升级包
        </button>
      </div>
    </article>
    <ConfirmDialog
      :open="!!deleting"
      title="删除升级包"
      :description="`删除 ${deleting?.version || ''} 的文件并释放存储额度。该版本号不能重新用于不同内容，历史授权和资金流水保留。`"
      confirm-text="确认删除升级包"
      @close="deleting = undefined"
      @confirm="removeRelease"
    />
  </section>
</template>

<style scoped>
.target-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}
.target-fields p {
  grid-column: 1/-1;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
}
.check input {
  width: auto;
}
@media (max-width: 600px) {
  .target-fields {
    grid-template-columns: 1fr;
  }
}
.release-panel {
  margin-top: 24px;
  padding: 16px 20px 20px;
  border-top: 1px solid var(--line);
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
  border-top: 1px solid var(--line);
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
