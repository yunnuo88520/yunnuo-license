package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

const offlineLicenseFileFormat = "yn-license-key"

type CreateOfflineLicenseInput struct {
	ProductID    string `json:"product_id"`
	MachineCode  string `json:"machine_code"`
	Label        string `json:"label"`
	DurationDays int    `json:"duration_days"`
	IsPermanent  bool   `json:"is_permanent"`
}

type OfflineLicensePayload struct {
	Version     int        `json:"version"`
	KeyVersion  int        `json:"key_version"`
	LicenseNo   string     `json:"license_no"`
	ProductID   string     `json:"product_id"`
	ProductCode string     `json:"product_code"`
	ProductName string     `json:"product_name"`
	AppKey      string     `json:"app_key"`
	BindMode    string     `json:"bind_mode"`
	MachineCode string     `json:"machine_code"`
	IssuedAt    time.Time  `json:"issued_at"`
	ExpiredAt   *time.Time `json:"expired_at,omitempty"`
	IsPermanent bool       `json:"is_permanent"`
}

type OfflineLicenseFile struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Token   string `json:"token"`
}

type OfflineLicenseDownload struct {
	Filename string
	Content  []byte
}

func (s *Service) CreateOfflineLicense(ctx context.Context, input CreateOfflineLicenseInput, actorID string) (domain.OfflineLicense, error) {
	input.ProductID = strings.TrimSpace(input.ProductID)
	input.MachineCode = strings.TrimSpace(input.MachineCode)
	input.Label = strings.TrimSpace(input.Label)
	if input.ProductID == "" || input.MachineCode == "" || len(input.MachineCode) > 256 || len(input.Label) > 100 {
		return domain.OfflineLicense{}, ErrInvalidRequest
	}
	if !input.IsPermanent && (input.DurationDays <= 0 || input.DurationDays > 36500) {
		return domain.OfflineLicense{}, ErrInvalidRequest
	}
	product, err := s.store.GetProduct(ctx, input.ProductID)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.OfflineLicense{}, ErrProductNotFound
		}
		return domain.OfflineLicense{}, err
	}
	if product.Status != domain.ProductActive {
		return domain.OfflineLicense{}, ErrProductDisabled
	}
	privatePEM, err := yncrypto.DecryptString(s.dataKey, product.PrivateKeyEncrypted)
	if err != nil {
		return domain.OfflineLicense{}, err
	}
	now := s.now()
	var expiredAt *time.Time
	if !input.IsPermanent {
		expires := now.Add(time.Duration(input.DurationDays) * 24 * time.Hour)
		expiredAt = &expires
	}
	license := domain.OfflineLicense{
		ID:                yncrypto.NewID("offlic"),
		LicenseNo:         yncrypto.NewID("off"),
		ProductID:         product.ID,
		Label:             input.Label,
		MachineCodeHash:   yncrypto.BindHash(s.cardPepper, "offline_machine", input.MachineCode),
		MachineCodeMasked: maskMachineCode(input.MachineCode),
		TokenVersion:      product.KeyVersion,
		Status:            domain.LicenseActive,
		IssuedAt:          now,
		ExpiredAt:         expiredAt,
		CreatedBy:         actorID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	license.MachineCodeEncrypted, err = yncrypto.EncryptString(s.dataKey, input.MachineCode)
	if err != nil {
		return domain.OfflineLicense{}, err
	}
	payload := OfflineLicensePayload{
		Version:     1,
		KeyVersion:  product.KeyVersion,
		LicenseNo:   license.LicenseNo,
		ProductID:   product.ID,
		ProductCode: product.Code,
		ProductName: product.Name,
		AppKey:      product.AppKey,
		BindMode:    domain.BindDevice,
		MachineCode: input.MachineCode,
		IssuedAt:    now,
		ExpiredAt:   expiredAt,
		IsPermanent: input.IsPermanent,
	}
	token, err := yncrypto.SignJSON(privatePEM, payload)
	if err != nil {
		return domain.OfflineLicense{}, err
	}
	license.SignedTokenEncrypted, err = yncrypto.EncryptString(s.dataKey, token)
	if err != nil {
		return domain.OfflineLicense{}, err
	}
	if err := s.store.CreateOfflineLicense(ctx, license); err != nil {
		return domain.OfflineLicense{}, err
	}
	_ = s.audit(ctx, "admin", actorID, product.ID, license.ID, "", "offline_license.create", "success", "")
	return license, nil
}

