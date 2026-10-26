package service

import (
	"context"
	"database/sql"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOutboxRetriesPersistAcrossServiceRestartWithoutDuplicateEnqueue(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dsn := "file:" + filepath.Join(dir, "outbox.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	st, err := store.Open(ctx, "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Migrate(ctx, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("k", 32))
	s := New(st, []byte("test-pepper"), key)
	for i := 0; i < 2; i++ {
		queued, e := s.queueEmail(ctx, []string{"private@example.com"}, "Notice", "expiry", "", "<p>Restored notice</p>", "same-event")
		if e != nil || queued != (i == 0) {
			t.Fatal(queued, e)
		}
	}
	if n, e := s.RunNotificationOutbox(ctx, 5); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	status, _ := s.NotificationStatus(ctx)
	if status["retry"] != 1 {
		t.Fatal(status)
	}
	db, e := sql.Open("sqlite", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var encrypted string
	db.QueryRow(`SELECT payload_enc FROM notification_outbox`).Scan(&encrypted)
	if strings.Contains(encrypted, "private@example.com") {
		t.Fatal("PII persisted unencrypted")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(ctx, "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	s = New(st, []byte("test-pepper"), key)
	defer s.Close()
	smtp := startFakeSMTPServer(t)
	if _, err = s.UpdateEmailSettings(ctx, UpdateEmailSettingsInput{Enabled: true, SMTPHost: "127.0.0.1", SMTPPort: smtp.port(), FromAddress: "notify@example.com", Encryption: "none"}); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE notification_outbox SET available_at=?`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano))
	if n, e := s.RunNotificationOutbox(ctx, 5); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if smtp.countBody("Restored notice") != 1 {
		t.Fatal("not delivered after restart")
	}
	status, _ = s.NotificationStatus(ctx)
	if status["sent"] != 1 {
		t.Fatal(status)
	}
	if n, e := s.RunNotificationOutbox(ctx, 5); e != nil || n != 0 {
		t.Fatal("duplicate delivery", n, e)
	}
	db.QueryRow(`SELECT payload_enc FROM notification_outbox`).Scan(&encrypted)
	if encrypted != "" {
		t.Fatal("completed payload retained")
	}
}
