<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import {
  CheckCircle2,
  Database,
  ShieldCheck,
  ArrowRight,
  LoaderCircle,
} from "lucide-vue-next";
import { request } from "../shared/api";
const ready = ref(false),
  done = ref(false),
  busy = ref(false),
  error = ref("");
const links = ref<Record<string, string>>({
  admin: "/admin",
  agent: "/agent-console/",
  public: "/",
  user: "/user-console/",
  developer: "/developer-console/",
});
const form = reactive({
  database_driver: "sqlite",
  database_path: "yn-license.db",
  database_host: "127.0.0.1",
  database_port: 3306,
  database_name: "yunnuo_license",
  database_user: "yunnuo",
  database_password: "",
  admin_username: "",
  admin_password: "",
  admin_name: "系统管理员",
});
onMounted(async () => {
  try {
    const status = await request<any>("/v1/setup/status");
    done.value = status.initialized;
    links.value = status.entries || links.value;
    form.database_driver = status.database_driver || "sqlite";
    form.database_path = (status.suggested_sqlite_dsn || "file:yn-license.db")
      .replace(/^file:/, "")
      .split("?")[0];
    ready.value = true;
  } catch (e: any) {
    error.value = e.message;
  }
});
async function install() {
  busy.value = true;
  error.value = "";
  try {
    const result = await request<any>("/v1/setup/initialize", {
      method: "POST",
      body: JSON.stringify(form),
    });
    links.value = result.entries;
    done.value = true;
    form.admin_password = "";
    form.database_password = "";
  } catch (e: any) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <main class="install-page">
    <header>
      <img src="/assets/yunnuo-mark.svg" alt="" />
      <div><strong>允诺云授权</strong><small>应用授权服务平台</small></div>
      <span>安装向导</span>
    </header>
    <section v-if="!done" class="intro">
      <span class="eyebrow">首次运行</span>
      <h1>开始搭建您的授权平台</h1>
      <p>
        选择数据库，创建平台管理员。完成安装后即可管理应用、代理和授权用户。
      </p>
    </section>
    <p v-if="error" role="alert" class="install-error">{{ error }}</p>
    <p v-if="!ready && !error">正在检查安装状态…</p>
    <form v-if="ready && !done" @submit.prevent="install">
      <fieldset :disabled="busy">
        <section class="setup-card">
          <div class="card-title">
            <Database :size="23" />
            <div>
              <h2>01 · 配置数据库</h2>
              <p>SQLite 适合单机部署，MySQL 适合持续运行的业务平台。</p>
            </div>
          </div>
          <div class="database-options">
            <button
              type="button"
              :class="{ chosen: form.database_driver === 'sqlite' }"
              @click="form.database_driver = 'sqlite'"
            >
              <strong>SQLite</strong><span>本机文件数据库</span></button
            ><button
              type="button"
              :class="{ chosen: form.database_driver === 'mysql' }"
              @click="form.database_driver = 'mysql'"
            >
              <strong>MySQL</strong><span>连接现有数据库</span>
            </button>
          </div>
          <label v-if="form.database_driver === 'sqlite'" class="field"
            >数据库文件路径<input
              v-model="form.database_path"
              aria-label="数据库文件路径"
              required
            /><small
              >路径相对于服务运行目录，也可填写服务器上的绝对路径。</small
            ></label
          >
          <div v-else class="install-grid">
            <label class="field"
              >数据库地址<input v-model="form.database_host" required /></label
            ><label class="field"
              >数据库端口<input
                v-model.number="form.database_port"
                type="number"
                min="1"
                max="65535"
                required /></label
            ><label class="field"
              >数据库名称<input v-model="form.database_name" required /></label
            ><label class="field"
              >数据库用户名<input
                v-model="form.database_user"
                required /></label
            ><label class="field wide"
              >数据库密码<input
                v-model="form.database_password"
                type="password"
                autocomplete="off"
            /></label>
          </div>
        </section>
        <section class="setup-card">
          <div class="card-title">
            <ShieldCheck :size="23" />
            <div>
              <h2>02 · 创建管理员</h2>
              <p>此账号拥有平台管理权限，请妥善保存登录信息。</p>
            </div>
          </div>
          <div class="install-grid">
            <label class="field"
              >管理员用户名<input
                v-model="form.admin_username"
                required
                maxlength="80"
                autocomplete="username" /></label
            ><label class="field"
              >管理员显示名称<input
                v-model="form.admin_name"
                required
                maxlength="160" /></label
            ><label class="field wide"
              >管理员密码<input
                v-model="form.admin_password"
                aria-label="管理员密码"
                type="password"
                required
                minlength="8"
                maxlength="256"
                autocomplete="new-password"
              /><small>至少 8 位，建议使用字母、数字和符号组合。</small></label
            >
          </div>
        </section>
        <button class="btn install-submit" :disabled="busy">
          <LoaderCircle v-if="busy" class="spin" :size="18" /><ArrowRight
            v-else
            :size="18"
          />{{ busy ? "正在验证并安装…" : "完成安装" }}
        </button>
      </fieldset>
    </form>
    <section v-if="ready && done" class="success">
      <CheckCircle2 :size="56" /><span class="eyebrow">安装已完成</span>
      <h1>您的授权平台已就绪</h1>
      <p>
        数据库及管理员账号已配置。请保存以下入口，使用刚设置的管理员账号登录后台。
      </p>
      <div class="entry-grid">
        <a :href="links.admin"
          ><ShieldCheck :size="24" /><strong>管理后台</strong
          ><code>{{ links.admin }}</code
          ><span>管理应用、账号与平台配置 <ArrowRight :size="15" /></span></a
        ><a :href="links.agent"
          ><strong>代理后台</strong><code>{{ links.agent }}</code
          ><span>代理登录与套餐发卡 <ArrowRight :size="15" /></span></a
        ><a :href="links.public"
          ><strong>前台首页</strong><code>{{ links.public }}</code
          ><span>公开授权查询与代理验真 <ArrowRight :size="15" /></span
        ></a>
      </div>
      <div class="extra-entries">
        <a :href="links.user">用户中心</a
        ><a :href="links.developer">开发者控制台</a>
      </div>
      <p class="locked">安装已锁定，重新访问此页会显示入口信息。</p>
    </section>
    <footer>允诺云授权 · 连接开发者、代理与正版用户</footer>
  </main>
</template>
<style scoped>
.install-page {
  max-width: 960px;
  margin: auto;
  padding: 34px 24px 24px;
}
header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 48px;
}
header img {
  width: 42px;
  height: 42px;
}
header strong {
  font-size: 18px;
}
header small {
  display: block;
  color: #72816c;
  font-size: 11px;
}
header > span {
  margin-left: auto;
  color: #72816c;
  font-size: 12px;
}
.intro {
  margin-bottom: 32px;
}
.eyebrow {
  color: #688c47;
  font-size: 12px;
  letter-spacing: 0.1em;
}
h1 {
  font-size: 34px;
  line-height: 1.4;
  margin: 10px 0 14px;
}
.intro p,
.success > p {
  color: #72816c;
  line-height: 1.9;
}
fieldset {
  border: 0;
  padding: 0;
  margin: 0;
  min-width: 0;
}
.setup-card {
  border: 1px solid #dce3d6;
  background: white;
  border-radius: 16px;
  padding: 30px;
  margin: 22px 0;
}
.card-title {
  display: flex;
  align-items: flex-start;
  gap: 15px;
  margin-bottom: 25px;
  color: #41613c;
}
.card-title h2 {
  margin: 0;
  font-size: 19px;
  color: #233622;
}
.card-title p {
  margin: 8px 0 0;
  color: #7b8675;
  font-size: 13px;
}
.database-options {
  display: flex;
  gap: 16px;
  margin-bottom: 22px;
}
.database-options button {
  flex: 1;
  text-align: left;
  background: #f8faf6;
  border: 1px solid #dfe5db;
  border-radius: 9px;
  padding: 18px 22px;
  cursor: pointer;
  color: #34462f;
}
.database-options strong,
.database-options span {
  display: block;
}
.database-options span {
  font-size: 12px;
  color: #819078;
  margin-top: 5px;
}
.database-options .chosen {
  border-color: #618344;
  background: #edf5e7;
  box-shadow: 0 0 0 1px #618344;
}
.install-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}
.wide {
  grid-column: 1/-1;
}
.field small {
  font-size: 11px;
  color: #819078;
  font-weight: 400;
}
.install-submit {
  width: 100%;
  padding: 15px;
  justify-content: center;
}
.install-error {
  border: 1px solid #eacbc5;
  background: #fff1ed;
  color: #8d342b;
  padding: 16px;
  border-radius: 9px;
}
.success {
  text-align: center;
  margin: 65px 0 45px;
}
.success > svg {
  color: #5a8148;
  display: block;
  margin: 0 auto 18px;
}
.entry-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 18px;
  margin: 36px 0;
}
.entry-grid > a {
  display: grid;
  align-content: start;
  gap: 13px;
  text-align: left;
  background: #fff;
  border: 1px solid #dce3d6;
  border-radius: 14px;
  padding: 26px;
  min-height: 190px;
}
.entry-grid > a:first-child {
  background: #263e2b;
  color: #fff;
}
.entry-grid > a > strong {
  font-size: 21px;
}
.entry-grid code {
  color: #7b9566;
  overflow-wrap: anywhere;
}
.entry-grid > a > span {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: #819078;
}
.extra-entries {
  display: flex;
  justify-content: center;
  gap: 24px;
  font-size: 13px;
}
.success .locked,
footer {
  font-size: 11px;
  color: #88917f;
}
footer {
  text-align: center;
  margin-top: 40px;
}
@media (max-width: 600px) {
  .install-page {
    padding: 24px 16px;
  }
  header {
    margin-bottom: 30px;
  }
  h1 {
    font-size: 27px;
  }
  .setup-card {
    padding: 22px;
  }
  .install-grid,
  .entry-grid {
    grid-template-columns: 1fr;
  }
  .entry-grid > a {
    min-height: 140px;
  }
  .database-options button {
    padding: 14px;
  }
  .success {
    margin-top: 30px;
  }
}
</style>
