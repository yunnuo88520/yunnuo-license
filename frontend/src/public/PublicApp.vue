<script setup lang="ts">
import AppearanceMenu from "../shared/components/AppearanceMenu.vue";
import { computed, onMounted, ref } from "vue";
import {
  ArrowRight,
  CheckCircle2,
  CircleHelp,
  CircleUserRound,
  Crosshair,
  Globe2,
  KeyRound,
  Network,
  RotateCcw,
  Search,
  ShieldCheck,
  TicketCheck,
} from "lucide-vue-next";
import { entries } from "../shared/entries";
import AgentVerify from "./AgentVerify.vue";
import CaptchaField from "../shared/components/CaptchaField.vue";
import HelpPanel from "../shared/components/HelpPanel.vue";
import { request, ApiError } from "../shared/api";
import { formatTime, statusLabel } from "../shared/format";
import { branding, loadBranding, logoSource } from "../shared/branding";

const mode = ref("license");
const query = ref("");
const applications=ref<any[]>([]),applicationID=ref("");
const queryType = ref("auto");
const loading = ref(false);
const error = ref("");
const result = ref<any>(null);
const needCaptcha = ref(false);
const captchaCode = ref("");
const helpOpen = ref(false);
const captchaField = ref<any>(null);
const queryTypes = [{value:"auto",label:"智能识别",icon:Crosshair},{value:"domain",label:"域名",icon:Globe2},{value:"ip",label:"IP",icon:Network},{value:"card",label:"卡密",icon:TicketCheck}];
const placeholders:Record<string,string>={auto:"输入域名、IP 或卡密",domain:"例如 example.com",ip:"例如 203.0.113.42",card:"输入完整激活卡密"};
const publicStatus=(r:any)=>r.authorized?"已授权":({suspended:"授权已暂停",revoked:"授权已吊销",expired:"授权已到期",policy_denied:"当前来源未获准",not_found:"未查询到授权"} as any)[r.status]||"未获有效授权";
const displayResults = computed(() =>
  result.value?.results?.length
    ? result.value.results
    : result.value
      ? [result.value]
      : [],
);
onMounted(async () => {
  await loadBranding("授权查询").catch(() => undefined);
 applications.value=await request<any[]>("/v2/public/applications").catch(()=>[]);applicationID.value=applications.value[0]?.app_id||"";
});
async function submit() {
  const value = query.value.trim();
  if (!value) return;
  loading.value = true;
  error.value = "";
  result.value = null;
  try {
    let kind=queryType.value;if(kind==="auto")kind=value.toUpperCase().startsWith("YN2-")?"card":/^[0-9.]+$/.test(value)||value.includes(":")&&!value.includes("://")?"ip":"domain";
    const payload:any={app_id:applicationID.value,binding:{type:kind==="card"?"domain":kind,value:kind==="card"?"":value}};
    if(kind==="card")payload.card_code=value;
    if(needCaptcha.value){payload.captcha_id=captchaField.value?.captchaId||"";payload.captcha_code=captchaCode.value;}
    result.value=await request("/v2/public/query",{method:"POST",body:JSON.stringify(payload)});
  } catch (e: any) {
    if (
      e instanceof ApiError &&
      (e.code === "CAPTCHA_REQUIRED" || e.code === "CAPTCHA_INVALID")
    ) {
      needCaptcha.value = true;
      error.value =
        e.code === "CAPTCHA_REQUIRED"
          ? "当前查询较为频繁，请输入图形验证码后重试"
          : "图形验证码不正确，请重新输入";
      captchaField.value?.refresh();
    } else if (
      e instanceof ApiError &&
      e.code === "RATE_LIMITED"
    ) {
      error.value = "查询过于频繁，请稍后再试";
    } else {
      error.value =
        e.code === "AUTHORIZATION_NOT_FOUND"
          ? "未查询到该授权"
          : "查询失败，请稍后重试";
    }
  } finally {
    loading.value = false;
  }
}
function clear() {
  query.value = "";
  result.value = null;
  error.value = "";
  needCaptcha.value = false;
  captchaCode.value = "";
}
function selectQueryType(value: string) {
  queryType.value = value;
  result.value = null;
  error.value = "";
}
</script>
<template>
  <main class="query-canvas">
    <header class="query-nav">
      <a class="query-brand" href="/"
        ><img :src="logoSource" alt="" /><span>{{
          branding.site_name
        }}</span></a
      >
      <div class="query-links">
        <a href="/user-console/">用户中心</a>
        <a href="/agent-console/">代理入口 <ArrowRight :size="15" /></a
        ><a :href="entries.admin">管理控制台</a
        ><a href="#" @click.prevent="helpOpen = true"
          ><CircleHelp :size="15" />使用帮助</a
        >
      </div>
      <AppearanceMenu />
    </header>
    <section class="query-stage">
      <div class="stage-index mono">
        YN / LICENSE INFRASTRUCTURE<br />PUBLIC VERIFY NODE 01
      </div>
      <div class="query-copy">
        <span class="eyebrow"
          ><ShieldCheck :size="15" /> AUTHORIZATION LOOKUP</span
        >
        <h1>软件授权<br /><em>公开验真。</em></h1>
        <p>查询应用授权与代理身份，让正版使用和交易有据可查。</p>
      </div>
      <div class="lookup-panel">
        <div class="query-types" role="group" aria-label="验真对象">
          <button
            type="button"
            :class="{ active: mode === 'license' }"
            :aria-pressed="mode === 'license'"
            @click="mode = 'license'"
          >
            授权验真</button
          ><button
            type="button"
            :class="{ active: mode === 'agent' }"
            :aria-pressed="mode === 'agent'"
            @click="mode = 'agent'"
          >
            代理验真
          </button>
        </div>
        <AgentVerify v-if="mode === 'agent'" />
        <form v-else class="query-form" @submit.prevent="submit">
          <label class="field">选择应用<select v-model="applicationID" aria-label="查询应用" required><option value="" disabled>请选择公开应用</option><option v-for="a in applications" :key="a.app_id" :value="a.app_id">{{a.name}}</option></select></label><p v-if="!applications.length">暂无公开应用，请联系产品开发者或登录用户中心查询。</p><label for="license-query">授权查询</label>
          <div class="query-types" role="group" aria-label="查询类型">
            <button
              v-for="type in queryTypes"
              :key="type.value"
              type="button"
              :class="{ active: queryType === type.value }"
              :aria-pressed="queryType === type.value"
              @click="selectQueryType(type.value)"
            >
              <component :is="type.icon" :size="15" />{{ type.label }}
            </button>
          </div>
          <div class="query-input">
            <Search :size="22" /><input
              id="license-query"
              :type="queryType==='card'||query.toUpperCase().startsWith('YN2-')?'password':'text'"
              v-model="query"
              required
              autocomplete="off"
              :placeholder="placeholders[queryType]"
              autofocus
            /><button :disabled="loading || !query.trim()||!applicationID" aria-label="查询">
              <RotateCcw v-if="loading" class="spin" :size="20" /><ArrowRight
                v-else
                :size="22"
              />
            </button>
          </div>
          <CaptchaField
            v-if="needCaptcha"
            ref="captchaField"
            v-model="captchaCode"
            endpoint="/v1/public-captcha"
          />
          <p class="error">{{ error }}</p>
        </form>
      </div>
      <div class="stage-rule">
        <span></span
        ><small class="mono">ENCRYPTED / TRACEABLE / VERIFIABLE</small>
      </div>
    </section>
    <Transition name="result">
      <section v-if="result && mode === 'license'" class="query-result">
        <div class="result-title"><div><span class="eyebrow"><CheckCircle2 :size="15" /> QUERY COMPLETE</span><h2>{{result.application.name}}</h2><code>{{result.application.app_id}}</code></div><span class="status" :class="result.authorized?'active':result.status">{{publicStatus(result)}}</span></div>
        <div class="result-list"><article><dl><div><dt>授权状态</dt><dd>{{publicStatus(result)}}</dd></div><div v-if="result.model"><dt>授权模型</dt><dd>{{({perpetual:'永久',subscription:'订阅',floating:'浮动席位',metered:'按量'} as any)[result.model]}}</dd></div><div v-if="result.model"><dt>使用期限</dt><dd>{{result.perpetual?'永久使用':result.awaiting_activation?'首次激活后起算':formatTime(result.usage_expires_at)}}</dd></div><div v-if="result.model"><dt>更新服务</dt><dd>{{result.updates_allowed?'当前可在线更新':'当前无在线更新资格'}}</dd></div><div v-if="result.updates_until"><dt>更新截止</dt><dd>{{formatTime(result.updates_until)}}</dd></div><div><dt>查询时间</dt><dd>{{formatTime(result.server_time)}}</dd></div></dl></article></div><p class="result-time">公开结果仅用于验真，不会签发运行凭证。账号、租户或设备授权请登录用户中心查看。</p>
        <button class="btn secondary" @click="clear">
          <KeyRound :size="16" />查询其他授权
        </button>
      </section>
    </Transition>
  </main>
  <HelpPanel :open="helpOpen" @close="helpOpen = false" />
</template>
