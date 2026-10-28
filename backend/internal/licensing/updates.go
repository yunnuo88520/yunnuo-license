package licensing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/filelock"
)

const MaxPackageSize int64 = 256 << 20

var versionRE = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)
var targetRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)
var imageRE = regexp.MustCompile(`^[a-z0-9][a-z0-9./:_-]{0,220}:[A-Za-z0-9_.-]{1,100}$`)
var imageDigestRE = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func compareVersion(a, b string) int {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, _ := strconv.Atoi(aa[i])
		y, _ := strconv.Atoi(bb[i])
		if x > y {
			return 1
		}
		if x < y {
			return -1
		}
	}
	return 0
}
func normalizeTarget(t ReleaseTarget) (ReleaseTarget, error) {
	if t.OS == "" {
		t.OS = "any"
	}
	if t.Arch == "" {
		t.Arch = "any"
	}
	if t.Runtime == "" {
		t.Runtime = "any"
	}
	if t.Channel == "" {
		t.Channel = "stable"
	}
	if t.Kind == "" {
		t.Kind = "archive"
	}
	if !targetRE.MatchString(t.OS) || !targetRE.MatchString(t.Arch) || !targetRE.MatchString(t.Runtime) || !targetRE.MatchString(t.Channel) {
		return t, invalid("版本平台、架构、运行时或渠道无效")
	}
	if t.Kind != "archive" && t.Kind != "installer" && t.Kind != "container_archive" {
		return t, invalid("升级包类型无效")
	}
	if t.MinVersion != "" && !versionRE.MatchString(t.MinVersion) || t.MaxVersion != "" && !versionRE.MatchString(t.MaxVersion) || t.MinVersion != "" && t.MaxVersion != "" && compareVersion(t.MinVersion, t.MaxVersion) > 0 {
		return t, invalid("升级路径版本范围无效")
	}
	if t.Kind == "container_archive" {
		if !imageRE.MatchString(t.ImageTag) || !imageDigestRE.MatchString(t.ImageDigest) {
			return t, invalid("容器归档必须填写镜像标签与不可变 sha256 镜像 ID")
		}
	} else if t.ImageTag != "" || t.ImageDigest != "" {
		return t, invalid("普通升级包不填写容器镜像身份")
	}
	return t, nil
}
func targetKey(t ReleaseTarget) string {
	raw, _ := json.Marshal([]string{t.OS, t.Arch, t.Runtime, t.Channel, t.Kind})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type UploadInput struct {
	Version, Notes, Filename string
	Target                   ReleaseTarget
}

func (s *Service) ListReleases(ctx context.Context, appID string) ([]Release, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return nil, e
	}
	var out []Release
	e = repo.View(scoped, func(v View) error { var e error; out, e = v.Releases().Find(scoped, Filter{Limit: 10000}); return e })
	return out, e
}
func (s *Service) UploadRelease(ctx context.Context, appID string, in UploadInput, content io.Reader) (Release, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return Release{}, e
	}
	target, e := normalizeTarget(in.Target)
	if e != nil {
		return Release{}, e
	}
	filename := filepath.Base(strings.ReplaceAll(in.Filename, `\`, "/"))
	lower := strings.ToLower(filename)
	if s.releaseDir == "" || !versionRE.MatchString(in.Version) || len(in.Notes) > 16000 || len(filename) > 200 || strings.ContainsAny(filename, "\x00\r\n") || !(strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")) {
		return Release{}, invalid("版本号需要 x.y.z，升级包仅支持 zip / tar.gz / tgz")
	}
	if e = os.MkdirAll(s.releaseDir, 0700); e != nil {
		return Release{}, e
	}
	temp, e := os.CreateTemp(s.releaseDir, ".upload-v2-")
	if e != nil {
		return Release{}, e
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	hash := sha256.New()
	size, e := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(content, MaxPackageSize+1))
	if e != nil {
		return Release{}, e
	}
	if size == 0 || size > MaxPackageSize {
		return Release{}, Error{"PACKAGE_SIZE_INVALID", "升级包为空或超过 256 MiB", 413}
	}
	if e = temp.Sync(); e != nil {
		return Release{}, e
	}
	if e = temp.Close(); e != nil {
		return Release{}, e
	}
	result := Release{Record: s.record(appID, "v2rel"), Version: in.Version, Target: target, TargetKey: targetKey(target), Status: "draft", Notes: in.Notes, Filename: filename, Size: size, SHA256: hex.EncodeToString(hash.Sum(nil))}
	result.StoragePath = result.ID
	unlock, e := filelock.Acquire(ctx, s.releaseDir+".lock", false)
	if e != nil {
		return Release{}, e
	}
	defer unlock()
	path := filepath.Join(s.releaseDir, result.ID)
	e = repo.Update(scoped, func(tx Transaction) error {
		app, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		if app.Status != "active" {
			return forbidden("APPLICATION_DISABLED", "应用已停用")
		}
		capacity, e := tx.ReleaseCapacity(scoped)
		if e != nil {
			return e
		}
		if size > s.storageLimits.TotalBytes-capacity.TotalBytes || size > s.storageLimits.ApplicationBytes-capacity.ApplicationBytes || capacity.Versions >= s.storageLimits.Versions || app.DeveloperID != "" && size > s.storageLimits.DeveloperBytes-capacity.DeveloperBytes {
			return conflict("STORAGE_QUOTA_EXCEEDED", "应用、开发者或平台存储额度已用完")
		}
		releases, e := tx.Releases().Find(scoped, Filter{Limit: 10000})
		if e != nil {
			return e
		}
		for _, r := range releases {
			if r.Version == result.Version && r.TargetKey == result.TargetKey {
				return conflict("RELEASE_EXISTS", "同一平台、渠道与版本不可覆盖，请发布新版本")
			}
		}
		if e = os.Rename(temp.Name(), path); e != nil {
			return e
		}
		if e = filelock.SyncDirectory(s.releaseDir); e != nil {
			return e
		}
		if e = tx.Releases().Insert(scoped, result); e != nil {
			return e
		}
		return s.append(scoped, tx, "release_uploaded", result.ID, map[string]any{"version": result.Version, "target": target, "size": size, "sha256": result.SHA256})
	})
	if e != nil {
		var exists bool
		lookupErr := repo.View(scoped, func(v View) error { _, e := v.Releases().Get(scoped, result.ID); exists = e == nil; return e })
		if !exists && lookupErr == ErrNotFound {
			_ = os.Remove(path)
		}
		return Release{}, e
	}
	return result, nil
}
func (s *Service) SetReleaseStatus(ctx context.Context, appID, releaseID, status, reason string) (Release, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return Release{}, e
	}
	if status != "published" && status != "withdrawn" || strings.TrimSpace(reason) == "" {
		return Release{}, invalid("请选择发布/撤回并填写原因")
	}
	var result Release
	e = repo.Update(scoped, func(tx Transaction) error {
		var e error
		result, e = tx.Releases().Get(scoped, releaseID)
		if e != nil {
			return notFound()
		}
		if result.Status == "deleted" {
			return notFound()
		}
		if status == "published" {
			file, e := s.openPackage(result)
			if e != nil {
				return e
			}
			file.Close()
		}
		result.Status = status
		result.UpdatedAt = s.now()
		if e = tx.Releases().Save(scoped, result); e != nil {
			return e
		}
		return s.append(scoped, tx, "release_"+status, result.ID, map[string]any{"version": result.Version, "reason": reason})
	})
	return result, e
}
func (s *Service) openPackage(r Release) (*os.File, error) {
	path := filepath.Join(s.releaseDir, r.ID)
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() {
		return nil, forbidden("PACKAGE_UNAVAILABLE", "升级包不存在或不是普通文件")
	}
	file, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	hash := sha256.New()
	size, e := io.Copy(hash, io.LimitReader(file, MaxPackageSize+1))
	if e != nil || size != r.Size || hex.EncodeToString(hash.Sum(nil)) != r.SHA256 {
		file.Close()
		return nil, forbidden("PACKAGE_INTEGRITY_FAILED", "升级包完整性验证失败")
	}
	if _, e = file.Seek(0, io.SeekStart); e != nil {
		file.Close()
		return nil, e
	}
	return file, nil
}

type ReleaseManifest struct {
	ID       string        `json:"id"`
	Version  string        `json:"version"`
	Target   ReleaseTarget `json:"target"`
	SHA256   string        `json:"sha256"`
	Size     int64         `json:"size"`
	Filename string        `json:"filename"`
}
type UpdateInput struct {
	AppID          string        `json:"app_id"`
	ActivationID   string        `json:"activation_id"`
	Binding        BindingInput  `json:"binding"`
	Form           string        `json:"form"`
	CurrentVersion string        `json:"current_version"`
	Target         ReleaseTarget `json:"target"`
}
type UpdateResult struct {
	Available   bool      `json:"available"`
	Release     *Release  `json:"release,omitempty"`
	ManifestJWT string    `json:"manifest_jwt,omitempty"`
	ServerTime  time.Time `json:"server_time"`
}

func releaseMatches(r Release, in UpdateInput) bool {
	t := r.Target
	for _, p := range [][2]string{{t.OS, in.Target.OS}, {t.Arch, in.Target.Arch}, {t.Runtime, in.Target.Runtime}} {
		if p[0] != "any" && p[0] != p[1] {
			return false
		}
	}
	channel := in.Target.Channel
	if channel == "" {
		channel = "stable"
	}
	if channel != t.Channel || in.Target.Kind != "" && t.Kind != in.Target.Kind {
		return false
	}
	if t.MinVersion != "" && compareVersion(in.CurrentVersion, t.MinVersion) < 0 || t.MaxVersion != "" && compareVersion(in.CurrentVersion, t.MaxVersion) > 0 {
		return false
	}
	return true
}
func (s *Service) updateContext(ctx context.Context, in UpdateInput, req SignedRequest) (context.Context, Repository, error) {
	lookup, _, repo, e := s.runtimeContext(ctx, in.AppID, Actor{Kind: "installation_lookup", ID: in.ActivationID})
	if e != nil {
		return ctx, nil, e
	}
	var activation Activation
	e = repo.View(lookup, func(v View) error {
		var e error
		activation, e = v.Activations().Get(lookup, in.ActivationID)
		return e
	})
	if e != nil {
		return ctx, nil, forbidden("ACTIVATION_INVALID", "安装激活不存在")
	}
	scoped, _, repo, e := s.authenticateInstallation(ctx, in.AppID, activation.PublicKey, req)
	return scoped, repo, e
}
func (s *Service) checkUpdateLicense(ctx context.Context, tx Transaction, in UpdateInput, req SignedRequest) (Application, License, Activation, error) {
	app, e := tx.Application(ctx)
	if e != nil {
		return app, License{}, Activation{}, e
	}
	a, e := tx.Activations().Get(ctx, in.ActivationID)
	if e != nil {
		return app, License{}, a, e
	}
	l, e := tx.Licenses().Get(ctx, a.LicenseID)
	if e != nil {
		return app, l, a, e
	}
	now := s.now()
	if e = checkUsable(app, l, now); e != nil {
		return app, l, a, e
	}
	if l.UpdatesBlocked {
		return app, l, a, forbidden("UPDATES_BLOCKED", "更新已被人工限制，请联系软件提供方核实")
	}
	if !app.UpdatesEnabled {
		return app, l, a, forbidden("UPDATES_DISABLED", "此应用未启用更新服务")
	}
	if l.ActivatedAt == nil || l.UpdatesUntil == nil || !now.Before(*l.UpdatesUntil) || !l.Terms.Perpetual && (l.ExpiresAt == nil || !now.Before(*l.ExpiresAt)) {
		return app, l, a, forbidden("UPDATES_EXPIRED", "更新资格已到期；使用宽限不延长更新资格")
	}
	if a.DeactivatedAt != nil {
		return app, l, a, forbidden("ACTIVATION_DEACTIVATED", "此安装已停用")
	}
	if a.LeaseExpiresAt != nil && !now.Before(*a.LeaseExpiresAt) {
		return app, l, a, forbidden("SEAT_LEASE_EXPIRED", "浮动席位租约已到期，请先在线验证重新签出")
	}
	if !contains(app.Forms, in.Form) || !contains(l.Terms.Forms, in.Form) || !contains(app.AllowedBindings, in.Binding.Type) || a.BindingType != in.Binding.Type {
		return app, l, a, forbidden("BINDING_MISMATCH", "产品形态或绑定不匹配")
	}
	b, e := s.runtimeBinding(app, in.Binding, l.Terms.MaxDomainAliases, req, a.PublicKeyHash)
	if e != nil {
		return app, l, a, e
	}
	rules, e := tx.Rules().Find(ctx, Filter{Limit: 10000})
	if e != nil {
		return app, l, a, e
	}
	if e = s.checkApplicationRules(app, rules, b, l.ID); e != nil {
		return app, l, a, e
	}
	allowed := false
	if strings.HasPrefix(a.BindingGroupID, "rule_") {
		for _, rule := range rules {
			if rule.ID == a.BindingGroupID && rule.LicenseID == l.ID && rule.Kind == b.Kind && ruleMatches(rule, b.Value) {
				allowed = true
			}
		}
	} else if b.Kind == "domain" {
		anchors, e := tx.Anchors().Find(ctx, Filter{LicenseID: l.ID, GroupID: a.BindingGroupID, BindingHash: b.Hash, Status: "active", Limit: 1})
		if e != nil {
			return app, l, a, e
		}
		allowed = len(anchors) == 1
	} else {
		allowed = a.BindingHash == b.Hash
	}
	if !allowed {
		return app, l, a, forbidden("BINDING_MISMATCH", "更新来源不符合安装的实时绑定")
	}
	a.ClientBindingHash = b.ClientHash
	return app, l, a, nil
}
func (s *Service) authorizeUpdate(ctx context.Context, in UpdateInput, releaseID string, req SignedRequest, download bool) (UpdateResult, *os.File, error) {
	if !versionRE.MatchString(in.CurrentVersion) {
		return UpdateResult{}, nil, invalid("请提供当前 x.y.z 版本")
	}
	scoped, repo, e := s.updateContext(ctx, in, req)
	if e != nil {
		return UpdateResult{}, nil, e
	}
	out := UpdateResult{ServerTime: s.now()}
	var opened *os.File
	e = repo.Update(scoped, func(tx Transaction) error {
		app, l, a, e := s.checkUpdateLicense(scoped, tx, in, req)
		if e != nil {
			return e
		}
		releases, e := tx.Releases().Find(scoped, Filter{Status: "published", Limit: 10000})
		if e != nil {
			return e
		}
		var chosen *Release
		score := -1
		for _, r := range releases {
			if releaseID != "" && r.ID != releaseID || compareVersion(r.Version, in.CurrentVersion) <= 0 || !releaseMatches(r, in) {
				continue
			}
			specific := 0
			for _, v := range []string{r.Target.OS, r.Target.Arch, r.Target.Runtime} {
				if v != "any" {
					specific++
				}
			}
			if chosen == nil || compareVersion(r.Version, chosen.Version) > 0 || r.Version == chosen.Version && specific > score {
				copy := r
				chosen = &copy
				score = specific
			}
		}
		if chosen == nil {
			if releaseID != "" {
				return forbidden("RELEASE_UNAVAILABLE", "版本已撤回、不适配当前环境或不符合升级路径")
			}
			return s.append(scoped, tx, "update_checked", l.ID, map[string]any{"available": false, "current_version": in.CurrentVersion})
		}
		if download {
			opened, e = s.openPackage(*chosen)
			if e != nil {
				return e
			}
			if _, _, _, e = s.checkUpdateLicense(scoped, tx, in, req); e != nil {
				return e
			}
		}
		now := s.now()
		until := now.Add(5 * time.Minute)
		for _, limit := range []*time.Time{l.UpdatesUntil, l.ExpiresAt, a.LeaseExpiresAt} {
			if limit != nil && limit.Before(until) {
				until = *limit
			}
		}
		if until.Unix() <= now.Unix() {
			return forbidden("UPDATES_EXPIRED", "更新资格已到期")
		}
		claims := JWTClaims{Issuer: TokenIssuer, Audience: in.AppID, Subject: l.ID, JWTID: s.record(in.AppID, "manifest").ID, IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: until.Unix(), TokenUse: "update_manifest", ActivationID: a.ID, Binding: BindingClaim{a.BindingType, a.ClientBindingHash}, Release: &ReleaseManifest{ID: chosen.ID, Version: chosen.Version, Target: chosen.Target, SHA256: chosen.SHA256, Size: chosen.Size, Filename: chosen.Filename}}
		out.ManifestJWT, e = s.signClaims(scoped, tx, app, claims)
		if e != nil {
			return e
		}
		out.Available = true
		out.Release = chosen
		out.ServerTime = now
		event := "update_checked"
		if releaseID != "" {
			event = "update_install_authorized"
		}
		if download {
			event = "update_download_authorized"
		}
		return s.append(scoped, tx, event, chosen.ID, map[string]any{"license_id": l.ID, "version": chosen.Version, "target": chosen.Target})
	})
	if e != nil {
		if opened != nil {
			opened.Close()
		}
		return UpdateResult{}, nil, s.failure(scoped, repo, "update_authorization", releaseID, e)
	}
	return out, opened, nil
}
func (s *Service) CheckUpdate(ctx context.Context, in UpdateInput, req SignedRequest) (UpdateResult, error) {
	out, _, e := s.authorizeUpdate(ctx, in, "", req, false)
	return out, e
}
func (s *Service) AuthorizeUpdate(ctx context.Context, in UpdateInput, releaseID string, req SignedRequest) (UpdateResult, error) {
	out, _, e := s.authorizeUpdate(ctx, in, releaseID, req, false)
	return out, e
}
func (s *Service) DownloadUpdate(ctx context.Context, in UpdateInput, releaseID string, req SignedRequest) (UpdateResult, *os.File, error) {
	return s.authorizeUpdate(ctx, in, releaseID, req, true)
}
