import { ref } from "vue";

export type ThemePreference = "light" | "dark" | "system";
const storageKey = "yn.appearance";
const systemTheme = window.matchMedia("(prefers-color-scheme: dark)");
function normalize(value: unknown): ThemePreference {
  return value === "light" || value === "dark" ? value : "system";
}
function savedPreference(): ThemePreference {
  try {
    return normalize(localStorage.getItem(storageKey));
  } catch {
    return normalize(document.documentElement.dataset.themePreference);
  }
}
export const themePreference = ref<ThemePreference>(savedPreference());
export const resolvedTheme = ref<"light" | "dark">("light");

function applyTheme() {
  resolvedTheme.value =
    themePreference.value === "system"
      ? systemTheme.matches
        ? "dark"
        : "light"
      : themePreference.value;
  const root = document.documentElement;
  root.dataset.theme = resolvedTheme.value;
  root.dataset.themePreference = themePreference.value;
  root.style.colorScheme = resolvedTheme.value;
  let meta = document.querySelector<HTMLMetaElement>(
    'meta[name="theme-color"]',
  );
  if (!meta) {
    meta = document.createElement("meta");
    meta.name = "theme-color";
    document.head.append(meta);
  }
  meta.content = resolvedTheme.value === "dark" ? "#121713" : "#f3f5f2";
}
export function setThemePreference(preference: ThemePreference) {
  themePreference.value = normalize(preference);
  try {
    localStorage.setItem(storageKey, themePreference.value);
  } catch {
    // Private/restricted browsers can still change appearance for this page.
  }
  applyTheme();
}
systemTheme.addEventListener("change", applyTheme);
window.addEventListener("storage", (event) => {
  if (event.key === storageKey || event.key === null) {
    themePreference.value = normalize(event.newValue);
    applyTheme();
  }
});
applyTheme();
