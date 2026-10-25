import { ApiError } from "./api";

// 面向最终用户的错误文案：覆盖授权业务最常见的错误码，
// 未覆盖的错误退回通用提示并保留错误码便于报障。
const friendlyMessages: Record<string, string> = {
  CARD_INVALID: "卡密不正确或不存在，请核对后重新输入",
  CARD_VOIDED: "该卡密已被作废，请联系发卡方处理",
  CARD_USED_BY_OTHER_BINDING: "该卡密已被其他绑定使用，无法重复激活",
  CARD_CANNOT_VOID: "仅未使用的卡密可以作废",
  OWNER_REQUIRED: "该卡密已归属其他账号，无法用当前账号激活",
  LICENSE_NOT_FOUND: "授权不存在或编号有误，请核对后重试",
  AUTHORIZATION_NOT_FOUND: "未查询到该授权，请核对查询内容",
  LICENSE_EXPIRED: "授权已到期，请续费后继续使用",
  LICENSE_REVOKED: "授权已被吊销，请联系平台管理员",
  LICENSE_PERMANENT: "永久授权无需续费",
  BINDING_REQUIRED: "请填写绑定信息（域名 / IP / 设备 / 账号）",
  BINDING_MISMATCH: "绑定方式与产品要求不符，请确认后重试",
  DEVICE_LIMIT_EXCEEDED: "绑定数量已达上限，请先解绑或更换绑定",
  REBIND_DISABLED: "该应用不允许更换绑定",
  REBIND_LIMIT_REACHED: "换绑次数已用完，请联系平台管理员",
  UNBIND_COOLDOWN: "换绑冷却时间内，请稍后再试",
  INVALID_CREDENTIALS: "账号或密码不正确",
  INVALID_APP_KEY: "应用代码有误，请核对后重试",
  PRODUCT_DISABLED: "该应用已被停用，请联系平台管理员",
  RISK_IP_BLOCKED: "当前网络被限制访问，请联系平台管理员",
  RISK_DEVICE_BLOCKED: "当前设备被限制访问，请联系平台管理员",
  RATE_LIMITED: "操作过于频繁，请稍后再试",
  EMAIL_TAKEN: "该邮箱已被其他账号使用",
  EMAIL_INVALID: "邮箱地址格式不正确",
  AGENT_QUOTA_NOT_ENOUGH: "可用额度不足，请联系管理员补充",
  AGENT_PRODUCT_NOT_ALLOWED: "没有该产品的发卡权限",
  PACKAGE_PRICE_CHANGED: "套餐价格已变化，请重新确认后生成",
  UPDATES_EXPIRED: "更新服务已到期，续费后可获取新版本",
};

export function friendlyError(e: unknown): string {
  if (e instanceof ApiError) {
    if (friendlyMessages[e.code]) return friendlyMessages[e.code];
    return `操作失败（${e.code}），请稍后重试`;
  }
  if (e instanceof Error && e.message) return e.message;
  return "操作失败，请稍后重试";
}
