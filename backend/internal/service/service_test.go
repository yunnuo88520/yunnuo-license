package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func TestOfflineLicenseSigningTamperAndRevocation(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	machineCode := "MACHINE-ABCDEF-123456"

	created, err := svc.CreateOfflineLicense(ctx, CreateOfflineLicenseInput{
		ProductID:    product.ID,
		MachineCode:  machineCode,
		Label:        "Factory workstation",
		DurationDays: 365,
	}, "admin-test")
	if err != nil {
		t.Fatalf("create offline license: %v", err)
	}
	if created.MachineCodeMasked == machineCode || created.MachineCodeEncrypted == machineCode {
		t.Fatalf("machine code was not protected: %#v", created)
	}

	listed, err := svc.ListOfflineLicenses(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list offline licenses: licenses=%#v err=%v", listed, err)
	}
	listedJSON, err := json.Marshal(listed)
	if err != nil {
		t.Fatalf("marshal offline licenses: %v", err)
	}
	if strings.Contains(string(listedJSON), machineCode) || strings.Contains(string(listedJSON), "signed_token") {
		t.Fatalf("offline list leaked protected data: %s", listedJSON)
	}

	download, err := svc.DownloadOfflineLicense(ctx, created.ID, "admin-test")
	if err != nil {
		t.Fatalf("download offline license: %v", err)
	}
	payload, err := VerifyOfflineLicenseFile(product.PublicKeyPEM, download.Content)
	if err != nil {
		t.Fatalf("verify offline license: %v", err)
	}
	if payload.LicenseNo != created.LicenseNo || payload.MachineCode != machineCode || payload.ProductID != product.ID {
		t.Fatalf("unexpected signed payload: %#v", payload)
	}

	var file OfflineLicenseFile
	if err := json.Unmarshal(download.Content, &file); err != nil {
		t.Fatalf("decode offline file: %v", err)
	}
	signatureStart := strings.IndexByte(file.Token, '.') + 1
	if signatureStart <= 0 || signatureStart >= len(file.Token) {
		t.Fatalf("invalid signed token: %q", file.Token)
	}
	if file.Token[signatureStart] == 'A' {
		file.Token = file.Token[:signatureStart] + "B" + file.Token[signatureStart+1:]
	} else {
		file.Token = file.Token[:signatureStart] + "A" + file.Token[signatureStart+1:]
	}
	tampered, _ := json.Marshal(file)
	if _, err := VerifyOfflineLicenseFile(product.PublicKeyPEM, tampered); err == nil {
		t.Fatal("expected tampered offline license verification to fail")
	}

	revoked, err := svc.RevokeOfflineLicense(ctx, created.ID, "deployment_retired", "admin-test")
	if err != nil || revoked.Status != domain.LicenseRevoked {
		t.Fatalf("revoke offline license: license=%#v err=%v", revoked, err)
	}
	_, err = svc.DownloadOfflineLicense(ctx, created.ID, "admin-test")
	assertCode(t, err, "OFFLINE_LICENSE_REVOKED")
}

