export interface AuthorizationTerms {
  version: number;
  card_purpose: string;
  forms: string[];
  bind_mode: string;
  binding_kinds: string[];
  include_apex: boolean;
  subdomain_depth: string;
  max_bindings: number;
  require_instance: boolean;
  max_deployments: number;
  max_devices: number;
  max_concurrent_seats: number;
  seat_lease_seconds: number;
  offline_grace_days: number;
  license_mode: string;
  update_duration_days: number;
  expire_grace_days: number;
  unbind_limit: number;
  unbind_cooldown_hours: number;
  features: string[];
}
export const bindingNames: Record<string, string> = {
  domain: "精确域名",
  domain_wildcard: "泛域名",
  ip: "单个 IP",
  ip_cidr: "IP 网段（CIDR）",
  device: "设备",
  instance: "安装实例",
  account: "产品账号",
  tenant: "组织／租户",
  cluster: "集群",
  composite: "组合绑定",
  none: "不绑定",
};
export function authorizationDefaults(product: any = {}): AuthorizationTerms {
  return JSON.parse(
    JSON.stringify({
      version: 2,
      card_purpose: "license",
      forms: ["service"],
      bind_mode: product.bind_mode || "device",
      binding_kinds: [],
      include_apex: false,
      subdomain_depth: "one",
      max_bindings: product.max_bind_count > 0 ? product.max_bind_count : 1,
      require_instance: false,
      max_deployments: 1,
      max_devices: 0,
      max_concurrent_seats: 0,
      seat_lease_seconds: 120,
      offline_grace_days: product.offline_grace_days ?? 0,
      license_mode: product.license_mode || "subscription",
      update_duration_days: product.update_duration_days || 0,
      expire_grace_days: product.expire_grace_days || 0,
      unbind_limit: product.unbind_limit || 0,
      unbind_cooldown_hours: product.unbind_cooldown_hours || 0,
      features: [],
      ...product.authorization_terms,
    }),
  );
}
export function termsSummary(t: any): string {
  if (!t) return "沿用应用规则";
  return `${bindingNames[t.bind_mode] || t.bind_mode} · ${t.license_mode === "perpetual" ? "永久使用" : "订阅使用"} · ${t.max_bindings} 个绑定${t.require_instance ? ` · ${t.max_deployments} 个实例` : ""}${t.max_concurrent_seats ? ` · ${t.max_concurrent_seats} 个并发席位` : ""}`;
}
