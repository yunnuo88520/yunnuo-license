package licensing

import (
	"regexp"
	"strings"
	"time"
)

var featureIdentifier = regexp.MustCompile(`^[\pL\pN_.:-]+$`)

var supportedModels = map[string]bool{"perpetual": true, "subscription": true, "floating": true, "metered": true}
var supportedBindings = map[string]bool{"machine_id": true, "domain": true, "ip": true, "seat": true, "installation": true, "account": true, "tenant": true, "cluster": true}
var supportedForms = map[string]bool{"web": true, "container": true, "cluster": true, "gui": true, "cli": true, "service": true, "plugin": true, "mobile": true, "iot": true, "saas": true}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func uniqueAllowed(values []string, allowed map[string]bool) bool {
	if len(values) == 0 || len(values) > len(allowed) {
		return false
	}
	seen := map[string]bool{}
	for _, v := range values {
		if !allowed[v] || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
func DefaultVerifyPolicy() VerifyPolicy {
	return VerifyPolicy{IPSource: "egress", JWTTTLSeconds: 86400, VerifyIntervalSeconds: 3600, NetworkGraceSeconds: 3 * 86400, SeatLeaseSeconds: 120, MaxBorrowSeconds: 86400, KeyOverlapSeconds: 8 * 86400, DomainRuleMode: "license_binding", IPRuleMode: "license_binding"}
}
func normalizeApplication(a *Application) error {
	if e := normalizeBlockPolicy(&a.BlockPolicy); e != nil {
		return e
	}
	if len(a.Group) > 160 {
		return invalid("应用分组名称过长")
	}
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || len(a.Name) > 160 {
		return invalid("应用名称必填，最长 160 字节")
	}
	if len(a.AllowedModels) == 0 {
		a.AllowedModels = []string{"perpetual", "subscription"}
	}
	if len(a.AllowedBindings) == 0 {
		a.AllowedBindings = []string{"machine_id", "domain", "ip", "installation"}
	}
	if len(a.Forms) == 0 {
		a.Forms = []string{"web", "container", "gui", "cli", "service"}
	}
	if !uniqueAllowed(a.AllowedModels, supportedModels) || !uniqueAllowed(a.AllowedBindings, supportedBindings) || !uniqueAllowed(a.Forms, supportedForms) {
		return invalid("请选择支持的模型、绑定与部署形态")
	}
	if a.Status != "active" && a.Status != "suspended" && a.Status != "archived" {
		return invalid("应用状态无效")
	}
	p := a.VerifyPolicy
	if p == (VerifyPolicy{}) {
		p = DefaultVerifyPolicy()
	}
	if p.JWTTTLSeconds < 60 || p.JWTTTLSeconds > 3*86400 || p.VerifyIntervalSeconds < 1 || p.VerifyIntervalSeconds > p.JWTTTLSeconds || p.NetworkGraceSeconds < 0 || p.NetworkGraceSeconds > 30*86400 || p.SubscriptionGraceSeconds < 0 || p.SubscriptionGraceSeconds > 365*86400 || p.SeatLeaseSeconds < 30 || p.SeatLeaseSeconds > 3600 || p.MaxBorrowSeconds < 0 || p.MaxBorrowSeconds > 30*86400 || p.KeyOverlapSeconds < 0 || p.KeyOverlapSeconds > 366*86400 {
		return invalid("令牌、刷新、宽限或租约配置超出范围")
	}
	if p.IPSource == "" {
		p.IPSource = "egress"
	}
	if p.IPSource != "egress" && p.IPSource != "declared" {
		return invalid("IP 来源策略无效")
	}
	if p.DomainRuleMode != "license_binding" && p.DomainRuleMode != "application_allowlist" || p.IPRuleMode != "license_binding" && p.IPRuleMode != "application_allowlist" {
		return invalid("绑定白名单策略无效")
	}
	a.VerifyPolicy = p
	return nil
}
func NormalizeTerms(app Application, t Terms) (Terms, error) {
	if !supportedModels[t.Model] || !contains(app.AllowedModels, t.Model) {
		return t, invalid("此应用未启用该授权模型")
	}
	if !uniqueAllowed(t.BindingTypes, supportedBindings) {
		return t, invalid("请选择套餐绑定类型")
	}
	for _, binding := range t.BindingTypes {
		if !contains(app.AllowedBindings, binding) {
			return t, invalid("套餐绑定类型必须由应用启用")
		}
	}
	if len(t.Forms) == 0 {
		t.Forms = append([]string{}, app.Forms...)
	}
	if !uniqueAllowed(t.Forms, supportedForms) {
		return t, invalid("部署形态无效")
	}
	for _, form := range t.Forms {
		if !contains(app.Forms, form) {
			return t, invalid("套餐形态未由应用启用")
		}
	}
	if t.StartsOn == "" {
		t.StartsOn = "activation"
	}
	if t.StartsOn != "activation" && t.StartsOn != "issue" {
		return t, invalid("有效期起算方式无效")
	}
	if t.Model == "perpetual" {
		t.Perpetual = true
		t.DurationDays = 0
	}
	if t.Model == "subscription" {
		t.Perpetual = false
	}
	if t.Model == "floating" {
		if len(t.BindingTypes) != 1 || t.BindingTypes[0] != "seat" || t.MaxSeats < 1 {
			return t, invalid("浮动授权需要席位绑定及正数席位额度")
		}
	} else if contains(t.BindingTypes, "seat") {
		return t, invalid("席位绑定仅用于浮动模型")
	}
	if !t.Perpetual && (t.DurationDays < 1 || t.DurationDays > 36500) || t.UpdateDurationDays < 0 || t.UpdateDurationDays > 36500 {
		return t, invalid("使用或更新期限无效")
	}
	if t.MaxBindingGroups == 0 {
		t.MaxBindingGroups = 1
	}
	if t.MaxBindingGroups < 1 || t.MaxBindingGroups > 1000 {
		return t, invalid("授权锚点组数量无效")
	}
	if t.MaxActivations == 0 {
		t.MaxActivations = 1
	}
	if t.MaxDomainAliases == 0 {
		t.MaxDomainAliases = 2
	}
	if t.MaxActivations < 1 || t.MaxActivations > 1000 || t.MaxSeats < 0 || t.MaxSeats > 10000 || t.MaxDomainAliases < 1 || t.MaxDomainAliases > 128 || t.RebindLimit < -1 || t.RebindLimit > 10000 || t.RebindCooldownSeconds < 0 || t.RebindCooldownSeconds > 3650*86400 || len(t.Features) > 100 {
		return t, invalid("数量、别名或换绑限制无效")
	}
	if t.Model == "metered" {
		if t.UsageQuota < 1 || t.UsageQuota > 1_000_000_000_000 || t.UsageUnit == "" || len(t.UsageUnit) > 64 || (t.UsagePeriod != "lifetime" && t.UsagePeriod != "monthly") {
			return t, invalid("按量模型需要额度、计量单位和期间")
		}
	} else {
		t.UsageQuota = 0
		t.UsageUnit = ""
		t.UsagePeriod = ""
	}
	if len(t.RenewalTargetPlanIDs) > 1000 {
		return t, invalid("续费目标套餐过多")
	}
	targets := map[string]bool{}
	for _, id := range t.RenewalTargetPlanIDs {
		if id == "" || len(id) > 96 || targets[id] {
			return t, invalid("续费目标套餐无效或重复")
		}
		targets[id] = true
	}
	seen := map[string]bool{}
	for _, feature := range t.Features {
		if !featureIdentifier.MatchString(feature) || len(feature) > 64 || feature != strings.TrimSpace(feature) || seen[feature] {
			return t, invalid("功能标识仅包含文字、数字及 _ . : -，且不可重复")
		}
		seen[feature] = true
	}
	return t, nil
}
func usageDeadline(app Application, l License) *time.Time {
	if l.Terms.Perpetual || l.ExpiresAt == nil {
		return nil
	}
	v := l.ExpiresAt.Add(time.Duration(app.VerifyPolicy.SubscriptionGraceSeconds) * time.Second)
	return &v
}
func checkUsable(app Application, l License, now time.Time) error {
	if l.RenewalOnly {
		return forbidden("LICENSE_RENEWAL_ONLY", "此记录仅为续费凭证，不可独立激活使用")
	}
	if app.Status != "active" {
		return forbidden("APPLICATION_DISABLED", "应用已停用")
	}
	switch l.Status {
	case "suspended":
		return forbidden("LICENSE_SUSPENDED", "授权已暂停")
	case "revoked":
		return forbidden("LICENSE_REVOKED", "授权已吊销")
	case "expired":
		return forbidden("LICENSE_EXPIRED", "授权已到期")
	case "active":
	default:
		return forbidden("LICENSE_UNUSABLE", "授权状态不可用")
	}
	if deadline := usageDeadline(app, l); deadline != nil && !now.Before(*deadline) {
		return forbidden("LICENSE_EXPIRED", "授权使用期限已结束")
	}
	if !contains(app.AllowedModels, l.Terms.Model) {
		return forbidden("MODEL_DISABLED", "此应用已停用该授权模型")
	}
	return nil
}