func TestProductKeyRotationPreservesOldPublicKeyAndSignsWithNewKey(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	code := createTestCard(t, ctx, svc, product.ID, 30)

	first, err := svc.Activate(ctx, ActivateInput{
		AppKey: product.AppKey, CardCode: code, BindMode: domain.BindDevice, BindValue: "machine-key-rotation",
	})
	if err != nil {
		t.Fatalf("activate before key rotation: %v", err)
	}
	var firstClaims map[string]any
	if err := yncrypto.VerifySignedJSON(product.PublicKeyPEM, first.OfflineToken, &firstClaims); err != nil {
		t.Fatalf("verify token with version 1 key: %v", err)
	}
	if firstClaims["key_version"] != float64(1) {
		t.Fatalf("expected key version 1, got %#v", firstClaims)
	}

	ring, err := svc.RotateProductKey(ctx, product.ID, "admin-test")
	if err != nil {
		t.Fatalf("rotate product key: %v", err)
	}
	if ring.CurrentVersion != 2 || len(ring.Keys) != 2 || ring.Keys[0].KeyVersion != 2 || ring.Keys[1].KeyVersion != 1 {
		t.Fatalf("unexpected key ring: %#v", ring)
	}
	if ring.Keys[1].PublicKeyPEM != product.PublicKeyPEM {
		t.Fatal("version 1 public key was not preserved")
	}

	second, err := svc.Verify(ctx, VerifyInput{
		AppKey: product.AppKey, LicenseNo: first.LicenseNo, BindMode: domain.BindDevice, BindValue: "machine-key-rotation",
	})
	if err != nil {
		t.Fatalf("verify after key rotation: %v", err)
	}
	var secondClaims map[string]any
	if err := yncrypto.VerifySignedJSON(ring.Keys[0].PublicKeyPEM, second.OfflineToken, &secondClaims); err != nil {
		t.Fatalf("verify token with version 2 key: %v", err)
	}
	if secondClaims["key_version"] != float64(2) {
		t.Fatalf("expected key version 2, got %#v", secondClaims)
	}
	if err := yncrypto.VerifySignedJSON(product.PublicKeyPEM, second.OfflineToken, &map[string]any{}); err == nil {
		t.Fatal("new token unexpectedly verified with retired signing key")
	}

	offline, err := svc.CreateOfflineLicense(ctx, CreateOfflineLicenseInput{
		ProductID: product.ID, MachineCode: "OFFLINE-ROTATION-MACHINE", DurationDays: 30,
	}, "admin-test")
	if err != nil {
		t.Fatalf("create offline license after rotation: %v", err)
	}
	if offline.TokenVersion != 2 {
		t.Fatalf("expected offline key version 2, got %d", offline.TokenVersion)
	}
	download, err := svc.DownloadOfflineLicense(ctx, offline.ID, "admin-test")
	if err != nil {
		t.Fatalf("download rotated offline license: %v", err)
	}
	var file OfflineLicenseFile
	if err := json.Unmarshal(download.Content, &file); err != nil || file.Version != 1 {
		t.Fatalf("offline file format version changed: file=%#v err=%v", file, err)
	}
	payload, err := VerifyOfflineLicenseFile(ring.Keys[0].PublicKeyPEM, download.Content)
	if err != nil || payload.KeyVersion != 2 {
		t.Fatalf("verify rotated offline license: payload=%#v err=%v", payload, err)
	}
}

func TestActivateIsIdempotentAndRejectsSecondDevice(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	code := createTestCard(t, ctx, svc, product.ID, 30)

	first, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  code,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	if err != nil {
		t.Fatalf("activate first device: %v", err)
	}
	second, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  code,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	if err != nil {
		t.Fatalf("activate same device: %v", err)
	}
	if first.LicenseNo != second.LicenseNo {
		t.Fatalf("expected idempotent license_no %q, got %q", first.LicenseNo, second.LicenseNo)
	}
	var offlineClaims map[string]any
	if err := yncrypto.VerifySignedJSON(product.PublicKeyPEM, first.OfflineToken, &offlineClaims); err != nil {
		t.Fatalf("verify offline token: %v", err)
	}
	if offlineClaims["bind_digest"] != yncrypto.BindDigest(domain.BindDevice, "machine-A") {
		t.Fatalf("offline token missing client-verifiable binding digest: %#v", offlineClaims)
	}

	_, err = svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  code,
		BindMode:  domain.BindDevice,
		BindValue: "machine-B",
	})
	assertCode(t, err, "DEVICE_LIMIT_EXCEEDED")
}

func TestPublicLicenseQuerySupportsCardAndLicenseNumber(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	code := createTestCard(t, ctx, svc, product.ID, 30)

	unused, err := svc.QueryPublicLicense(ctx, PublicLicenseQueryInput{CardCode: code})
	if err != nil {
		t.Fatalf("query unused card: %v", err)
	}
	if unused.CardStatus != domain.CardUnused || unused.ProductName != product.Name || unused.LicenseNo != "" {
		t.Fatalf("unexpected unused card query: %#v", unused)
	}

	activated, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  code,
		BindMode:  domain.BindDevice,
		BindValue: "public-query-device",
	})
	if err != nil {
		t.Fatalf("activate card: %v", err)
	}
	byCard, err := svc.QueryPublicLicense(ctx, PublicLicenseQueryInput{CardCode: code})
	if err != nil {
		t.Fatalf("query activated card: %v", err)
	}
	byLicense, err := svc.QueryPublicLicense(ctx, PublicLicenseQueryInput{LicenseNo: activated.LicenseNo})
	if err != nil {
		t.Fatalf("query license: %v", err)
	}
	if byCard.LicenseNo != activated.LicenseNo || byLicense.LicenseStatus != domain.LicenseActive {
		t.Fatalf("unexpected public query results: card=%#v license=%#v", byCard, byLicense)
	}

	_, err = svc.QueryPublicLicense(ctx, PublicLicenseQueryInput{LicenseNo: "lic_missing"})
	assertCode(t, err, "AUTHORIZATION_NOT_FOUND")
}