func (s *Service) ListOfflineLicenses(ctx context.Context) ([]domain.OfflineLicense, error) {
	licenses, err := s.store.ListOfflineLicenses(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	for i := range licenses {
		if licenses[i].Status == domain.LicenseActive && licenses[i].ExpiredAt != nil && !now.Before(*licenses[i].ExpiredAt) {
			licenses[i].Status = domain.LicenseExpired
		}
	}
	return licenses, nil
}

func (s *Service) DownloadOfflineLicense(ctx context.Context, id, actorID string) (OfflineLicenseDownload, error) {
	license, err := s.store.GetOfflineLicense(ctx, strings.TrimSpace(id))
	if err != nil {
		if store.IsNotFound(err) {
			return OfflineLicenseDownload{}, ErrOfflineLicenseNotFound
		}
		return OfflineLicenseDownload{}, err
	}
	if license.Status == domain.LicenseRevoked {
		return OfflineLicenseDownload{}, ErrOfflineLicenseRevoked
	}
	if license.ExpiredAt != nil && !s.now().Before(*license.ExpiredAt) {
		return OfflineLicenseDownload{}, ErrOfflineLicenseExpired
	}
	token, err := yncrypto.DecryptString(s.dataKey, license.SignedTokenEncrypted)
	if err != nil {
		return OfflineLicenseDownload{}, err
	}
	content, err := json.MarshalIndent(OfflineLicenseFile{Format: offlineLicenseFileFormat, Version: 1, Token: token}, "", "  ")
	if err != nil {
		return OfflineLicenseDownload{}, err
	}
	content = append(content, '\n')
	_ = s.audit(ctx, "admin", actorID, license.ProductID, license.ID, "", "offline_license.download", "success", "")
	return OfflineLicenseDownload{Filename: fmt.Sprintf("%s.key", license.LicenseNo), Content: content}, nil
}

func (s *Service) RevokeOfflineLicense(ctx context.Context, id, reason, actorID string) (domain.OfflineLicense, error) {
	id = strings.TrimSpace(id)
	reason = strings.TrimSpace(reason)
	if id == "" || reason == "" || len(reason) > 200 {
		return domain.OfflineLicense{}, ErrInvalidRequest
	}
	license, err := s.store.GetOfflineLicense(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.OfflineLicense{}, ErrOfflineLicenseNotFound
		}
		return domain.OfflineLicense{}, err
	}
	if license.Status == domain.LicenseRevoked {
		return domain.OfflineLicense{}, ErrOfflineLicenseRevoked
	}
	now := s.now()
	if err := s.store.RevokeOfflineLicense(ctx, id, reason, now); err != nil {
		return domain.OfflineLicense{}, err
	}
	license.Status = domain.LicenseRevoked
	license.RevokedAt = &now
	license.RevokedReason = reason
	license.UpdatedAt = now
	_ = s.audit(ctx, "admin", actorID, license.ProductID, license.ID, "", "offline_license.revoke", "success", "")
	return license, nil
}

func VerifyOfflineLicenseFile(publicKeyPEM string, content []byte) (OfflineLicensePayload, error) {
	var file OfflineLicenseFile
	if err := json.Unmarshal(content, &file); err != nil {
		return OfflineLicensePayload{}, err
	}
	if file.Format != offlineLicenseFileFormat || file.Version != 1 || file.Token == "" {
		return OfflineLicensePayload{}, fmt.Errorf("invalid offline license file")
	}
	var payload OfflineLicensePayload
	if err := yncrypto.VerifySignedJSON(publicKeyPEM, file.Token, &payload); err != nil {
		return OfflineLicensePayload{}, err
	}
	if payload.Version != file.Version {
		return OfflineLicensePayload{}, fmt.Errorf("offline license version mismatch")
	}
	return payload, nil
}

func maskMachineCode(value string) string {
	runes := []rune(value)
	if len(runes) <= 8 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:4]) + strings.Repeat("*", len(runes)-8) + string(runes[len(runes)-4:])
}
