package service

import (
	"encoding/json"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"regexp"
)

var targetName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)
var imageName = regexp.MustCompile(`^[a-z0-9][a-z0-9./_-]{0,120}:[A-Za-z0-9_.-]{1,100}$`)
var imageDigest = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func normalizeReleaseTarget(t *domain.ReleaseTarget) (*domain.ReleaseTarget, error) {
	if t == nil {
		return nil, nil
	}
	copy := *t
	t = &copy
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
	if !targetName.MatchString(t.OS) || !targetName.MatchString(t.Arch) || !targetName.MatchString(t.Runtime) || !targetName.MatchString(t.Channel) {
		return nil, ErrInvalidRequest
	}
	if t.Kind != "archive" && t.Kind != "installer" && t.Kind != "container_archive" {
		return nil, ErrInvalidRequest
	}
	if t.MinVersion != "" && !versionPattern.MatchString(t.MinVersion) || t.MaxVersion != "" && !versionPattern.MatchString(t.MaxVersion) || t.MinVersion != "" && t.MaxVersion != "" && newerVersion(t.MinVersion, t.MaxVersion) {
		return nil, ErrInvalidRequest
	}
	if t.Kind == "container_archive" && (!imageName.MatchString(t.ImageTag) || !imageDigest.MatchString(t.ImageDigest)) {
		return nil, badRequest("IMAGE_IDENTITY_REQUIRED", "容器版本需要镜像标签及不可变 sha256 镜像 ID")
	}
	if t.Kind != "container_archive" && (t.ImageTag != "" || t.ImageDigest != "") {
		return nil, ErrInvalidRequest
	}
	return t, nil
}
func sameReleaseTarget(a, b *domain.ReleaseTarget) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.OS == b.OS && a.Arch == b.Arch && a.Runtime == b.Runtime && a.Channel == b.Channel && a.Kind == b.Kind
}
func releaseMatches(r domain.ProductRelease, in domain.ClientInstance) bool {
	if r.Target == nil {
		return true
	}
	if in.Target == nil {
		return false
	}
	a, b := r.Target, in.Target
	for _, pair := range [][2]string{{a.OS, b.OS}, {a.Arch, b.Arch}, {a.Runtime, b.Runtime}} {
		if pair[0] != "any" && pair[0] != pair[1] {
			return false
		}
	}
	channel := b.Channel
	if channel == "" {
		channel = "stable"
	}
	if channel != a.Channel {
		return false
	}
	if b.Kind != "" && b.Kind != a.Kind {
		return false
	}
	if a.MinVersion != "" || a.MaxVersion != "" {
		if !versionPattern.MatchString(in.CurrentVersion) {
			return false
		}
		if a.MinVersion != "" && newerVersion(a.MinVersion, in.CurrentVersion) || a.MaxVersion != "" && newerVersion(in.CurrentVersion, a.MaxVersion) {
			return false
		}
	}
	return true
}
func ParseReleaseTarget(raw string) (*domain.ReleaseTarget, error) {
	if raw == "" {
		return nil, nil
	}
	var t domain.ReleaseTarget
	if json.Unmarshal([]byte(raw), &t) != nil {
		return nil, ErrInvalidRequest
	}
	return normalizeReleaseTarget(&t)
}
