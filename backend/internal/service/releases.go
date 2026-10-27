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
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/filelock"
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
	Target    *domain.ReleaseTarget
	ProductID string
	Version   string
	Notes     string
	Filename  string
}

func (s *Service) UploadRelease(ctx context.Context, input UploadReleaseInput, content io.Reader, actorID string) (domain.ProductRelease, error) {
	var result domain.ProductRelease
	target, targetErr := normalizeReleaseTarget(input.Target)
	if targetErr != nil {
		return result, targetErr
	}
	input.Target = target
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
		if r.Version == input.Version && sameReleaseTarget(r.Target, input.Target) {
			return result, ErrReleaseExists
		}
	}
	if err := os.MkdirAll(s.releaseDir, 0700); err != nil {
		return result, err
	}
	if err := s.checkReleaseDisk(uint64(MaxReleaseSize)); err != nil {
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
	if err := tmp.Sync(); err != nil {
		return result, err
	}
	if err := tmp.Close(); err != nil {
		return result, err
	}
	result = domain.ProductRelease{Target: input.Target, ID: yncrypto.NewID("rel"), ProductID: product.ID, Version: input.Version, Notes: input.Notes, Filename: filename, Status: "draft", Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)), KeyVersion: product.KeyVersion, CreatedAt: s.now()}
	privateKey, err := yncrypto.DecryptString(s.dataKey, product.PrivateKeyEncrypted)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	result.ManifestToken, err = yncrypto.SignJSON(privateKey, map[string]any{"type": "product_release", "release_id": result.ID, "product_id": product.ID, "version": result.Version, "sha256": result.SHA256, "size": size, "key_version": product.KeyVersion, "target": input.Target})
	if err != nil {
		return domain.ProductRelease{}, err
	}
	path := filepath.Join(s.releaseDir, result.ID)
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	unlock, err := filelock.Acquire(lockCtx, s.releaseDir+".lock", false)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	defer unlock()
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if e := tx.LockReleaseStorage(ctx); e != nil {
			return e
		}
		if e := tx.LockProduct(ctx, product.ID); e != nil {
			return e
		}
		fresh, e := tx.GetProduct(ctx, product.ID)
		if e != nil {
			return e
		}
		total, developer, used, count, e := tx.ReleaseCapacity(ctx, fresh)
		if e != nil {
			return e
		}
		limits := s.resourceLimits
		if size > limits.TotalBytes-total || size > limits.ProductBytes-used || count >= limits.ProductVersions || fresh.DeveloperID != "" && size > limits.DeveloperBytes-developer {
			return AppError{Code: "STORAGE_QUOTA_EXCEEDED", Message: "应用、开发者或平台的升级包存储额度已用完", HTTPStatus: 409}
		}
		exists, e := tx.ReleaseVersionExists(ctx, product.ID, domain.ReleaseStorageVersion(result.Version, result.Target))
		if e != nil {
			return e
		}
		if exists {
			return ErrReleaseExists
		}
		if e = os.Rename(tmp.Name(), path); e != nil {
			return e
		}
		if e = filelock.SyncDirectory(s.releaseDir); e != nil {
			return e
		}
		if e = tx.CreateRelease(ctx, result); e != nil {
			return e
		}
		return s.transactionAudit(ctx, tx, "admin", actorID, product.ID, "", "product.release.upload", map[string]any{"release_id": result.ID, "size": result.Size, "sha256": result.SHA256})
	})
	if err != nil {
		// Do not destroy a successfully committed file after an ambiguous connection error.
		if _, e := s.currentStore().GetRelease(ctx, result.ID); store.IsNotFound(e) {
			_ = os.Remove(path)
		}
		return domain.ProductRelease{}, err
	}
	return result, nil
}

