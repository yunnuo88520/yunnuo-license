import { ref } from "vue";
import { request } from "./api";
export const entries = ref<Record<string, string>>({
  admin: "/admin",
  agent: "/agent-console/",
  public: "/",
  user: "/user-console/",
  developer: "/developer-console/",
});
export async function loadEntries() {
  const status = await request<any>("/v1/setup/status");
  if (status.entries) entries.value = status.entries;
  if (!status.initialized && location.pathname !== "/install")
    location.replace("/install");
  return entries.value;
}
