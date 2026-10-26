package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"log"
	"time"
)

type notificationPayload struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	AgentID string   `json:"agent_id"`
	HTML    string   `json:"html"`
}

func (s *Service) queueEmail(ctx context.Context, to []string, subject, kind, agentID, html, dedup string) (bool, error) {
	raw, err := json.Marshal(notificationPayload{to, subject, agentID, html})
	if err != nil {
		return false, err
	}
	encrypted, err := yncrypto.EncryptString(s.dataKey, string(raw))
	if err != nil {
		return false, err
	}
	id := yncrypto.NewID("notice")
	if dedup == "" {
		dedup = id
	} else {
		hash := sha256.Sum256([]byte(dedup))
		dedup = hex.EncodeToString(hash[:])
	}
	return s.currentStore().EnqueueNotification(ctx, id, dedup, encrypted, kind, time.Now().UTC())
}

// RunNotificationOutbox drains a bounded batch. Leases survive worker crashes;
// delivery is at least once, since SMTP cannot atomically commit with our DB.
func (s *Service) RunNotificationOutbox(ctx context.Context, limit int) (int, error) {
	sent := 0
	for i := 0; i < limit; i++ {
		if err := ctx.Err(); err != nil {
			return sent, err
		}
		n, err := s.currentStore().ClaimNotification(ctx, time.Now().UTC())
		if store.IsNotFound(err) {
			return sent, nil
		}
		if err != nil {
			return sent, err
		}
		raw, err := yncrypto.DecryptString(s.dataKey, n.Payload)
		var payload notificationPayload
		if err == nil {
			err = json.Unmarshal([]byte(raw), &payload)
		}
		code := "INVALID_PAYLOAD"
		if err == nil {
			var runtime *emailRuntime
			runtime, err = s.emailRuntime(ctx)
			code = "SMTP_UNAVAILABLE"
			if err == nil && runtime.usable() {
				err = s.deliverEmail(ctx, runtime, payload.To, payload.Subject, n.Kind, payload.AgentID, payload.HTML)
				code = "SMTP_DELIVERY_FAILED"
			} else if err == nil {
				err = ErrPermissionDenied
			}
		}
		status := "sent"
		next := time.Now().UTC()
		if err != nil {
			status = "retry"
			delay := time.Minute * time.Duration(1<<min(n.Attempts-1, 10))
			next = next.Add(delay)
			if n.Attempts >= 12 || code == "INVALID_PAYLOAD" {
				status = "dead"
			}
		} else {
			code = ""
			sent++
		}
		if err = s.currentStore().FinishNotification(ctx, n, status, code, next); err != nil {
			return sent, err
		}
	}
	return sent, nil
}
func (s *Service) StartNotificationWorker() {
	s.notificationOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.stopNotifications = cancel
		s.notificationDone = make(chan struct{})
		go func() {
			defer close(s.notificationDone)
			ticker := time.NewTicker(5 * time.Second)
			lastCleanup := time.Now().Add(-2 * time.Hour)
			defer ticker.Stop()
			for {
				if time.Since(lastCleanup) > time.Hour {
					if _, err := s.CleanupReleaseTemps(time.Now().UTC()); err != nil {
						log.Printf("release temporary cleanup failed: %v", err)
					}
					lastCleanup = time.Now()
				}
				if _, err := s.RunNotificationOutbox(ctx, 20); err != nil && ctx.Err() == nil {
					log.Printf("notification outbox batch failed: %v", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				case <-s.notificationWake:
				}
			}
		}()
	})
	select {
	case s.notificationWake <- struct{}{}:
	default:
	}
}
func (s *Service) stopNotificationWorker() {
	if s.stopNotifications != nil {
		s.stopNotifications()
		<-s.notificationDone
	}
}
func (s *Service) NotificationStatus(ctx context.Context) (map[string]int, error) {
	return s.currentStore().NotificationStatus(ctx)
}
func (s *Service) RetryDeadNotifications(ctx context.Context, actor string) error {
	if err := s.currentStore().RetryDeadNotifications(ctx, time.Now().UTC()); err != nil {
		return err
	}
	s.StartNotificationWorker()
	return s.audit(ctx, "admin", actor, "", "", "", "notification.retry", "success", "")
}
