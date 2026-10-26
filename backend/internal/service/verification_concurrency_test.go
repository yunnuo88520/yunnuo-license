package service

import (
	"context"
	"sync"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func TestConcurrentLicenseVerificationAndAuditWrites(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	card := createTestCard(t, ctx, s, p.ID, 30)
	lic, err := s.Activate(ctx, ActivateInput{AppKey: p.AppKey, CardCode: card, BindMode: domain.BindDevice, BindValue: "concurrent-client"})
	if err != nil {
		t.Fatal(err)
	}
	in := VerifyInput{AppKey: p.AppKey, LicenseNo: lic.LicenseNo, BindMode: domain.BindDevice, BindValue: "concurrent-client"}
	start := make(chan struct{})
	results := make(chan error, 100)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for n := 0; n < 10; n++ {
				_, e := s.Verify(ctx, in)
				results <- e
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for n := 0; n < 20; n++ {
			results <- s.auditClient(ctx, p.ID, "", "", "test.concurrent.audit", "success", "", "", "")
		}
	}()
	close(start)
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatalf("valid license failed during concurrent audit writes: %v", e)
		}
	}
}