func TestProductLifecycleCardVoidAndBatchExport(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	batch, err := svc.CreateCardBatch(ctx, CreateCardBatchInput{
		ProductID:    product.ID,
		Name:         "lifecycle cards",
		Quantity:     2,
		DurationDays: 30,
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	exported, err := svc.ExportCardBatch(ctx, batch.Batch.ID, "admin-test")
	if err != nil || len(exported.Codes) != 2 || exported.Batch.ExportCount != 1 {
		t.Fatalf("export batch: result=%#v err=%v", exported, err)
	}
	cards, err := svc.ListCardsByBatch(ctx, batch.Batch.ID)
	if err != nil || len(cards) != 2 {
		t.Fatalf("list cards: cards=%#v err=%v", cards, err)
	}
	voided, err := svc.VoidCard(ctx, cards[0].ID, "duplicate_generation", "admin-test")
	if err != nil || voided.Status != domain.CardVoided {
		t.Fatalf("void unused card: card=%#v err=%v", voided, err)
	}
	queried, err := svc.QueryPublicLicense(ctx, PublicLicenseQueryInput{CardCode: batch.Codes[0]})
	if err != nil || queried.CardStatus != domain.CardVoided {
		t.Fatalf("query voided card: result=%#v err=%v", queried, err)
	}
	_, err = svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  batch.Codes[0],
		BindMode:  domain.BindDevice,
		BindValue: "voided-device",
	})
	assertCode(t, err, "CARD_VOIDED")

	activated, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  batch.Codes[1],
		BindMode:  domain.BindDevice,
		BindValue: "active-device",
	})
	if err != nil {
		t.Fatalf("activate second card: %v", err)
	}
	_, err = svc.VoidCard(ctx, cards[1].ID, "already_activated", "admin-test")
	assertCode(t, err, "CARD_CANNOT_VOID")

	if _, err := svc.ChangeProductStatus(ctx, product.ID, domain.ProductDisabled, "admin-test"); err != nil {
		t.Fatalf("disable product: %v", err)
	}
	_, err = svc.CreateCardBatch(ctx, CreateCardBatchInput{ProductID: product.ID, Quantity: 1, DurationDays: 30})
	assertCode(t, err, "PRODUCT_DISABLED")
	_, err = svc.Verify(ctx, VerifyInput{
		AppKey:    product.AppKey,
		LicenseNo: activated.LicenseNo,
		BindMode:  domain.BindDevice,
		BindValue: "active-device",
	})
	assertCode(t, err, "PRODUCT_DISABLED")
	if _, err := svc.ChangeProductStatus(ctx, product.ID, domain.ProductActive, "admin-test"); err != nil {
		t.Fatalf("enable product: %v", err)
	}
	if _, err := svc.Verify(ctx, VerifyInput{
		AppKey:    product.AppKey,
		LicenseNo: activated.LicenseNo,
		BindMode:  domain.BindDevice,
		BindValue: "active-device",
	}); err != nil {
		t.Fatalf("verify after product enable: %v", err)
	}
}

