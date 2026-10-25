<script setup lang="ts">
import { X } from "lucide-vue-next";
import { onBeforeUnmount, watch } from "vue";

const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ close: [] }>();

const topics = [
  {
    title: "开发者创建应用后如何接入授权？",
    body: "在应用的「接入授权」指南核对授权站地址、App Key 和绑定值，下载 Node.js、Go 或 Java 接入包。包内含 SDK 源码、配置及可运行 demo，按步骤用测试卡激活并验证受保护接口；上线前也要验证到期、吊销、错误绑定和授权站断网时的拒绝行为。",
  },
  {
    title: "如何激活卡密？",
    body: "登录用户中心，在「激活卡密」中填写应用代码、卡密和绑定信息即可。仅未激活的卡密可以首次认领；已归属其他账号的卡密无法用你的账号激活。",
  },
  {
    title: "什么是绑定信息？",
    body: "绑定信息由应用方规定，可能是域名、服务器 IP、设备标识或应用账号。填写的绑定信息必须与应用内上报的一致，否则验证会失败。",
  },
  {
    title: "如何续费？",
    body: "在用户中心的授权卡片中点击「卡密续费」，选择当前绑定并输入续费卡密。未到期的期限从当前到期时间顺延，已到期的期限从续费时间起算；使用与更新权益按应用规则分别计算。",
  },
  {
    title: "如何更换绑定（换绑）？",
    body: "在授权卡片中点击「更换绑定」。换绑受应用规则限制：可能禁止换绑、限制次数或要求冷却时间；成功后旧绑定立即失效，并扣除一次换绑额度。",
  },
  {
    title: "到期后会发生什么？",
    body: "订阅应用的使用期限到期后停止使用，是否有宽限期由产品规则决定；永久使用的应用可保留旧版。更新服务到期只影响获取新版本，使用与更新资格在用户中心分别展示。",
  },
  {
    title: "离线环境如何使用？",
    body: "能否离线使用取决于应用的接入实现和授权策略，请向应用方确认。支持离线缓存的应用仍受离线窗口和授权期限限制；完全离线设备需要申请离线授权文件，文件有效期最长 365 天，到期需重新签发。",
  },
  {
    title: "卡密安全提示",
    body: "卡密是付费凭证：请勿向任何人透露明文卡密，通过官方渠道购买，并在公开验真页核对授权状态。发现卡密泄漏应立即联系发卡方作废止损。",
  },
  {
    title: "遇到问题如何求助？",
    body: "账号或授权异常时，先在用户中心查看授权状态与错误提示；仍无法解决时联系平台管理员，或通过安全申诉通道提交申诉。",
  },
];

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") emit("close");
}
watch(
  () => props.open,
  (open) => {
    if (open) addEventListener("keydown", onKeydown);
    else removeEventListener("keydown", onKeydown);
  },
);
onBeforeUnmount(() => removeEventListener("keydown", onKeydown));
</script>
<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="open" class="modal-backdrop" @click.self="emit('close')">
        <div
          class="modal-card help-card"
          role="dialog"
          aria-modal="true"
          aria-label="使用帮助"
        >
          <div class="modal-head">
            <div>
              <h3>使用帮助</h3>
              <p>激活、续费、换绑与授权安全的常见问题</p>
            </div>
            <button
              class="icon-btn"
              type="button"
              title="关闭"
              @click="emit('close')"
            >
              <X :size="17" />
            </button>
          </div>
          <div class="modal-body help-body">
            <details v-for="t in topics" :key="t.title">
              <summary>{{ t.title }}</summary>
              <p>{{ t.body }}</p>
            </details>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