func (s *Service) ChangeReleaseStatus(ctx context.Context, productID, releaseID, status, actorID string) (domain.ProductRelease, error) {
	var r domain.ProductRelease
	if status != "published" && status != "withdrawn" {
		return r, ErrInvalidRequest
	}
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	unlock, err := filelock.Acquire(lockCtx, s.releaseDir+".lock", false)
	if err != nil {
		return r, err
	}
	defer unlock()
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockReleaseStorage(ctx); err != nil {
			return err
		}
		var err error
		r, err = tx.GetRelease(ctx, releaseID)
		if store.IsNotFound(err) {
			return ErrReleaseNotFound
		}
		if err != nil {
			return err
		}
		if r.Status == "deleted" || r.ProductID != productID {
			return ErrReleaseNotFound
		}
		if err != nil {
			return err
		}
		if status == "published" {
			f, err := s.openRelease(r)
			if err != nil {
				return err
			}
			f.Close()
		}
		if err = tx.SetReleaseStatus(ctx, r.ID, status); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actorID, productID, "", "product.release."+status, map[string]string{"release_id": r.ID})
	})
	if err == nil {
		r.Status = status
	}
	return r, err
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
	input.skipSeat = true // Updating does not consume a runtime seat; all current license, instance and security checks still apply.
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
	lic, err := s.currentStore().GetLicenseByNo(ctx, input.LicenseNo)
	if err != nil {
		return "", err
	}
	if err := enforceSecurity(ctx, s.currentStore(), lic, true); err != nil {
		return "", err
	}
	return p.ID, nil
}

func (s *Service) CheckUpdate(ctx context.Context, input CheckUpdateInput) (CheckUpdateResult, error) {
	input.VerifyInput.CurrentVersion = input.CurrentVersion
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
		if r.Status == "published" && releaseMatches(r, input.ClientInstance) && newerVersion(r.Version, input.CurrentVersion) && (result.Release == nil || newerVersion(r.Version, result.Release.Version)) {
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

func (s *Service) AuthorizeRelease(ctx context.Context, releaseID string, input VerifyInput) (domain.ProductRelease, error) {
	var result domain.ProductRelease
	productID, err := s.authorizeUpdate(ctx, input)
	if err != nil {
		return result, err
	}
	settings, err := s.ProductUpdates(ctx, productID)
	if err != nil {
		return result, err
	}
	if !settings.Enabled {
		return result, ErrUpdatesDisabled
	}
	result, err = s.currentStore().GetRelease(ctx, releaseID)
	if err != nil {
		if store.IsNotFound(err) {
			return result, ErrReleaseNotFound
		}
		return result, err
	}
	if result.ProductID != productID || result.Status != "published" || !releaseMatches(result, input.ClientInstance) {
		return domain.ProductRelease{}, ErrReleaseNotFound
	}
	// Validity is deliberately request-scoped; no reusable download ticket is issued.
	if _, err = s.authorizeUpdate(ctx, input); err != nil {
		return domain.ProductRelease{}, err
	}
	return result, nil
}
func (s *Service) DownloadRelease(ctx context.Context, releaseID string, input VerifyInput) (domain.ProductRelease, *os.File, error) {
	result, err := s.AuthorizeRelease(ctx, releaseID, input)
	if err != nil {
		return result, nil, err
	}
	productID := result.ProductID
	file, err := s.openRelease(result)
	if err == nil {
		// File hashing can take time. Recheck rights immediately before handing out the bytes.
		if _, authErr := s.authorizeUpdate(ctx, input); authErr != nil {
			file.Close()
			return domain.ProductRelease{}, nil, authErr
		}
		current, e := s.currentStore().GetRelease(ctx, result.ID)
		if e != nil || current.Status != "published" {
			file.Close()
			return domain.ProductRelease{}, nil, ErrReleaseNotFound
		}
		settings, e := s.ProductUpdates(ctx, productID)
		if e != nil || !settings.Enabled {
			file.Close()
			if e != nil {
				return domain.ProductRelease{}, nil, e
			}
			return domain.ProductRelease{}, nil, ErrUpdatesDisabled
		}
		if _, e = s.authorizeUpdate(ctx, input); e != nil {
			file.Close()
			return domain.ProductRelease{}, nil, e
		}
	}
	return result, file, err
}
