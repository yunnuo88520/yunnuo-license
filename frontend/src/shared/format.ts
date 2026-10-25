export const formatTime = (value?: string | null) =>
  value
    ? new Intl.DateTimeFormat("zh-CN", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      }).format(new Date(value))
    : "-";

export const shortId = (value?: string) =>
  !value ? "-" : value.length > 18 ? `${value.slice(0, 15)}...` : value;

export type ExpiryLevel = "permanent" | "ok" | "warn" | "urgent" | "expired";

// Expiry signal shared by every console: amber inside 30 days, red inside 7.
export const expiryInfo = (
  expiredAt?: string | null,
  now: Date = new Date(),
): { level: ExpiryLevel; daysLeft: number; label: string } => {
  if (!expiredAt) return { level: "permanent", daysLeft: Infinity, label: "永久" };
  const msLeft = new Date(expiredAt).getTime() - now.getTime();
  const daysLeft = Math.ceil(msLeft / 86400000);
  if (msLeft <= 0) return { level: "expired", daysLeft, label: "已到期" };
  if (daysLeft <= 7)
    return { level: "urgent", daysLeft, label: `剩 ${daysLeft} 天` };
  if (daysLeft <= 30)
    return { level: "warn", daysLeft, label: `剩 ${daysLeft} 天` };
  return { level: "ok", daysLeft, label: `剩 ${daysLeft} 天` };
};

export const statusLabel = (value?: string) =>
  ({
    active: "有效",
    disabled: "已停用",
    suspended: "已暂停",
    expired: "已过期",
    revoked: "已吊销",
    unused: "未使用",
    activated: "已激活",
    consumed_for_renewal: "已续费使用",
    voided: "已作废",
    open: "待处理",
    resolved: "已解决",
    success: "成功",
    failed: "失败",
    unbound: "已解绑",
    kicked: "已移除",
  })[value || ""] ||
  value ||
  "-";

export const roleLabel = (value?: string) =>
  ({
    super_admin: "超级管理员",
    admin: "管理员",
    operator: "运营人员",
    auditor: "审计人员",
    developer: "开发者",
    agent_owner: "主账号",
    agent_manager: "经理",
    agent_staff: "员工",
    agent_readonly: "只读",
  })[value || ""] ||
  value ||
  "-";

// 服务端心跳视图：online 24 小时内有上报，idle 超一天但在容忍期内，offline 已失联。
export const onlineStateLabel = (state?: string, days?: number) => {
  switch (state) {
    case "online":
      return "在线";
    case "idle":
      return `空闲 ${days ?? 1} 天`;
    case "offline":
      return `失联 ${days ?? 1} 天`;
    default:
      return "";
  }
};
