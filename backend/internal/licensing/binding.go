package licensing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

var hostLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var fingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func canonicalHost(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "://") {
		u, e := url.Parse(value)
		if e != nil || u.User != nil {
			return "", invalid("域名无效")
		}
		value = u.Hostname()
	} else if host, _, e := net.SplitHostPort(value); e == nil {
		value = host
	}
	value = strings.TrimSuffix(strings.ToLower(value), ".")
	ascii, e := idna.Lookup.ToASCII(value)
	if e != nil || len(ascii) == 0 || len(ascii) > 253 || strings.ContainsAny(ascii, "/:?#@\\") {
		return "", invalid("域名无效")
	}
	if _, e := netip.ParseAddr(ascii); e == nil {
		return "", invalid("域名绑定不能使用 IP 地址")
	}
	for _, label := range strings.Split(ascii, ".") {
		if !hostLabel.MatchString(label) {
			return "", invalid("域名标签无效")
		}
	}
	return ascii, nil
}
func canonicalAddress(value string) (string, error) {
	a, e := netip.ParseAddr(strings.TrimSpace(value))
	if e != nil || a.Zone() != "" {
		return "", invalid("IP 地址无效")
	}
	return a.Unmap().String(), nil
}
func normalizeRule(kind, value string) (string, error) {
	value = strings.TrimSpace(value)
	if kind == "domain" {
		wild := strings.HasPrefix(value, "*.")
		base := strings.TrimPrefix(value, "*.")
		host, e := canonicalHost(base)
		if e != nil {
			return "", e
		}
		if wild {
			suffix, icann := publicsuffix.PublicSuffix(host)
			if suffix == host && (icann || strings.Contains(host, ".")) {
				return "", invalid("不能对白名单公共后缀或托管后缀开放通配符")
			}
			return "*." + host, nil
		}
		return host, nil
	}
	if prefix, e := netip.ParsePrefix(value); e == nil {
		if prefix.Addr().Zone() != "" {
			return "", invalid("IP 网段无效")
		}
		if prefix.Addr().Is4In6() && prefix.Bits() >= 96 {
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		return prefix.Masked().String(), nil
	}
	return canonicalAddress(value)
}
func ruleMatches(rule BindingRule, actual string) bool {
	if !rule.Enabled {
		return false
	}
	if rule.Kind == "domain" {
		if !strings.HasPrefix(rule.Pattern, "*.") {
			return rule.Pattern == actual
		}
		base := strings.TrimPrefix(rule.Pattern, "*.")
		if actual == base {
			return rule.IncludeApex
		}
		if !strings.HasSuffix(actual, "."+base) {
			return false
		}
		prefix := strings.TrimSuffix(actual, "."+base)
		return prefix != "" && (rule.SubdomainDepth == "all" || !strings.Contains(prefix, "."))
	}
	prefix, e := netip.ParsePrefix(rule.Pattern)
	address, e2 := netip.ParseAddr(actual)
	if e2 != nil {
		return false
	}
	if e == nil {
		return prefix.Contains(address.Unmap())
	}
	return rule.Pattern == actual
}

type BindingInput struct {
	Type    string   `json:"type"`
	Value   string   `json:"value"`
	Aliases []string `json:"aliases,omitempty"`
	Source  string   `json:"source,omitempty"`
}
type normalizedBinding struct {
	Kind, Value, GroupValue, Hash, ClientHash, Display string
	Aliases                                            []string
}

func (s *Service) normalizeBinding(app string, input BindingInput, maxAliases int) (normalizedBinding, error) {
	var b normalizedBinding
	b.Kind = input.Type
	value := strings.TrimSpace(input.Value)
	if maxAliases < 1 {
		maxAliases = 2
	}
	switch input.Type {
	case "seat":
		value = "seat"
	case "machine_id":
		value = strings.ToLower(value)
		if !fingerprintPattern.MatchString(value) {
			return b, invalid("机器绑定需要 64 位 SHA-256 指纹，不接收原始硬件序列号")
		}
	case "domain":
		if value == "" && len(input.Aliases) > 0 {
			value = input.Aliases[0]
		}
		host, e := canonicalHost(value)
		if e != nil {
			return b, e
		}
		value = host
		aliases := input.Aliases
		if len(aliases) == 0 {
			aliases = []string{value}
		}
		if len(aliases) > maxAliases {
			return b, forbidden("DOMAIN_ALIAS_LIMIT_EXCEEDED", "域名别名数量超过套餐额度")
		}
		seen := map[string]bool{}
		for _, alias := range aliases {
			host, e := canonicalHost(alias)
			if e != nil {
				return b, e
			}
			if seen[host] {
				return b, invalid("域名别名重复")
			}
			seen[host] = true
			b.Aliases = append(b.Aliases, host)
		}
		if !seen[value] {
			return b, invalid("实际域名必须位于登记的别名集合中")
		}
		sort.Strings(b.Aliases)
	case "ip":
		address, e := canonicalAddress(value)
		if e != nil {
			return b, e
		}
		value = address
	case "installation", "cluster", "account", "tenant":
		if value == "" || len(value) > 160 || strings.ContainsAny(value, "\x00\r\n") {
			return b, invalid("稳定业务标识无效")
		}
	default:
		return b, invalid("绑定类型不受支持")
	}
	b.Value = value
	b.GroupValue = value
	if len(b.Aliases) > 0 {
		raw, _ := json.Marshal(b.Aliases)
		b.GroupValue = string(raw)
	}
	b.Hash = s.hash("binding/"+b.Kind, app, value)
	sum := sha256.Sum256([]byte(b.Kind + "\x00" + value))
	b.ClientHash = hex.EncodeToString(sum[:])
	b.Display = maskAnchor(b.Kind, value)
	return b, nil
}
func maskAnchor(kind, value string) string {
	if kind == "domain" || kind == "ip" {
		return "已登记" + kind + " · " + digestShort(value)
	}
	if len(value) < 8 {
		return "****"
	}
	return value[:4] + "…" + value[len(value)-4:]
}
func digestShort(value string) string {
	v := sha256.Sum256([]byte(value))
	return hex.EncodeToString(v[:4])
}
func (s *Service) checkApplicationRules(app Application, rules []BindingRule, b normalizedBinding, licenseID string) error {
	mode := app.VerifyPolicy.DomainRuleMode
	if b.Kind == "ip" {
		mode = app.VerifyPolicy.IPRuleMode
	}
	if b.Kind != "domain" && b.Kind != "ip" {
		return nil
	}
	if mode != "application_allowlist" {
		return nil
	}
	for _, rule := range rules {
		if rule.Kind == b.Kind && (rule.LicenseID == "" || rule.LicenseID == licenseID) && ruleMatches(rule, b.Value) {
			return nil
		}
	}
	return forbidden("BINDING_NOT_ALLOWLISTED", "当前域名或 IP 不在应用白名单中")
}
