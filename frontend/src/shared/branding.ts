import { computed, ref } from "vue";
import { request } from "./api";

export interface SiteSettings {
  site_name: string;
  browser_title: string;
  logo_data_url?: string;
  favicon_data_url?: string;
  updated_at?: string;
}

const defaults: SiteSettings = {
  site_name: "允诺云授权",
  browser_title: "允诺云授权",
};

export const branding = ref<SiteSettings>({ ...defaults });
export const logoSource = computed(
  () => branding.value.logo_data_url || "/assets/yunnuo-mark.svg",
);

export function applyBranding(settings: SiteSettings, section = "") {
  branding.value = { ...defaults, ...settings };
  document.title = [branding.value.browser_title, section]
    .filter(Boolean)
    .join(" · ");
  const icon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (icon) icon.href = branding.value.favicon_data_url || "/assets/yunnuo-mark.svg";
}

export async function loadBranding(section = "") {
  const settings = await request<SiteSettings>("/v1/site/settings");
  applyBranding(settings, section);
  return settings;
}
