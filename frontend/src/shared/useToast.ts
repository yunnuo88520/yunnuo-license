import { ref } from "vue";

export const toastMessage = ref("");
let timer = 0;
export function toast(message: unknown) {
  toastMessage.value =
    message instanceof Error ? message.message : String(message);
  window.clearTimeout(timer);
  timer = window.setTimeout(() => {
    toastMessage.value = "";
  }, 2800);
}
