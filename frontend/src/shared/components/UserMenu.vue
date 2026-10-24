<script setup lang="ts">
import { ChevronDown, KeyRound, LogOut } from "lucide-vue-next";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

const props = withDefaults(
  defineProps<{
    name: string;
    role?: string;
    passwordAction?: string;
  }>(),
  { role: "", passwordAction: "" },
);
const emit = defineEmits<{ password: []; logout: [] }>();
const open = ref(false);
const root = ref<HTMLElement>();
const initial = computed(() => props.name.trim().slice(0, 1).toUpperCase());
function onDocClick(e: MouseEvent) {
  if (open.value && root.value && !root.value.contains(e.target as Node))
    open.value = false;
}
function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") open.value = false;
}
onMounted(() => {
  addEventListener("click", onDocClick);
  addEventListener("keydown", onKeydown);
});
onBeforeUnmount(() => {
  removeEventListener("click", onDocClick);
  removeEventListener("keydown", onKeydown);
});
</script>
<template>
  <div ref="root" class="user-menu">
    <button
      type="button"
      class="user-trigger"
      :aria-expanded="open"
      aria-haspopup="menu"
      @click="open = !open"
    >
      <span class="avatar" aria-hidden="true">{{ initial }}</span>
      <span class="user-meta">
        <b>{{ name }}</b>
        <small v-if="role">{{ role }}</small>
      </span>
      <ChevronDown :size="14" class="user-caret" />
    </button>
    <Transition name="pop">
      <div v-if="open" class="user-pop" role="menu" :aria-label="`${name} 的账户菜单`">
        <button
          v-if="passwordAction"
          type="button"
          role="menuitem"
          @click="
            open = false;
            emit('password');
          "
        >
          <KeyRound :size="15" />{{ passwordAction }}
        </button>
        <button
          type="button"
          role="menuitem"
          class="danger"
          @click="
            open = false;
            emit('logout');
          "
        >
          <LogOut :size="15" />退出登录
        </button>
      </div>
    </Transition>
  </div>
</template>
