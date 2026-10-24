package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

const MaxReleaseSize int64 = 256 << 20

var (
	ErrUpdatesDisabled  = forbidden("UPDATES_DISABLED", "product update service is disabled")
	ErrReleaseNotFound  = notFound("RELEASE_NOT_FOUND", "release not found")
	ErrReleaseExists    = badRequest("RELEASE_EXISTS", "this product version already exists")
	ErrReleaseTooLarge  = AppError{Code: "RELEASE_TOO_LARGE", Message: "package exceeds 256 MiB", HTTPStatus: 413}
	ErrReleaseIntegrity = badRequest("RELEASE_INTEGRITY_FAILED", "release package is missing or corrupted")
	versionPattern      = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)
)

// WithReleaseDir must be called before serving requests. Packages are never in a static directory.
func (s *Service) WithReleaseDir(dir string) *Service { s.releaseDir = dir; return s }

func newerVersion(a, b string) bool {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range aa {
		x, _ := strconv.Atoi(aa[i])
		y, _ := strconv.Atoi(bb[i])
		if x != y {
			return x > y
		}
	}
	return false
}

func (s *Service) ProductUpdates(ctx context.Context, productID string) (domain.ProductUpdateSettings, error) {
	if _, err := s.currentStore().GetProduct(ctx, productID); err != nil {
		if store.IsNotFound(err) {
			return domain.ProductUpdateSettings{}, ErrProductNotFound
		}
		return domain.ProductUpdateSettings{}, err
	}
	return s.currentStore().ProductUpdates(ctx, productID)
}

func (s *Service) SetProductUpdates(ctx context.Context, productID string, enabled bool, actorID string) (domain.ProductUpdateSettings, error) {
	settings, err := s.ProductUpdates(ctx, productID)
	if err != nil {
		return settings, err
	}
	settings.Enabled = enabled
	if err = s.currentStore().SetProductUpdates(ctx, settings); err != nil {
		return settings, err
	}
	_ = s.audit(ctx, "admin", actorID, productID, "", "", "product.updates.configure", "success", "")
	return settings, nil
}

func (s *Service) ListReleases(ctx context.Context, productID string) ([]domain.ProductRelease, error) {
	if _, err := s.ProductUpdates(ctx, productID); err != nil {
		return nil, err
	}
	return s.currentStore().ListReleases(ctx, productID)
}

type UploadReleaseInput struct {
	ProductID string
	Version   string
	Notes     string
	Filename  string
}

