package service

import (
	"context"
	"fmt"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/filelock"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"os"
	"strconv"
	"strings"
	"time"
)

type ResourceLimits struct {
	TotalBytes          int64  `json:"total_bytes"`
	DeveloperBytes      int64  `json:"developer_bytes"`
	ProductBytes        int64  `json:"product_bytes"`
	ProductVersions     int    `json:"product_versions"`
	ConcurrentDownloads int    `json:"concurrent_downloads"`
	ConcurrentUploads   int    `json:"concurrent_uploads"`
	MinFreeBytes        uint64 `json:"min_free_bytes"`
}

func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{TotalBytes: 16 << 30, DeveloperBytes: 8 << 30, ProductBytes: 2 << 30, ProductVersions: 100, ConcurrentUploads: 2, ConcurrentDownloads: 8, MinFreeBytes: 256 << 20}
}
func ResourceLimitsFromEnvironment() (ResourceLimits, error) {
	l := DefaultResourceLimits()
	for _, item := range []struct {
		name  string
		value *int64
	}{{"YN_RELEASE_TOTAL_BYTES", &l.TotalBytes}, {"YN_RELEASE_DEVELOPER_BYTES", &l.DeveloperBytes}, {"YN_RELEASE_PRODUCT_BYTES", &l.ProductBytes}} {
		if raw := os.Getenv(item.name); raw != "" {
			n, e := strconv.ParseInt(raw, 10, 64)
			if e != nil || n < 1 || n > 1<<50 {
				return l, fmt.Errorf("%s is invalid", item.name)
			}
			*item.value = n
		}
	}
	for _, item := range []struct {
		name  string
		value *int
		max   int
	}{{"YN_RELEASE_PRODUCT_VERSIONS", &l.ProductVersions, 100000}, {"YN_RELEASE_CONCURRENT_UPLOADS", &l.ConcurrentUploads, 16}, {"YN_RELEASE_CONCURRENT_DOWNLOADS", &l.ConcurrentDownloads, 64}} {
		if raw := os.Getenv(item.name); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 || n > item.max {
				return l, fmt.Errorf("%s is invalid", item.name)
			}
			*item.value = n
		}
	}
	if raw := os.Getenv("YN_RELEASE_MIN_FREE_BYTES"); raw != "" {
		n, e := strconv.ParseUint(raw, 10, 64)
		if e != nil {
			return l, fmt.Errorf("YN_RELEASE_MIN_FREE_BYTES is invalid")
		}
		l.MinFreeBytes = n
	}
	return l, nil
}
func (s *Service) WithResourceLimits(l ResourceLimits) *Service {
	if l.ConcurrentDownloads < 1 {
		l.ConcurrentDownloads = DefaultResourceLimits().ConcurrentDownloads
	}
	s.resourceLimits = l
	s.uploadSlots = make(chan struct{}, l.ConcurrentUploads)
	s.downloadSlots = make(chan struct{}, l.ConcurrentDownloads)
	return s
}
func (s *Service) ResourceLimits() ResourceLimits { return s.resourceLimits }
func (s *Service) BeginReleaseUpload(ctx context.Context) (func(), error) {
	slots := s.uploadSlots
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, AppError{Code: "UPLOAD_BUSY", Message: "同时上传的升级包过多，请稍后重试", HTTPStatus: 429}
	}
	release := func() { <-slots }
	if err := s.checkReleaseDisk(2 * uint64(MaxReleaseSize) * uint64(len(slots))); err != nil {
		release()
		return nil, err
	}
	return release, nil
}
func (s *Service) checkReleaseDisk(reserved uint64) error {
	if s.releaseDir == "" {
		return ErrInvalidRequest
	}
	if err := os.MkdirAll(s.releaseDir, 0700); err != nil {
		return err
	}
	for _, path := range []string{s.releaseDir, os.TempDir()} {
		free, err := availableDiskBytes(path)
		if err != nil {
			return err
		}
		if free < s.resourceLimits.MinFreeBytes+reserved {
			return AppError{Code: "STORAGE_SPACE_LOW", Message: "存储空间不足，暂时停止上传", HTTPStatus: 507}
		}
	}
	return nil
}
func (s *Service) ReleaseStorageUsage(ctx context.Context) (map[string]any, error) {
	usage, err := s.currentStore().ReleaseStorageUsage(ctx)
	return map[string]any{"usage": usage, "limits": s.resourceLimits}, err
}

// CleanupReleaseTemps removes only application-owned stale upload/trash/probe files.
// Published packages are never removed by this maintenance pass.
func (s *Service) CleanupReleaseTemps(now time.Time) (int, error) {
	if s.releaseDir == "" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	unlock, err := filelock.Acquire(ctx, s.releaseDir+".lock", false)
	if err != nil {
		return 0, err
	}
	defer unlock()
	entries, err := os.ReadDir(s.releaseDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, ".upload-") && !strings.HasPrefix(name, ".trash-") && !strings.HasPrefix(name, ".ready-") {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return removed, e
		}
		if now.Sub(info.ModTime()) < 24*time.Hour {
			continue
		}
		path := s.releaseDir + string(os.PathSeparator) + name
		if strings.HasPrefix(name, ".trash-") {
			r, e := s.currentStore().GetRelease(context.Background(), strings.TrimPrefix(name, ".trash-"))
			if e != nil && !store.IsNotFound(e) {
				return removed, e
			}
			if e == nil && r.Status != "deleted" {
				original := s.releaseDir + string(os.PathSeparator) + r.ID
				if _, statErr := os.Stat(original); os.IsNotExist(statErr) {
					if e = os.Rename(path, original); e != nil {
						return removed, e
					}
					continue
				}
				// Leave ambiguous copies for an operator instead of deleting a referenced blob.
				continue
			}
		}
		if e = os.Remove(path); e != nil {
			return removed, e
		}
		removed++
	}
	return removed, nil
}

// The HTTP handler holds this slot through hashing and streaming, limiting
// open packages and disk/CPU work independently of authorization traffic.
func (s *Service) BeginReleaseDownload(ctx context.Context) (func(), error) {
	select {
	case s.downloadSlots <- struct{}{}:
		return func() { <-s.downloadSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, AppError{Code: "DOWNLOAD_BUSY", Message: "同时下载的升级包过多，请稍后重试", HTTPStatus: 429}
	}
}
