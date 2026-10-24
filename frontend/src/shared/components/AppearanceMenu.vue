<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import { Check, Monitor, Moon, Sun } from "lucide-vue-next";
import {
  resolvedTheme,
  setThemePreference,
  themePreference,
  type ThemePreference,
} from "../theme";

const options = [
  { value: "light" as const, label: "浅色", icon: Sun },
  { value: "dark" as const, label: "深色", icon: Moon },
  { value: "system" as const, label: "跟随系统", icon: Monitor },
];
const selected = computed(() =>
  options.find((option) => option.value === themePreference.value)!,
);
const opened = ref(false),
  root = ref<HTMLElement>(),
  trigger = ref<HTMLButtonElement>();
function close(restoreFocus = false) {
  opened.value = false;
  if (restoreFocus) trigger.value?.focus();
}
async function toggle() {
  opened.value = !opened.value;
  if (opened.value) {
    await nextTick();
    root.value
      ?.querySelector<HTMLButtonElement>('[aria-checked="true"]')
      ?.focus();
  }
}
function choose(value: ThemePreference) {
  setThemePreference(value);
  close(true);
}
function keyboard(event: KeyboardEvent) {
  if (event.key === "Escape") {
    event.preventDefault();
    close(true);
  } else if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
    event.preventDefault();
    const buttons = [
      ...root.value!.querySelectorAll<HTMLButtonElement>(
        '[role="menuitemradio"]',
      ),
    ];
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const target =
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? buttons.length - 1
          : (index + (event.key === "ArrowDown" ? 1 : -1) + buttons.length) %
            buttons.length;
    buttons[target]?.focus();
  }
}
function outside(event: PointerEvent) {
  if (!root.value?.contains(event.target as Node)) close();
}
onMounted(() => document.addEventListener("pointerdown", outside));
onUnmounted(() => document.removeEventListener("pointerdown", outside));
</script>
<template>
  <div
    ref="root"
    class="appearance"
    @focusout="
      (event) => {
        if (!root?.contains(event.relatedTarget as Node)) close();
      }
    "
  >
    <button
      ref="trigger"
      type="button"
      class="appearance-trigger"
      aria-label="外观主题"
      aria-haspopup="menu"
      :aria-expanded="opened"
      :title="`外观主题：${selected.label}（当前${resolvedTheme === 'dark' ? '深色' : '浅色'}）`"
      @click="toggle"
      @keydown.down.prevent="opened || toggle()"
      @keydown.esc="close(true)"
    >
      <component :is="selected.icon" :size="18" aria-hidden="true" />
      <span class="appearance-label">{{ selected.label }}</span>
    </button>
    <div
      v-if="opened"
      class="appearance-options"
      role="menu"
      aria-label="选择外观主题"
      @keydown="keyboard"
    >
      <button
        v-for="option in options"
        :key="option.value"
        type="button"
        role="menuitemradio"
        :aria-checked="themePreference === option.value"
        @click="choose(option.value)"
      >
        <component :is="option.icon" :size="17" aria-hidden="true" />{{
          option.label
        }}
        <Check
          v-if="themePreference === option.value"
          class="selection"
          :size="16"
          aria-hidden="true"
        />
      </button>
    </div>
  </div>
</template>
<style scoped>
.appearance {
  position: relative;
  flex-shrink: 0;
  color: var(--ink);
  font-size: 12px;
  font-weight: 500;
}
.appearance-trigger {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 40px;
  padding: 0 12px;
  color: var(--ink);
  background: var(--paper);
  border: 1px solid var(--line);
  border-radius: 7px;
}
.appearance-trigger:hover {
  background: var(--surface-hover);
}
.appearance-options {
  position: absolute;
  z-index: 80;
  top: calc(100% + 9px);
  right: 0;
  width: 178px;
  padding: 6px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--paper);
  box-shadow: var(--shadow);
}
.appearance-options button {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  min-height: 42px;
  border: 0;
  border-radius: 6px;
  padding: 0 10px;
  color: var(--ink);
  background: transparent;
  text-align: left;
}
.appearance-options button:hover,
.appearance-options button:focus-visible,
.appearance-options button[aria-checked="true"] {
  color: var(--green);
  background: var(--green-soft);
}
.selection {
  margin-left: auto;
}
@media (max-width: 620px) {
  .appearance-trigger {
    min-width: 44px;
    min-height: 44px;
    padding: 0 10px;
  }
  .appearance-label {
    display: none;
  }
  .appearance-options button {
    min-height: 44px;
  }
}
</style>
