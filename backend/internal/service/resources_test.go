package service

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReleaseQuotaConcurrentCommitAndSafeDeletion(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t).WithReleaseDir(t.TempDir())
	limits := DefaultResourceLimits()
	limits.ProductBytes = 10
	limits.TotalBytes = 10
	limits.ProductVersions = 2
	s.WithResourceLimits(limits)
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	upload := func(version string) (domain.ProductRelease, error) {
		return s.UploadRelease(ctx, UploadReleaseInput{ProductID: p.ID, Version: version, Filename: "app.zip"}, strings.NewReader("123456"), "admin")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	releases := make(chan domain.ProductRelease, 2)
	for _, version := range []string{"1.0.0", "2.0.0"} {
		wg.Add(1)
		go func(version string) {
			defer wg.Done()
			r, e := upload(version)
			if e == nil {
				releases <- r
			}
			results <- e
		}(version)
	}
	wg.Wait()
	close(results)
	close(releases)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else {
			assertCode(t, e, "STORAGE_QUOTA_EXCEEDED")
		}
	}
	if success != 1 {
		t.Fatal("quota was overspent", success)
	}
	r := <-releases
	s.ChangeReleaseStatus(ctx, p.ID, r.ID, "published", "admin")
	if e := s.DeleteRelease(ctx, p.ID, r.ID, "admin"); e == nil {
		t.Fatal("deleted a published package")
	} else {
		assertCode(t, e, "RELEASE_PUBLISHED")
	}
	s.ChangeReleaseStatus(ctx, p.ID, r.ID, "withdrawn", "admin")
	if e := s.DeleteRelease(ctx, p.ID, r.ID, "admin"); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(s.releaseDir, r.ID)); !os.IsNotExist(e) {
		t.Fatal("file retained", e)
	}
	if list, e := s.ListReleases(ctx, p.ID); e != nil || len(list) != 0 {
		t.Fatal(list, e)
	}
	_, e := upload(r.Version)
	assertCode(t, e, "RELEASE_EXISTS")
	if _, e = upload("3.0.0"); e != nil {
		t.Fatal("deletion did not free quota", e)
	}
}
func TestUploadSlotsAndTemporaryFileCleanup(t *testing.T) {
	s := newTestService(t).WithReleaseDir(t.TempDir())
	l := DefaultResourceLimits()
	l.ConcurrentUploads = 1
	s.WithResourceLimits(l)
	done, e := s.BeginReleaseUpload(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.BeginReleaseUpload(context.Background()); e == nil {
		t.Fatal("unlimited concurrent uploads")
	} else {
		assertCode(t, e, "UPLOAD_BUSY")
	}
	done()
	path := filepath.Join(s.releaseDir, ".upload-stale")
	os.WriteFile(path, []byte("temp"), 0600)
	old := s.now().Add(-25 * time.Hour)
	os.Chtimes(path, old, old)
	os.WriteFile(filepath.Join(s.releaseDir, "unrelated-file"), []byte("keep"), 0600)
	n, e := s.CleanupReleaseTemps(s.now())
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if _, e = os.Stat(filepath.Join(s.releaseDir, "unrelated-file")); e != nil {
		t.Fatal(e)
	}
}

func TestDownloadSlotsAreBoundedAndReleased(t *testing.T) {
	s := newTestService(t)
	limits := DefaultResourceLimits()
	limits.ConcurrentDownloads = 1
	s.WithResourceLimits(limits)
	release, err := s.BeginReleaseDownload(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginReleaseDownload(context.Background()); err == nil {
		t.Fatal("unbounded downloads")
	}
	release()
	release, err = s.BeginReleaseDownload(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}
