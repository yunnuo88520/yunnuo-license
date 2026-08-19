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
    agent_owner: "主账号",
    agent_manager: "经理",
    agent_staff: "员工",
    agent_readonly: "只读",
  })[value || ""] ||
  value ||
  "-";
