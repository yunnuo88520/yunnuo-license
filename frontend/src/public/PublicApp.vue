<script setup lang="ts">
import { ref } from "vue";
import {
  ArrowRight,
  CheckCircle2,
  KeyRound,
  RotateCcw,
  Search,
  ShieldCheck,
} from "lucide-vue-next";
import { request } from "../shared/api";
import { formatTime, statusLabel } from "../shared/format";

const query = ref("");
const loading = ref(false);
const error = ref("");
const result = ref<any>(null);
async function submit() {
  const value = query.value.trim();
  if (!value) return;
  loading.value = true;
  error.value = "";
  result.value = null;
  try {
    result.value = await request("/v1/licenses/query", {
      method: "POST",
      body: JSON.stringify(
        value.toLowerCase().startsWith("lic_")
          ? { license_no: value }
          : { card_code: value },
      ),
    });
  } catch (e: any) {
    error.value =
      e.code === "AUTHORIZATION_NOT_FOUND"
        ? "未查询到该授权"
        : "查询失败，请稍后重试";
  } finally {
    loading.value = false;
  }
}
function clear() {
  query.value = "";
  result.value = null;
  error.value = "";
}
</script>
<template>
  <main class="query-canvas">
    <header class="query-nav">
      <a class="query-brand" href="/"
        ><img src="/assets/yunnuo-mark.svg" alt="" /><span>允诺云授权</span></a
      >
      <div>
        <a href="/agent-console/">代理入口 <ArrowRight :size="15" /></a
        ><a href="/admin-console/">管理控制台</a>
      </div>
    </header>
    <section class="query-stage">
      <div class="stage-index mono">
        YN / LICENSE INFRASTRUCTURE<br />PUBLIC VERIFY NODE 01
      </div>
      <div class="query-copy">
        <span class="eyebrow"
          ><ShieldCheck :size="15" /> AUTHORIZATION LOOKUP</span
        >
        <h1>验证每一份<br /><em>允诺。</em></h1>
        <p>输入卡密或授权编号，立即查看授权状态与有效期。</p>
      </div>
      <form class="query-form" @submit.prevent="submit">
        <label for="license-query">授权凭证</label>
        <div class="query-input">
          <Search :size="22" /><input
            id="license-query"
            v-model="query"
            autocomplete="off"
            placeholder="卡密 / lic_ 授权编号"
            autofocus
          /><button :disabled="loading || !query.trim()" aria-label="查询">
            <RotateCcw v-if="loading" class="spin" :size="20" /><ArrowRight
              v-else
              :size="22"
            />
          </button>
        </div>
        <p class="error">{{ error }}</p>
      </form>
      <div class="stage-rule">
        <span></span
        ><small class="mono">ENCRYPTED / TRACEABLE / VERIFIABLE</small>
      </div>
    </section>
    <Transition name="result">
      <section v-if="result" class="query-result">
        <div class="result-title">
          <div>
            <span class="eyebrow"
              ><CheckCircle2 :size="15" /> QUERY COMPLETE</span
            >
            <h2>{{ result.product_name }}</h2>
            <code>{{ result.product_code }}</code>
          </div>
          <span
            class="status"
            :class="result.license_status || result.card_status"
            >{{
              statusLabel(result.license_status || result.card_status)
            }}</span
          >
        </div>
        <dl>
          <div v-if="result.license_no">
            <dt>授权编号</dt>
            <dd class="mono">{{ result.license_no }}</dd>
          </div>
          <div v-if="result.card_status">
            <dt>卡密状态</dt>
            <dd>{{ statusLabel(result.card_status) }}</dd>
          </div>
          <div>
            <dt>授权状态</dt>
            <dd>{{ statusLabel(result.license_status) }}</dd>
          </div>
          <div>
            <dt>授权时长</dt>
            <dd>
              {{
                result.is_permanent
                  ? "永久"
                  : result.duration_days
                    ? `${result.duration_days} 天`
                    : "按到期时间"
              }}
            </dd>
          </div>
          <div>
            <dt>激活时间</dt>
            <dd>{{ formatTime(result.activated_at) }}</dd>
          </div>
          <div>
            <dt>到期时间</dt>
            <dd>
              {{ result.is_permanent ? "永久" : formatTime(result.expired_at) }}
            </dd>
          </div>
          <div>
            <dt>最近验证</dt>
            <dd>{{ formatTime(result.last_verify_at) }}</dd>
          </div>
          <div>
            <dt>服务器时间</dt>
            <dd>{{ formatTime(result.server_time) }}</dd>
          </div>
        </dl>
        <button class="btn secondary" @click="clear">
          <KeyRound :size="16" />查询其他授权
        </button>
      </section>
    </Transition>
  </main>
</template>
