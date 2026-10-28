<script setup lang="ts">
import { onMounted, reactive, ref, watch } from "vue";
import { formatTime } from "../format";
const props = defineProps<{
  application: any;
  api: (path: string, options?: RequestInit) => Promise<any>;
  canWrite: boolean;
}>();
const releases = ref<any[]>([]),
  error = ref(""),
  notice = ref(""),
  busy = ref(false),
  file = ref<File | null>(null);
const upload = reactive({
  version: "",
  notes: "",
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
const reason = ref(""),
  action = ref(""),
  item = ref<any>(null);
const base = () =>
  `/v2/console/applications/${props.application.application_id}/releases`;
async function run(fn: () => Promise<void>) {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await fn();
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function refresh() {
  releases.value = await props.api(base());
}
async function save() {
  if (!file.value) throw new Error("请选择升级包");
  const body = new FormData();
  body.set("file", file.value);
  body.set("version", upload.version);
  body.set("notes", upload.notes);
  body.set(
    "target",
    JSON.stringify({
      os: upload.os,
      arch: upload.arch,
      runtime: upload.runtime,
      channel: upload.channel,
      kind: upload.kind,
      min_version: upload.min_version,
      max_version: upload.max_version,
      image_tag: upload.image_tag,
      image_digest: upload.image_digest,
    }),
  );
  await props.api(`${base()}/upload`, { method: "POST", body });
  await refresh();
  notice.value = "升级包已上传为草稿，发布后客户端才可获得。";
  upload.version = "";
}
async function status() {
  await props.api(`${base()}/${item.value.id}/status`, {
    method: "POST",
    body: JSON.stringify({ status: action.value, reason: reason.value }),
  });
  action.value = "";
  reason.value = "";
  await refresh();
  notice.value = "版本状态已更新并写入应用审计。";
}
watch(
  () => upload.kind,
  (value) => {
    if (value !== "container_archive") {
      upload.image_tag = "";
      upload.image_digest = "";
    }
  },
);
watch(
  () => props.application.application_id,
  () => run(refresh),
);
onMounted(() => run(refresh));
</script>
<template>
  <section class="panel app-releases">
    <h3>版本与在线升级</h3>
    <p>
      同一版本可分别上传
      Web、桌面系统与容器包。平台、架构、运行时、渠道与升级路径由服务端匹配，同一目标版本不能覆盖。
    </p>
    <p v-if="!application.updates_enabled" class="notice">
      此应用的升级服务尚未启用，请在「应用与策略」中开启。可以先上传草稿准备版本。
    </p>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <form v-if="canWrite" @submit.prevent="run(save)">
      <div class="release-grid">
        <label class="field"
          >版本号<input
            v-model="upload.version"
            required
            pattern="[0-9]+\.[0-9]+\.[0-9]+"
            placeholder="1.2.0" /></label
        ><label class="field"
          >操作系统<select v-model="upload.os" aria-label="操作系统">
            <option value="any">不限</option>
            <option value="linux">Linux</option>
            <option value="windows">Windows</option>
            <option value="darwin">macOS</option>
            <option value="android">Android</option>
            <option value="ios">iOS</option>
          </select></label
        ><label class="field"
          >架构<select v-model="upload.arch" aria-label="架构">
            <option value="any">不限</option>
            <option value="amd64">x86_64 / amd64</option>
            <option value="arm64">ARM64</option>
            <option value="arm">ARM</option>
          </select></label
        ><label class="field"
          >运行时<input
            v-model="upload.runtime"
            required
            placeholder="any / node / php / java / docker" /></label
        ><label class="field"
          >发布渠道<input
            v-model="upload.channel"
            required
            placeholder="stable / beta" /></label
        ><label class="field"
          >升级包类型<select v-model="upload.kind" aria-label="升级包类型">
            <option value="archive">程序文件归档</option>
            <option value="installer">桌面安装包归档</option>
            <option value="container_archive">Docker 镜像离线归档</option>
          </select></label
        ><label class="field"
          >允许的最低旧版本（可选）<input
            v-model="upload.min_version"
            placeholder="1.0.0" /></label
        ><label class="field"
          >允许的最高旧版本（可选）<input
            v-model="upload.max_version"
            placeholder="1.9.9" /></label
        ><template v-if="upload.kind === 'container_archive'"
          ><label class="field"
            >镜像标签<input
              v-model="upload.image_tag"
              required
              placeholder="registry.example.com/product:1.2.0" /></label
          ><label class="field"
            >不可变镜像 ID<input
              v-model="upload.image_digest"
              required
              placeholder="sha256:..." /></label></template
        ><label class="field"
          >升级包（最多 256 MiB）<input
            type="file"
            required
            accept=".zip,.tar.gz,.tgz"
            @change="
              file = ($event.target as HTMLInputElement).files?.[0] || null
            "
        /></label>
      </div>
      <label class="field"
        >更新说明<textarea v-model="upload.notes" maxlength="16000"></textarea>
      </label>
      <p class="muted">
        归档应只包含可替换的程序文件；客户配置、数据库、上传数据与授权私钥使用独立持久目录，升级前备份并准备回滚。容器镜像由客户端加载，授权站不会执行上传内容。
      </p>
      <button class="btn" :disabled="busy">上传为草稿</button>
    </form>
    <div class="release-table">
      <table>
        <thead>
          <tr>
            <th>版本 / 时间</th>
            <th>目标与渠道</th>
            <th>状态</th>
            <th>文件与完整性</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in releases" :key="r.id">
            <td>
              {{ r.version }}<small>{{ formatTime(r.created_at) }}</small>
            </td>
            <td>
              {{ r.target.os }} / {{ r.target.arch }} / {{ r.target.runtime
              }}<small>{{ r.target.channel }} · {{ r.target.kind }}</small>
            </td>
            <td>
              {{
                (
                  {
                    draft: "草稿",
                    published: "已发布",
                    withdrawn: "已撤回",
                    deleted: "已删除",
                  } as any
                )[r.status]
              }}
            </td>
            <td>
              {{ r.filename }} ·
              {{ (r.size / 1024 / 1024).toFixed(2) }} MiB<small class="hash"
                >SHA-256 {{ r.sha256 }}</small
              >
            </td>
            <td>
              <button
                v-if="canWrite && r.status !== 'published'"
                class="btn secondary"
                @click="
                  item = r;
                  action = 'published';
                "
              >
                发布</button
              ><button
                v-if="canWrite && r.status === 'published'"
                class="btn secondary"
                @click="
                  item = r;
                  action = 'withdrawn';
                "
              >
                撤回
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <form v-if="action" class="release-action" @submit.prevent="run(status)">
      <p>
        确认{{ action === "published" ? "发布" : "撤回" }} {{ item.version }}？
      </p>
      <label class="field"
        >操作原因<input v-model="reason" required maxlength="500" /></label
      ><button class="btn" :disabled="busy">确认</button
      ><button class="btn secondary" type="button" @click="action = ''">
        取消
      </button>
    </form>
    <p class="muted">
      检查版本、下载与安装前确认均需要安装签名与实时有效的更新资格。离线使用凭证不能绕过更新到期；使用宽限不会延长更新时间。
    </p>
  </section>
</template>
<style scoped>
.app-releases {
  padding: 24px;
  min-width: 0;
}
.release-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}
form {
  display: grid;
  gap: 16px;
  margin: 22px 0;
}
.release-table {
  overflow: auto;
  margin: 24px 0;
}
table {
  width: 100%;
  min-width: 680px;
  border-collapse: collapse;
}
td,
th {
  padding: 14px 10px;
  text-align: left;
  border-bottom: 1px solid var(--line);
  vertical-align: top;
}
small {
  display: block;
  margin-top: 5px;
  color: var(--muted);
}
.hash {
  max-width: 300px;
  overflow-wrap: anywhere;
}
.release-action {
  border: 1px solid var(--line);
  padding: 18px;
  border-radius: 10px;
}
@media (max-width: 600px) {
  .release-grid {
    grid-template-columns: 1fr;
  }
  .app-releases {
    padding: 16px;
  }
}
</style>
