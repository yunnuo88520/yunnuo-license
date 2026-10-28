package licensing

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

//go:embed templates/*
var integrationAssets embed.FS

type IntegrationFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}
type IntegrationOptions struct{ PlanID, BindingType, Form, BaseURL string }
type IntegrationKit struct {
	PlanID         string            `json:"plan_id"`
	DefaultBinding string            `json:"default_binding"`
	DefaultForm    string            `json:"default_form"`
	BaseURL        string            `json:"base_url"`
	ArchiveBase64  string            `json:"archive_base64"`
	Language       string            `json:"language"`
	AppID          string            `json:"app_id"`
	InstallCommand string            `json:"install_command"`
	Snippets       map[string]string `json:"snippets"`
	Files          []IntegrationFile `json:"files"`
	OpenAPI        string            `json:"openapi"`
}

func (s *Service) Integration(ctx context.Context, appID, language string, options ...IntegrationOptions) (IntegrationKit, error) {
	scoped, app, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return IntegrationKit{}, e
	}
	specs := map[string][]string{"go": {"main.go.txt", "go.mod.txt", "updates.go.txt"}, "java": {"LicenseClient.java", "pom.xml"}, "python": {"client.py", "requirements.txt", "gui.py", "update.py", "web.py"}, "rust": {"main.rs", "Cargo.toml"}, "typescript": {"client.ts", "package.json"}, "http": {}}
	names, ok := specs[language]
	if !ok {
		return IntegrationKit{}, invalid("请选择 Go / Java / Python / Rust / TypeScript / HTTP")
	}
	kit := IntegrationKit{Language: language, AppID: appID, Snippets: map[string]string{}, Files: []IntegrationFile{}}
	var key SigningKey
	e = repo.View(scoped, func(v View) error { var e error; key, e = v.Keys().Get(scoped, app.CurrentKID); return e })
	if e != nil {
		return kit, e
	}
	opt := IntegrationOptions{}
	if len(options) > 0 {
		opt = options[0]
	}
	baseURL := "http://127.0.0.1:8080"
	if opt.BaseURL != "" {
		u, err := url.Parse(opt.BaseURL)
		if err != nil || u.User != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
			return kit, invalid("接入地址需要 HTTP(S) 站点根地址")
		}
		host := u.Hostname()
		if _, err := netip.ParseAddr(host); err != nil {
			if _, err = canonicalHost(host); err != nil {
				return kit, invalid("接入地址主机名无效")
			}
		}
		baseURL = strings.TrimRight(u.String(), "/")
	}
	selectedBindings, selectedForms := app.AllowedBindings, app.Forms
	var selectedPlan Plan
	e = repo.View(scoped, func(v View) error {
		plans, e := v.Plans().Find(scoped, Filter{Limit: 1000})
		if e != nil {
			return e
		}
		for _, p := range plans {
			if opt.PlanID != "" && p.ID == opt.PlanID || opt.PlanID == "" && selectedPlan.ID == "" && p.Enabled {
				selectedPlan = p
			}
		}
		if opt.PlanID != "" && selectedPlan.ID == "" {
			return notFound()
		}
		return nil
	})
	if e != nil {
		return kit, e
	}
	if selectedPlan.ID != "" {
		selectedBindings = selectedPlan.Terms.BindingTypes
		selectedForms = selectedPlan.Terms.Forms
	}
	if len(selectedBindings) == 0 || len(selectedForms) == 0 {
		return kit, invalid("请先配置应用或套餐的绑定与形态")
	}
	defaultBinding, defaultForm := selectedBindings[0], selectedForms[0]
	if opt.BindingType != "" {
		if !contains(selectedBindings, opt.BindingType) {
			return kit, invalid("此套餐不支持选择的绑定")
		}
		defaultBinding = opt.BindingType
	}
	if opt.Form != "" {
		if !contains(selectedForms, opt.Form) {
			return kit, invalid("此套餐不支持选择的产品形态")
		}
		defaultForm = opt.Form
	}
	kit.PlanID = selectedPlan.ID
	kit.DefaultBinding = defaultBinding
	kit.DefaultForm = defaultForm
	kit.BaseURL = baseURL
	for _, name := range names {
		raw, e := integrationAssets.ReadFile("templates/" + name)
		if e != nil {
			return kit, e
		}
		kit.Files = append(kit.Files, IntegrationFile{integrationFilename(name), strings.NewReplacer("@@APP_ID@@", appID, "@@DEFAULT_BINDING@@", defaultBinding, "@@DEFAULT_FORM@@", defaultForm, "@@LICENSE_URL@@", baseURL).Replace(string(raw))})
	}
	commands := map[string]string{"go": "go mod tidy\ngo run . activate\ngo run . verify\ngo run . local", "java": "mvn compile\nmvn exec:java -Dexec.args=activate\nmvn exec:java -Dexec.args=verify\nmvn exec:java -Dexec.args=local", "python": "python -m pip install -r requirements.txt\npython client.py activate\npython client.py verify\npython client.py local", "rust": "cargo build --release\ncargo run --release -- activate\ncargo run --release -- verify\ncargo run --release -- local", "typescript": "npm install\nnode client.ts activate\nnode client.ts verify\nnode client.ts local", "http": "使用任意语言的 HTTP 客户端、Ed25519 签名库与标准 RS256 JWT 验证库。无须安装本平台 SDK。"}
	kit.InstallCommand = commands[language]
	overview := fmt.Sprintf(`应用：%s
公开 AppID：%s

1. 给产品填写一次 AppID 与授权站 HTTPS 地址，不为每位客户重新编译。
2. 选择与套餐一致的 YN_BINDING_TYPE、YN_BINDING_VALUE、YN_FORM。
3. 客户输入卡密 YN_CARD，或填写一次性安装邀请信息。程序生成自己的 Ed25519 安装密钥。
4. activate 首次激活；verify 按应用策略在线刷新；local 只进行本地 JWT 校验。
5. 把实际业务动作接到本地验签 + 定期在线校验的结果；consume 动作每次必须在线、使用唯一业务幂等键。
6. 把安装密钥、JWT、可信 JWKS 缓存保存在专属持久目录，不记录卡密原文，不复制多台机器共用私钥。

允许模型：%s
允许绑定：%s
允许形态：%s
JWT TTL：%d 秒；刷新周期：%d 秒；断网宽限：%d 秒；在线租约：%d 秒。
永久使用不等于永久更新；所有更新检查、下载、安装授权需要在线且仍有更新资格。

发行包：只包含 AppID、公钥来源与通用程序。
AppSecret：只给开发者自己控制的可信服务端，不要进入客户发行包。
凭证遗失：凭原卡密恢复（受绑定与额度限制），或在页面生成一次性安装邀请；不要求客户接触数据库。
`, app.Name, appID, strings.Join(app.AllowedModels, ", "), strings.Join(app.AllowedBindings, ", "), strings.Join(app.Forms, ", "), app.VerifyPolicy.JWTTTLSeconds, app.VerifyPolicy.VerifyIntervalSeconds, app.VerifyPolicy.NetworkGraceSeconds, app.VerifyPolicy.SeatLeaseSeconds)
	deployment := `多种产品形态共用同一 AppID、同一卡密规则，不同套餐可以限制可用形态。

Web / B/S：从你配置的真实站点地址取得域名，不信任任意 Host；主域名与 www 需要显式登记为同组别名。每份安装独立签名。签名不能证明物理域名控制权，首次关联仍需要卡密、安装邀请或管理员批准。
域名 / IP：服务端每次 verify 重新匹配许可证绑定和启用的规则。IP 默认匹配可信请求出口地址；NAT / 私网场景必须明确选择已批准安装声明模式。
GUI / 裸机 / IoT：用真实硬件标识的 SHA-256；Windows/macOS 请提供平台对应的硬件提供器。无法获取真实锚点时明确报错，不用随机值伪装硬件指纹。
Docker / Kubernetes：禁止默认读取容器 MAC、hostname、machine-id。短生命周期容器使用 floating + seat；宿主机绑定需要只读挂载宿主 SHA-256 指纹文件，并为每份安装挂载独立持久卷。
硬性要求：不挂载持久卷会丢失安装私钥，重建会成为新安装，可能再次消耗激活额度。不要把私钥烘焙进镜像，不要让多个副本写同一身份目录。
CLI / 服务 / 插件 / 移动端 / SaaS：按业务选择机器、稳定安装、账号、租户或集群锚点。账号/租户声明由产品自己的身份系统验证，不把用户任意输入当成可信账号。
浮动：心跳 verify 续租，崩溃后 lease_expires_at 到期自动归还；离线借用在借用到期前持续占位，不能提前释放后重复签出。
按量：consume + units + idempotency_key，额度由授权站事务扣除。业务系统也应以同一业务键去重，不能靠本地 JWT 的静态额度执行付费动作。

断网：标准 JWT 始终检查 exp。仅在真实连接/超时故障时使用单独签发的 offline_jwt（token_use=offline_grace），仍检查签名、exp、aud、安装与绑定。没有离线凭证、超过宽限、系统时钟回退都不得放行。
明确拒绝：revoked/suspended/expired、绑定不符、席位超限、签名错误等，清除 JWT 缓存并停止受控功能，禁止进入离线回退。HTTP 4xx/5xx 明确响应也采用此保守策略。
离线边界：服务端吊销不能即时收回已离线签发的 JWT。短 TTL 和宽限仅收敛窗口；客户端逻辑被破解属于代码完整性保护边界。
公钥：配置可信 HTTPS JWKS 地址；不读取 JWT 中的 jku/x5u 地址。缓存可信 JWKS 支持断网；遇到新 kid 重新拉取。高安全场景配合更新签名或操作系统安全存储保护信任根。

Docker 客户产品示例（把镜像替换为你自己的同一发行镜像）：
services:
  product:
    image: your-product:1.0.0
    environment:
      YN_LICENSE_URL: https://license.example.com
      YN_BINDING_TYPE: seat
      YN_FORM: container
      YN_STATE_DIR: /var/lib/product/license
    volumes:
      - product-license:/var/lib/product/license
volumes:
  product-license:

安装私钥与可信状态属于敏感文件，Unix 权限 0600、目录 0700；Windows 使用应用服务账号的 ACL。备份应加密且不能被多份运行实例同时复用。
`
	contract := fmt.Sprintf(`POST /v2/activate
Content-Type: application/json
X-YN-Timestamp: Unix 秒数
X-YN-Nonce: 至少 16 字节随机数的 base64url（无 =）
X-YN-Signature: Ed25519(下面签名串) 的 base64url（无 =）

签名串（换行连接，无末尾换行）：
YN-AUTH-V2
POST
/v2/activate
%s
<timestamp>
<nonce>
<sha256(实际发送的原始 JSON 字节，小写十六进制)>

请求体：
{"app_id":"%s","card_code":"<客户卡密>","public_key":"<安装 Ed25519 原始 32 字节公钥的 base64url>","binding":{"type":"domain","value":"example.com","aliases":["example.com","www.example.com"]},"form":"web"}

免卡密：用 invitation_id + invitation_token 替代 card_code；或先 POST /v2/installations/request 提交安装公钥及绑定，页面批准后使用 pending_id。三种凭证只选一种。
浮动：binding.type=seat，form=container；可用 borrow_seconds 请求有限离线借用。
响应为 {success:true,data:{license_id,activation_id,jwt,offline_jwt,expires_at,refresh_after,grace_until,lease_expires_at,usage_expires_at,updates_until,server_time}}。
失败返回稳定 error.code，不返回“授权成功”的降级标志。卡密不进入请求日志、浏览器持久存储或发行包。
`, appID, appID)
	verify := fmt.Sprintf(`POST /v2/verify
使用同一安装私钥签名，签名串中的路径改为 /v2/verify，每次生成新 nonce。
{"app_id":"%s","activation_id":"<activate 返回的安装 ID>","binding":{"type":"domain","value":"example.com"},"form":"web"}

jwt 字段可附带旧 license JWT 作为关联提示，服务端仍以安装签名与数据库状态为准。JWT 过期或旧 kid 退休后，可省略 jwt，通过安装凭证恢复，不跳过本地 exp 验证。
action 默认 verify；release 归还在线租约；consume 需要 units 与 idempotency_key；renew 需要 renewal_card。

客户端分支：
LICENSE_REVOKED / LICENSE_SUSPENDED / LICENSE_EXPIRED → 清除缓存、停止受控功能。
SEAT_EXCEEDED / ACTIVATION_LIMIT_EXCEEDED → 不启用核心功能，提示释放旧安装或增购。
BINDING_MISMATCH / BINDING_NOT_ALLOWLISTED / SOURCE_IP_MISMATCH → 核对真实站点或申请换绑。
REQUEST_REPLAYED → 使用新 nonce 重试，不复用旧的签名请求。
请求连接失败或超时 → 只有独立 offline_jwt 严格验签通过且未超过自身 exp 时才允许离线使用。
每次验证服务端重新检查 status、期限、实时规则与租约；JWT 不携带 used_seats。
`, appID)
	updatesSnippet := `可选升级：先在应用策略启用，在在线升级页上传并发布版本。
POST /v2/updates/check → POST /v2/updates/{release_id}/download → POST /v2/updates/{release_id}/authorize
三个请求均由当前安装私钥签名，使用全新 nonce，携带 app_id、activation_id、binding、form、current_version 和 target（os/arch/runtime/channel/kind）。
检查结果包含 manifest_jwt，下载头 X-YN-Update-Manifest 也是 RS256 JWT，严格验证 iss/aud/exp、token_use=update_manifest、安装、绑定和 release 的目标、版本、大小、SHA-256。
下载后核对实际包哈希；修改文件和数据库前立即在线 authorize，再执行产品自己的备份、迁移、原子切换、健康检查和失败回滚。已下载不代表未来仍能安装，更新绝不使用离线宽限。
Go 项目使用 update-check / update-download / update-authorize 命令；设置 YN_CURRENT_VERSION 或通过产品构建注入版本。Python 项目包含可运行 update.py，依次执行 check / download / authorize；YN_CURRENT_VERSION 指定本产品当前版本，YN_UPDATE_OS/ARCH/RUNTIME/CHANNEL/KIND 指定目标。
容器归档还需核对不可变镜像 ID；GUI 安装器需处理系统签名、进程退出和可执行文件替换，不能通用覆盖未知文件。
`
	salesSnippet := `在销售与卡密页配置独立 HMAC 渠道，回调 POST /v2/sales/{app_id}/{channel_id}。
X-YN-Timestamp: Unix 秒数（两分钟窗口）；X-YN-Signature: hex(HMAC-SHA256(channel_secret, timestamp + "\n" + 原始 JSON 请求体))。
支付事件：{"event_id":"唯一销售事件号","event":"payment.confirmed","plan_id":"当前应用套餐","customer_ref":"客户订单号","quantity":1,"amount_cents":10000,"distribute_cards":true}
退款事件：{"event_id":"唯一退款事件号","event":"payment.refunded","original_event_id":"原支付事件号"}
仅已授予渠道的上架套餐可售，金额须满足套餐定价；同事件重试不重复签发，不重复返回卡密原文。
可靠发货：商城先在自己的受控订单系统保存随机卡密，再随事件发送 card_sha256 数组：SHA256(去除首尾空白后转大写的卡密，保留内部字符及横线)。授权站只保存摘要，商城重试后仍可交付自己保存的原文。不要把渠道密钥放进客户发行包。
这是通用回调协议；第三方电商的签名由对应适配器先验证，人工收款仍需管理员明确确认。
`
	publicSnippet := fmt.Sprintf("GET /v2/applications/%s/.well-known/jwks.json\n算法 RS256；kid 按应用独立。轮换后新 kid 自动出现在 JWKS，旧 kid 在重叠期内保留。\n可选静态公钥（需要通过受信任更新刷新）：\n%s", appID, key.PublicPEM)
	localSnippet := `标准 JWT 验证顺序：固定 RS256 → 从受信任 JWKS 按 kid 取 RSA 公钥 → 签名 → iss=urn:yunnuo:licensing:v2 → aud=当前 AppID → exp/nbf/iat → token_use=license → activation_id=本安装 → binding.type 与 binding.hash=本地实际锚点。
binding.hash = SHA-256(binding_type + NUL + 规范化绑定值)，小写十六进制。seat 的绑定值是安装原始 Ed25519 公钥的 SHA-256，不是容器 hostname。
域名使用规范化 ASCII/Punycode 主机名；IP 使用规范化 IPv4 / IPv6；硬件只发送 SHA-256 指纹。
离线凭证采用 token_use=offline_grace 并检查其自身 exp；不为已过期的 license JWT 关闭 exp 验证。
本地验签不会请求数据库，不代表服务端未吊销；按 refresh_after 在线验证。`
	if len(kit.Files) > 0 {
		source := kit.Files[0].Content
		markers := map[string][]string{"go": {"func loadPublicPEM(", "func send(", "func localVerify("}, "python": {"def jwks(", "def send(", "def local_verify("}, "java": {"static PublicKey loadPublicPEM(", "static ObjectNode send(", "static JWTClaimsSet localVerify("}, "rust": {"fn keys(", "fn send(", "fn local_verify("}, "typescript": {"async function keys(", "async function send(", "async function localVerify("}}
		m := markers[language]
		publicSnippet += "\n\n" + extractFunction(source, m[0], language)
		contract += "\n\n" + extractFunction(source, m[1], language)
		verify += "\n\n" + extractFunction(source, m[1], language)
		localSnippet += "\n\n" + extractFunction(source, m[2], language)
	}
	overview = fmt.Sprintf("本次示例：套餐 %s；绑定 %s；形态 %s；授权站 %s。\n域名、IP、账号或租户等值由客户安装时填写；安装标识与席位类型由程序自动处理。\n\n", selectedPlan.Name, defaultBinding, defaultForm, baseURL) + overview
	if language == "python" {
		deployment += "\n真实 B/S 示例：设置 YN_PRODUCT_ADMIN_PASSWORD（产品自己的私有管理密码）、YN_PRODUCT_DIR（客户数据目录），运行 python web.py --bind 127.0.0.1 --port 8088；HTTPS 代理后访问 /activate，用 owner 和所设密码输入卡密。每个受保护页面与 API 先验证授权，失败返回配置公告和 HTTP 403。/healthz 用于升级健康检查。不要将这个密码设为开发者 AppSecret。\n原生 GUI 示例：python gui.py，图形界面激活、刷新和恢复。\n完整性报告：设置 YN_EVIDENCE_SHA256、YN_INTEGRITY_SUMMARY 后 python client.py report；只提交 SHA-256 摘要，待管理员人工核实，不自动封禁。客户可在用户中心提交申诉。\n"
	}
	kit.Snippets = map[string]string{"overview": overview, "public_key": publicSnippet, "activate": contract, "verify": verify, "local_verify": localSnippet, "updates": updatesSnippet, "sales": salesSnippet, "deployment": deployment}
	kit.OpenAPI = licensingOpenAPI(appID)
	kit.Files = append(kit.Files, IntegrationFile{"public.pem", key.PublicPEM}, IntegrationFile{"config.env.example", fmt.Sprintf("YN_LICENSE_URL=%s\nYN_BINDING_TYPE=%s\nYN_FORM=%s\nYN_STATE_DIR=./license-state\n# Set actual customer domain / IP / account / tenant when required:\n# YN_BINDING_VALUE=\n# Customer inputs card at activation; do not bake it into a release:\nYN_CARD=\n# Public AppID: %s\n# Never add developer AppSecret here.\n", baseURL, defaultBinding, defaultForm, appID)}, IntegrationFile{"README.md", overview + "\n运行命令：\n" + kit.InstallCommand + "\n\n" + contract + "\n\n" + verify}, IntegrationFile{"DEPLOYMENT.md", deployment}, IntegrationFile{"UPDATES.md", updatesSnippet}, IntegrationFile{"SALES.md", salesSnippet}, IntegrationFile{"openapi.json", kit.OpenAPI})
	archive, e := IntegrationZIP(kit)
	if e != nil {
		return kit, e
	}
	kit.ArchiveBase64 = base64.StdEncoding.EncodeToString(archive)
	return kit, nil
}
func extractFunction(source, marker, language string) string {
	start := strings.Index(source, marker)
	if start < 0 {
		return "请查看下方完整示例文件。"
	}
	if language == "python" {
		end := strings.Index(source[start+len(marker):], "\ndef ")
		if end < 0 {
			return source[start:]
		}
		return source[start : start+len(marker)+end]
	}
	brace := strings.Index(source[start:], "{")
	if brace < 0 {
		return ""
	}
	depth := 0
	for i := start + brace; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[start : i+1]
			}
		}
	}
	return source[start:]
}
func IntegrationZIP(kit IntegrationKit) ([]byte, error) {
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	for _, file := range kit.Files {
		writer, e := archive.Create(file.Name)
		if e != nil {
			return nil, e
		}
		if _, e = writer.Write([]byte(file.Content)); e != nil {
			return nil, e
		}
	}
	if e := archive.Close(); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}