func (s *Service) UploadRelease(ctx context.Context, input UploadReleaseInput, content io.Reader, actorID string) (domain.ProductRelease, error) {
	var result domain.ProductRelease
	if !versionPattern.MatchString(input.Version) || len(input.Notes) > 16000 || s.releaseDir == "" {
		return result, ErrInvalidRequest
	}
	filename := filepath.Base(strings.ReplaceAll(input.Filename, `\`, "/"))
	lower := strings.ToLower(filename)
	if len(filename) > 200 || strings.ContainsAny(filename, "\r\n\x00") || !(strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")) {
		return result, ErrInvalidRequest
	}
	product, err := s.currentStore().GetProduct(ctx, input.ProductID)
	if err != nil {
		if store.IsNotFound(err) {
			return result, ErrProductNotFound
		}
		return result, err
	}
	existing, err := s.currentStore().ListReleases(ctx, product.ID)
	if err != nil {
		return result, err
	}
	for _, r := range existing {
		if r.Version == input.Version {
			return result, ErrReleaseExists
		}
	}
	if err := os.MkdirAll(s.releaseDir, 0700); err != nil {
		return result, err
	}
	tmp, err := os.CreateTemp(s.releaseDir, ".upload-*")
	if err != nil {
		return result, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(content, MaxReleaseSize+1))
	if err != nil {
		return result, err
	}
	if size > MaxReleaseSize {
		return result, ErrReleaseTooLarge
	}
	if size == 0 {
		return result, ErrInvalidRequest
	}
	if err := tmp.Close(); err != nil {
		return result, err
	}
	result = domain.ProductRelease{ID: yncrypto.NewID("rel"), ProductID: product.ID, Version: input.Version, Notes: input.Notes, Filename: filename, Status: "draft", Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)), KeyVersion: product.KeyVersion, CreatedAt: s.now()}
	privateKey, err := yncrypto.DecryptString(s.dataKey, product.PrivateKeyEncrypted)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	result.ManifestToken, err = yncrypto.SignJSON(privateKey, map[string]any{"type": "product_release", "release_id": result.ID, "product_id": product.ID, "version": result.Version, "sha256": result.SHA256, "size": size, "key_version": product.KeyVersion})
	if err != nil {
		return domain.ProductRelease{}, err
	}
	path := filepath.Join(s.releaseDir, result.ID)
	if err := os.Rename(tmp.Name(), path); err != nil {
		return domain.ProductRelease{}, err
	}
	if err := s.currentStore().CreateRelease(ctx, result); err != nil {
		_ = os.Remove(path)
		return domain.ProductRelease{}, err
	}
	_ = s.audit(ctx, "admin", actorID, product.ID, "", "", "product.release.upload", "success", "")
	return result, nil
}

func (s *Service) ChangeReleaseStatus(ctx context.Context, productID, releaseID, status, actorID string) (domain.ProductRelease, error) {
	r, err := s.currentStore().GetRelease(ctx, releaseID)
	if err != nil {
		if store.IsNotFound(err) {
			return r, ErrReleaseNotFound
		}
		return r, err
	}
	if r.ProductID != productID {
		return domain.ProductRelease{}, ErrReleaseNotFound
	}
	if status != "published" && status != "withdrawn" {
		return domain.ProductRelease{}, ErrInvalidRequest
	}
	if status == "published" {
		file, err := s.openRelease(r)
		if err != nil {
			return r, err
		}
		_ = file.Close()
	}
	if err := s.currentStore().SetReleaseStatus(ctx, r.ID, status); err != nil {
		return r, err
	}
	r.Status = status
	_ = s.audit(ctx, "admin", actorID, r.ProductID, "", "", "product.release."+status, "success", "")
	return r, nil
}

type CheckUpdateInput struct {
	VerifyInput
	CurrentVersion string `json:"current_version"`
}

type CheckUpdateResult struct {
	Enabled         bool                   `json:"enabled"`
	UpdateAvailable bool                   `json:"update_available"`
	Release         *domain.ProductRelease `json:"release,omitempty"`
}

// Update access deliberately excludes the license's runtime expiry grace period.
func (s *Service) authorizeUpdate(ctx context.Context, input VerifyInput) (string, error) {
	verified, err := s.Verify(ctx, input)
	if err != nil {
		return "", err
	}
	if verified.ExpiredAt != nil && !s.now().Before(*verified.ExpiredAt) {
		return "", ErrLicenseExpired
	}
	if !beforeExpiry(s.now(), verified.UpdatesExpiredAt) {
		return "", forbidden("UPDATES_EXPIRED", "update entitlement expired; existing usage rights are unchanged")
	}
	p, err := s.productByAppKey(ctx, input.AppKey)
	if err != nil {
		return "", err
	}
	return p.ID, nil
}

func (s *Service) CheckUpdate(ctx context.Context, input CheckUpdateInput) (CheckUpdateResult, error) {
	var result CheckUpdateResult
	if !versionPattern.MatchString(input.CurrentVersion) {
		return result, ErrInvalidRequest
	}
	productID, err := s.authorizeUpdate(ctx, input.VerifyInput)
	if err != nil {
		return result, err
	}
	settings, err := s.ProductUpdates(ctx, productID)
	if err != nil {
		return result, err
	}
	result.Enabled = settings.Enabled
	if !settings.Enabled {
		return result, nil
	}
	items, err := s.currentStore().ListReleases(ctx, productID)
	if err != nil {
		return result, err
	}
	for _, r := range items {
		if r.Status == "published" && newerVersion(r.Version, input.CurrentVersion) && (result.Release == nil || newerVersion(r.Version, result.Release.Version)) {
			copy := r
			result.Release = &copy
		}
	}
	result.UpdateAvailable = result.Release != nil
	return result, nil
}

func (s *Service) openRelease(r domain.ProductRelease) (*os.File, error) {
	if s.releaseDir == "" {
		return nil, ErrReleaseIntegrity
	}
	file, err := os.Open(filepath.Join(s.releaseDir, r.ID))
	if err != nil {
		return nil, ErrReleaseIntegrity
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, r.Size+1))
	if err != nil || size != r.Size || hex.EncodeToString(hash.Sum(nil)) != r.SHA256 {
		_ = file.Close()
		return nil, ErrReleaseIntegrity
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func (s *Service) DownloadRelease(ctx context.Context, releaseID string, input VerifyInput) (domain.ProductRelease, *os.File, error) {
	var result domain.ProductRelease
	productID, err := s.authorizeUpdate(ctx, input)
	if err != nil {
		return result, nil, err
	}
	settings, err := s.ProductUpdates(ctx, productID)
	if err != nil {
		return result, nil, err
	}
	if !settings.Enabled {
		return result, nil, ErrUpdatesDisabled
	}
	result, err = s.currentStore().GetRelease(ctx, releaseID)
	if err != nil {
		if store.IsNotFound(err) {
			return result, nil, ErrReleaseNotFound
		}
		return result, nil, err
	}
	if result.ProductID != productID || result.Status != "published" {
		return domain.ProductRelease{}, nil, ErrReleaseNotFound
	}
	file, err := s.openRelease(result)
	if err == nil {
		// File hashing can take time. Recheck rights immediately before handing out the bytes.
		if _, authErr := s.authorizeUpdate(ctx, input); authErr != nil {
			file.Close()
			return domain.ProductRelease{}, nil, authErr
		}
	}
	return result, file, err
}