func TestLicenseListFilteringAndPagination(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	batch, err := svc.CreateCardBatch(ctx, CreateCardBatchInput{
		ProductID:    product.ID,
		Name:         "paged licenses",
		Quantity:     3,
		DurationDays: 30,
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	licenses := make([]LicenseResponse, 0, 3)
	for i, code := range batch.Codes {
		license, err := svc.Activate(ctx, ActivateInput{
			AppKey:    product.AppKey,
			CardCode:  code,
			BindMode:  domain.BindDevice,
			BindValue: "paged-device-" + string(rune('A'+i)),
		})
		if err != nil {
			t.Fatalf("activate card %d: %v", i, err)
		}
		licenses = append(licenses, license)
	}

	pageOne, err := svc.ListLicensesPage(ctx, LicenseListFilter{Page: 1, PageSize: 2})
	if err != nil || pageOne.Total != 3 || len(pageOne.Items) != 2 {
		t.Fatalf("unexpected first page: page=%#v err=%v", pageOne, err)
	}
	pageTwo, err := svc.ListLicensesPage(ctx, LicenseListFilter{Page: 2, PageSize: 2})
	if err != nil || pageTwo.Total != 3 || len(pageTwo.Items) != 1 {
		t.Fatalf("unexpected second page: page=%#v err=%v", pageTwo, err)
	}
	searched, err := svc.ListLicensesPage(ctx, LicenseListFilter{Query: licenses[0].LicenseNo, Page: 1, PageSize: 20})
	if err != nil || searched.Total != 1 || searched.Items[0].LicenseNo != licenses[0].LicenseNo {
		t.Fatalf("unexpected search result: page=%#v err=%v", searched, err)
	}
	if _, err := svc.RevokeLicense(ctx, RevokeLicenseInput{LicenseNo: licenses[0].LicenseNo, Reason: "test"}); err != nil {
		t.Fatalf("revoke license: %v", err)
	}
	revoked, err := svc.ListLicensesPage(ctx, LicenseListFilter{Status: domain.LicenseRevoked})
	if err != nil || revoked.Total != 1 {
		t.Fatalf("unexpected revoked filter: page=%#v err=%v", revoked, err)
	}
	future := time.Now().UTC().AddDate(0, 0, 31)
	svc.now = func() time.Time { return future }
	expired, err := svc.ListLicensesPage(ctx, LicenseListFilter{Status: domain.LicenseExpired})
	if err != nil || expired.Total != 2 || expired.Items[0].Status != domain.LicenseExpired {
		t.Fatalf("unexpected expired filter: page=%#v err=%v", expired, err)
	}
}

func TestKickOldestAllowsNewDeviceAndInvalidatesOldBinding(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictKickOldest)
	code := createTestCard(t, ctx, svc, product.ID, 30)

	first, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  code,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	if err != nil {
		t.Fatalf("activate first device: %v", err)
	}
	second, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  code,
		BindMode:  domain.BindDevice,
		BindValue: "machine-B",
	})
	if err != nil {
		t.Fatalf("activate second device with kick_oldest: %v", err)
	}
	if first.LicenseNo != second.LicenseNo {
		t.Fatalf("expected same license after kick, got %q and %q", first.LicenseNo, second.LicenseNo)
	}

	_, err = svc.Verify(ctx, VerifyInput{
		AppKey:    product.AppKey,
		LicenseNo: first.LicenseNo,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	assertCode(t, err, "BINDING_MISMATCH")

	if _, err := svc.Verify(ctx, VerifyInput{
		AppKey:    product.AppKey,
		LicenseNo: first.LicenseNo,
		BindMode:  domain.BindDevice,
		BindValue: "machine-B",
	}); err != nil {
		t.Fatalf("verify new binding: %v", err)
	}
}

func TestUnbindReleasesSeat(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	code := createTestCard(t, ctx, svc, product.ID, 30)

	activated, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  code,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if _, err := svc.Unbind(ctx, UnbindInput{
		AppKey:    product.AppKey,
		LicenseNo: activated.LicenseNo,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
		Reason:    "change_device",
	}); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	_, err = svc.Verify(ctx, VerifyInput{
		AppKey:    product.AppKey,
		LicenseNo: activated.LicenseNo,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	assertCode(t, err, "BINDING_MISMATCH")

	if _, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  code,
		BindMode:  domain.BindDevice,
		BindValue: "machine-B",
	}); err != nil {
		t.Fatalf("activate after unbind: %v", err)
	}
}

func TestRevokeBlocksVerify(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	code := createTestCard(t, ctx, svc, product.ID, 30)

	activated, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  code,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if _, err := svc.RevokeLicense(ctx, RevokeLicenseInput{
		LicenseNo: activated.LicenseNo,
		Reason:    "policy_violation",
	}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_, err = svc.Verify(ctx, VerifyInput{
		AppKey:    product.AppKey,
		LicenseNo: activated.LicenseNo,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	assertCode(t, err, "LICENSE_REVOKED")
}

func TestRenewExtendsFromExistingExpiry(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	activateCode := createTestCard(t, ctx, svc, product.ID, 30)
	renewCode := createTestCard(t, ctx, svc, product.ID, 10)

	activated, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  activateCode,
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	renewed, err := svc.Renew(ctx, RenewInput{
		AppKey:        product.AppKey,
		LicenseNo:     activated.LicenseNo,
		RenewCardCode: renewCode,
		BindMode:      domain.BindDevice,
		BindValue:     "machine-A",
	})
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if activated.ExpiredAt == nil || renewed.ExpiredAt == nil {
		t.Fatal("expected non-permanent expiry")
	}
	want := activated.ExpiredAt.AddDate(0, 0, 10)
	if !renewed.ExpiredAt.Equal(want) {
		t.Fatalf("expected renewed expiry %s, got %s", want, *renewed.ExpiredAt)
	}
}

func TestAgentCanGenerateCardsWithinQuota(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	agent, err := svc.CreateAgent(ctx, CreateAgentInput{
		Name: "华东代理",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.UpsertAgentProductPolicy(ctx, AgentProductPolicyInput{
		AgentID:             agent.ID,
		ProductID:           product.ID,
		CanGenerate:         true,
		AllowedDurationDays: []int{30},
		MaxBatchQuantity:    10,
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	if _, err := svc.GrantAgentQuota(ctx, AgentQuotaInput{
		AgentID:      agent.ID,
		ProductID:    product.ID,
		DurationDays: 30,
		Quantity:     5,
	}); err != nil {
		t.Fatalf("grant quota: %v", err)
	}
	generated, err := svc.AgentCreateCardBatch(ctx, AgentCreateCardBatchInput{
		AgentID:      agent.ID,
		ProductID:    product.ID,
		Name:         "agent batch",
		Quantity:     2,
		DurationDays: 30,
	})
	if err != nil {
		t.Fatalf("agent create cards: %v", err)
	}
	if generated.Batch.AgentID != agent.ID {
		t.Fatalf("expected batch agent_id %q, got %q", agent.ID, generated.Batch.AgentID)
	}
	quotas, err := svc.ListAgentQuotaSummaries(ctx, agent.ID)
	if err != nil {
		t.Fatalf("list quota summaries: %v", err)
	}
	if len(quotas) != 1 || quotas[0].Balance != 3 {
		t.Fatalf("expected remaining quota 3, got %#v", quotas)
	}
	activated, err := svc.Activate(ctx, ActivateInput{
		AppKey:    product.AppKey,
		CardCode:  generated.Codes[0],
		BindMode:  domain.BindDevice,
		BindValue: "machine-A",
	})
	if err != nil {
		t.Fatalf("activate agent card: %v", err)
	}
	licenses, err := svc.ListLicenses(ctx)
	if err != nil {
		t.Fatalf("list licenses: %v", err)
	}
	var matched *domain.License
	for i := range licenses {
		if licenses[i].LicenseNo == activated.LicenseNo {
			matched = &licenses[i]
			break
		}
	}
	if matched == nil || matched.AgentID != agent.ID {
		t.Fatalf("expected activated license to inherit agent_id %q, got %#v", agent.ID, matched)
	}
}

func TestAgentCardGenerationRequiresPolicyAndQuota(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	agent, err := svc.CreateAgent(ctx, CreateAgentInput{Name: "华南代理"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	_, err = svc.AgentCreateCardBatch(ctx, AgentCreateCardBatchInput{
		AgentID:      agent.ID,
		ProductID:    product.ID,
		Name:         "agent batch",
		Quantity:     1,
		DurationDays: 30,
	})
	assertCode(t, err, "AGENT_PRODUCT_NOT_ALLOWED")

	if _, err := svc.UpsertAgentProductPolicy(ctx, AgentProductPolicyInput{
		AgentID:             agent.ID,
		ProductID:           product.ID,
		CanGenerate:         true,
		AllowedDurationDays: []int{30},
		MaxBatchQuantity:    10,
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	_, err = svc.AgentCreateCardBatch(ctx, AgentCreateCardBatchInput{
		AgentID:      agent.ID,
		ProductID:    product.ID,
		Name:         "agent batch",
		Quantity:     1,
		DurationDays: 90,
	})
	assertCode(t, err, "AGENT_DURATION_NOT_ALLOWED")

	_, err = svc.AgentCreateCardBatch(ctx, AgentCreateCardBatchInput{
		AgentID:      agent.ID,
		ProductID:    product.ID,
		Name:         "agent batch",
		Quantity:     1,
		DurationDays: 30,
	})
	assertCode(t, err, "AGENT_QUOTA_NOT_ENOUGH")
}

func TestAgentLoginIssuesAuthenticatableToken(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	agent, err := svc.CreateAgent(ctx, CreateAgentInput{Name: "华北代理"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	user, err := svc.CreateAgentUser(ctx, CreateAgentUserInput{
		AgentID:     agent.ID,
		Username:    "Owner",
		Password:    "secret123",
		DisplayName: "负责人",
	})
	if err != nil {
		t.Fatalf("create agent user: %v", err)
	}
	if user.Username != "owner" {
		t.Fatalf("expected normalized username owner, got %q", user.Username)
	}
	login, err := svc.AgentLogin(ctx, AgentLoginInput{
		AgentNo:  agent.AgentNo,
		Username: "owner",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("agent login: %v", err)
	}
	if login.AccessToken == "" || login.TokenType != "Bearer" {
		t.Fatalf("expected bearer token, got %#v", login)
	}
	session, err := svc.AuthenticateAgentToken(ctx, login.AccessToken)
	if err != nil {
		t.Fatalf("authenticate token: %v", err)
	}
	if session.AgentID != agent.ID || session.UserID != user.ID || session.Role != domain.AgentRoleOwner {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestAgentLoginRejectsBadPasswordAndExpiredToken(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	agent, err := svc.CreateAgent(ctx, CreateAgentInput{Name: "西南代理"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.CreateAgentUser(ctx, CreateAgentUserInput{
		AgentID:  agent.ID,
		Username: "owner",
		Password: "secret123",
	}); err != nil {
		t.Fatalf("create agent user: %v", err)
	}
	_, err = svc.AgentLogin(ctx, AgentLoginInput{
		AgentID:  agent.ID,
		Username: "owner",
		Password: "wrong-password",
	})
	assertCode(t, err, "INVALID_CREDENTIALS")

	login, err := svc.AgentLogin(ctx, AgentLoginInput{
		AgentID:  agent.ID,
		Username: "owner",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("agent login: %v", err)
	}
	svc.now = func() time.Time { return login.ExpiresAt.Add(time.Second) }
	_, err = svc.AuthenticateAgentToken(ctx, login.AccessToken)
	assertCode(t, err, "INVALID_AGENT_TOKEN")
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, "sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	if err := st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := New(st, []byte("test-card-pepper"), []byte("0123456789abcdef0123456789abcdef"))
	base := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }
	return svc
}

func createTestProduct(t *testing.T, ctx context.Context, svc *Service, conflictStrategy string) domain.Product {
	t.Helper()
	product, err := svc.CreateProduct(ctx, CreateProductInput{
		Name:                 "Test Product",
		Code:                 "YN",
		BindMode:             domain.BindDevice,
		MaxBindCount:         1,
		BindConflictStrategy: conflictStrategy,
		OfflineGraceDays:     15,
		ExpireGraceDays:      3,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	return product
}

func createTestCard(t *testing.T, ctx context.Context, svc *Service, productID string, days int) string {
	t.Helper()
	batch, err := svc.CreateCardBatch(ctx, CreateCardBatchInput{
		ProductID:    productID,
		Name:         "test batch",
		Quantity:     1,
		DurationDays: days,
	})
	if err != nil {
		t.Fatalf("create card batch: %v", err)
	}
	if len(batch.Codes) != 1 {
		t.Fatalf("expected one code, got %d", len(batch.Codes))
	}
	return batch.Codes[0]
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %s, got nil", code)
	}
	var appErr AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError %s, got %T %v", code, err, err)
	}
	if appErr.Code != code {
		t.Fatalf("expected error code %s, got %s", code, appErr.Code)
	}
}
