package buildinfo

import (
	"os"
	"path/filepath"
	"strings"
)

// Version is the release version. Release builds replace it via -ldflags; the
// fallback reads the repo-root VERSION file, the single place where the
// version is maintained for local runs.
var Version = "dev"

var (
	Commit    = "development"
	BuildTime = "unknown"
)

func init() {
	if Version != "dev" {
		return
	}
	for _, candidate := range []string{"VERSION", filepath.Join("..", "VERSION")} {
		if raw, err := os.ReadFile(candidate); err == nil {
			if v := strings.TrimSpace(string(raw)); v != "" {
				Version = v
				return
			}
		}
	}
}

type Release struct {
	Version    string   `json:"version"`
	Date       string   `json:"date"`
	Title      string   `json:"title"`
	Highlights []string `json:"highlights"`
}

type UpgradeCapabilities struct {
	OnlineUpgrade     bool `json:"online_upgrade"`
	SignedPackages    bool `json:"signed_packages"`
	AutomaticRollback bool `json:"automatic_rollback"`
}

type Info struct {
	SchemaVersion  int                 `json:"schema_version"`
	Product        string              `json:"product"`
	CurrentVersion string              `json:"current_version"`
	Channel        string              `json:"channel"`
	Commit         string              `json:"commit"`
	BuildTime      string              `json:"build_time"`
	Capabilities   UpgradeCapabilities `json:"capabilities"`
	Releases       []Release           `json:"releases"`
}

func Current() Info {
	return Info{
		SchemaVersion:  1,
		Product:        "Yunnuo License",
		CurrentVersion: Version,
		Channel:        "stable",
		Commit:         Commit,
		BuildTime:      BuildTime,
		Capabilities:   UpgradeCapabilities{},
		Releases: []Release{
			{
				Version: "0.3.0",
				Date:    "2026-10-25",
				Title:   "控制台导航与表单体验重构",
				Highlights: []string{
					"侧边导航按业务分组、可折叠，业务概览提为一级入口",
					"卡密、授权、风控、审计、产品升级与套餐结算拆分为标签页",
					"新增与编辑表单统一为弹窗交互，产品与代理支持信息编辑",
					"顶栏改为头像下拉菜单，修改密码与退出登录集中收纳",
					"业务概览重构为数据看板，统一控件圆角、焦点态与表单分区",
				},
			},
			{
				Version: "0.2.0",
				Date:    "2026-08-19",
				Title:   "Vue 管理体验重构",
				Highlights: []string{
					"公共查询、管理后台和代理工作台迁移至 Vue 3 与 TypeScript",
					"公共授权查询支持域名、IP、QQ、手机号和业务账号",
					"重构桌面与移动端布局，统一使用 Lucide 图标",
					"增加前端生产构建、单镜像部署和完整 CI 验证",
					"建立系统版本接口与结构化更新记录",
				},
			},
			{
				Version: "0.1.0",
				Date:    "2026-08-18",
				Title:   "首个开源预览版本",
				Highlights: []string{
					"提供产品、卡密、授权、代理、风控和审计核心能力",
					"支持在线验证、离线授权与 Go、Java、Node.js SDK",
					"支持内置 MySQL 的单 Docker 镜像部署",
				},
			},
		},
	}
}