func licensingOpenAPI(app string) string {
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"app_id", "public_key", "binding", "form"}, "properties": map[string]any{"app_id": map[string]any{"type": "string", "example": app}, "public_key": map[string]any{"type": "string", "description": "Raw Ed25519 public key, base64url without padding"}, "card_code": map[string]any{"type": "string"}, "invitation_id": map[string]any{"type": "string"}, "invitation_token": map[string]any{"type": "string"}, "pending_id": map[string]any{"type": "string"}, "binding": map[string]any{"$ref": "#/components/schemas/Binding"}, "form": map[string]any{"type": "string", "enum": []string{"web", "container", "cluster", "gui", "cli", "service", "plugin", "mobile", "iot", "saas"}}, "session_id": map[string]any{"type": "string", "maxLength": 64}, "label": map[string]any{"type": "string", "maxLength": 160}, "borrow_seconds": map[string]any{"type": "integer", "minimum": 0}}, "description": "Exactly one credential: card_code, invitation_id+invitation_token, or approved pending_id"}
	parameters := []any{map[string]any{"name": "X-YN-Timestamp", "in": "header", "required": true, "schema": map[string]any{"type": "integer"}}, map[string]any{"name": "X-YN-Nonce", "in": "header", "required": true, "schema": map[string]any{"type": "string", "minLength": 22}}, map[string]any{"name": "X-YN-Signature", "in": "header", "required": true, "schema": map[string]any{"type": "string"}}}
	responses := map[string]any{"200": map[string]any{"description": "Signed license JWT and independent offline_grace JWT, or release acknowledgement", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/RuntimeEnvelope"}}}}, "400": map[string]any{"description": "Invalid request"}, "403": map[string]any{"description": "Explicit refusal; clear cached authorization, NEVER fall back offline"}, "409": map[string]any{"description": "Nonce replay or idempotency conflict"}, "429": map[string]any{"description": "Rate limited by AppID and trusted client IP"}}
	operation := func(summary, ref string) map[string]any {
		return map[string]any{"summary": summary, "parameters": parameters, "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + ref}}}}, "responses": responses}
	}
	verifySchema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"app_id", "activation_id", "binding", "form"}, "properties": map[string]any{"app_id": map[string]any{"type": "string", "example": app}, "activation_id": map[string]any{"type": "string"}, "binding": map[string]any{"$ref": "#/components/schemas/Binding"}, "form": map[string]any{"type": "string"}, "jwt": map[string]any{"type": "string"}, "action": map[string]any{"type": "string", "enum": []string{"verify", "release", "consume", "renew"}, "default": "verify"}, "units": map[string]any{"type": "integer", "minimum": 1}, "idempotency_key": map[string]any{"type": "string", "maxLength": 128}, "renewal_card": map[string]any{"type": "string"}}}
	binding := map[string]any{"type": "object", "required": []string{"type"}, "additionalProperties": false, "properties": map[string]any{"type": map[string]any{"type": "string", "enum": []string{"machine_id", "domain", "ip", "seat", "installation", "account", "tenant", "cluster"}}, "value": map[string]any{"type": "string"}, "aliases": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 128}, "source": map[string]any{"type": "string"}}}
	result := map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": "Application Licensing V2", "version": "2.0.0", "description": "Language-neutral HTTP contract. Request proof: Ed25519 over YN-AUTH-V2 + METHOD + escaped path + app_id + Unix timestamp + nonce + SHA256(raw body), newline-separated. Client JWT: RS256, issuer urn:yunnuo:licensing:v2, audience app_id, strict exp and binding checks. AppSecret must never be distributed to customer installations."}, "paths": map[string]any{"/v2/activate": map[string]any{"post": operation("Activate by application card, invitation or approved installation", "Activate")}, "/v2/verify": map[string]any{"post": operation("Verify current server state, renew a lease, consume units or release", "Verify")}, "/v2/applications/{app_id}/.well-known/jwks.json": map[string]any{"get": map[string]any{"summary": "Per-application JWKS with rotation overlap", "parameters": []any{map[string]any{"name": "app_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}, "responses": map[string]any{"200": map[string]any{"description": "Standard public RSA JWK Set; no private components"}}}}}, "components": map[string]any{"schemas": map[string]any{"Activate": schema, "Verify": verifySchema, "Binding": binding, "RuntimeEnvelope": map[string]any{"type": "object", "required": []string{"success", "data"}, "properties": map[string]any{"success": map[string]any{"type": "boolean"}, "request_id": map[string]any{"type": "string"}, "data": map[string]any{"type": "object", "properties": map[string]any{"jwt": map[string]any{"type": "string"}, "offline_jwt": map[string]any{"type": "string"}, "activation_id": map[string]any{"type": "string"}, "license_id": map[string]any{"type": "string"}, "released": map[string]any{"type": "boolean"}}}}}}}}
	paths := result["paths"].(map[string]any)
	schemas := result["components"].(map[string]any)["schemas"].(map[string]any)
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/components/schemas/" + name} }
	text := map[string]any{"type": "string"}
	targetProperties := map[string]any{}
	for _, name := range []string{"os", "arch", "runtime", "channel", "kind", "min_version", "max_version", "image_tag", "image_digest"} {
		targetProperties[name] = text
	}
	schemas["ReleaseTarget"] = map[string]any{"type": "object", "required": []string{"os", "arch", "runtime", "channel", "kind"}, "properties": targetProperties}
	schemas["Update"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"app_id", "activation_id", "binding", "form", "current_version", "target"}, "properties": map[string]any{"app_id": text, "activation_id": text, "binding": ref("Binding"), "form": text, "current_version": map[string]any{"type": "string", "pattern": `^[0-9]+\.[0-9]+\.[0-9]+$`}, "target": ref("ReleaseTarget")}}
	schemas["UpdateEnvelope"] = map[string]any{"type": "object", "required": []string{"success", "data"}, "properties": map[string]any{"success": map[string]any{"type": "boolean"}, "request_id": text, "data": map[string]any{"type": "object", "properties": map[string]any{"available": map[string]any{"type": "boolean"}, "manifest_jwt": text, "server_time": map[string]any{"type": "string", "format": "date-time"}, "release": map[string]any{"type": "object", "description": "Immutable published release; verify its identity, target, size and SHA-256 against manifest_jwt"}}}}}
	updateOperation := func(summary string, hasID bool, binary bool) map[string]any {
		op := operation(summary, "Update")
		if hasID {
			op["parameters"] = append(append([]any{}, parameters...), map[string]any{"name": "release_id", "in": "path", "required": true, "schema": text})
		}
		response := map[string]any{"description": "Fresh online entitlement and release authorization", "content": map[string]any{"application/json": map[string]any{"schema": ref("UpdateEnvelope")}}}
		if binary {
			response = map[string]any{"description": "Authorized immutable package bytes", "headers": map[string]any{"X-YN-Update-Manifest": map[string]any{"schema": text, "description": "RS256 JWT, token_use=update_manifest"}, "X-Checksum-SHA256": map[string]any{"schema": text}}, "content": map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}}
		}
		op["responses"] = map[string]any{"200": response, "400": map[string]any{"description": "Invalid request"}, "403": map[string]any{"description": "Refused: expired/revoked/suspended, binding mismatch, lease expired or release withdrawn; never fall back to offline updating"}, "409": map[string]any{"description": "Replayed request"}, "429": map[string]any{"description": "Rate limited"}}
		return op
	}
	paths["/v2/updates/check"] = map[string]any{"post": updateOperation("Select a published update by fresh entitlement and deployment target", false, false)}
	paths["/v2/updates/{release_id}/download"] = map[string]any{"post": updateOperation("Download package with a newly signed update manifest", true, true)}
	paths["/v2/updates/{release_id}/authorize"] = map[string]any{"post": updateOperation("Recheck online immediately before applying files and database changes", true, false)}
	schemas["PendingInstallation"] = map[string]any{"type": "object", "required": []string{"app_id", "public_key", "binding"}, "properties": map[string]any{"app_id": text, "public_key": text, "binding": ref("Binding"), "label": text}}
	paths["/v2/installations/request"] = map[string]any{"post": operation("Submit an installation public key for administrator approval; this does not grant usage", "PendingInstallation")}
	schemas["IntegrityReport"] = map[string]any{"type": "object", "required": []string{"app_id", "activation_id", "signal", "evidence_sha256"}, "properties": map[string]any{"app_id": text, "activation_id": text, "signal": map[string]any{"type": "string", "enum": []string{"binary_modified", "unexpected_signature", "copied_installation"}}, "evidence_sha256": map[string]any{"type": "string", "pattern": "^[0-9a-fA-F]{64}$"}, "summary": text}}
	paths["/v2/security/report"] = map[string]any{"post": operation("Submit signed evidence for manual review; never automatically revokes a license", "IntegrityReport")}
	raw, _ := json.MarshalIndent(result, "", "  ")
	return string(raw)
}

func integrationFilename(name string) string {
	if strings.HasSuffix(name, ".go.txt") || name == "go.mod.txt" {
		return strings.TrimSuffix(name, ".txt")
	}
	return name
}
