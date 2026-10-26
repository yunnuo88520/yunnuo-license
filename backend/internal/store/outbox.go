package store

import (
	"context"
	"time"
)

type Notification struct {
	ID, Payload, Kind string
	Attempts          int
}

func (s *Store) EnqueueNotification(ctx context.Context, id, dedup, payload, kind string, now time.Time) (bool, error) {
	prefix := "INSERT OR IGNORE"
	if s.driver == "mysql" {
		prefix = "INSERT IGNORE"
	}
	result, err := s.db.ExecContext(ctx, prefix+` INTO notification_outbox (id,dedup_key,payload_enc,kind,status,attempts,available_at,lease_until,last_error,created_at) VALUES (?,?,?,?,'pending',0,?,'','',?)`, id, dedup, payload, kind, ts(now), ts(now))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}
func (s *Store) ClaimNotification(ctx context.Context, now time.Time) (Notification, error) {
	var out Notification
	err := s.WithTx(ctx, func(tx *Tx) error {
		suffix := ""
		if tx.driver == "mysql" {
			suffix = " FOR UPDATE SKIP LOCKED"
		}
		err := tx.tx.QueryRowContext(ctx, `SELECT id,payload_enc,kind,attempts FROM notification_outbox WHERE (status IN ('pending','retry') AND available_at<=?) OR (status='sending' AND lease_until<=?) ORDER BY available_at,id LIMIT 1`+suffix, ts(now), ts(now)).Scan(&out.ID, &out.Payload, &out.Kind, &out.Attempts)
		if err != nil {
			return err
		}
		out.Attempts++
		_, err = tx.tx.ExecContext(ctx, `UPDATE notification_outbox SET status='sending',attempts=?,lease_until=? WHERE id=?`, out.Attempts, ts(now.Add(2*time.Minute)), out.ID)
		return err
	})
	return out, err
}
func (s *Store) FinishNotification(ctx context.Context, n Notification, status, code string, available time.Time) error {
	var completed any
	if status == "sent" || status == "dead" {
		completed = ts(time.Now().UTC())
	}
	_, err := s.db.ExecContext(ctx, `UPDATE notification_outbox SET status=?,last_error=?,available_at=?,lease_until='',completed_at=?,payload_enc=CASE WHEN ?='sent' THEN '' ELSE payload_enc END WHERE id=? AND status='sending' AND attempts=?`, status, code, ts(available), completed, status, n.ID, n.Attempts)
	return err
}
func (s *Store) NotificationStatus(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status,COUNT(*) FROM notification_outbox GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{"pending": 0, "retry": 0, "sending": 0, "sent": 0, "dead": 0}
	for rows.Next() {
		var key string
		var n int
		if err = rows.Scan(&key, &n); err != nil {
			return nil, err
		}
		out[key] = n
	}
	return out, rows.Err()
}
func (s *Store) RetryDeadNotifications(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notification_outbox SET status='retry',attempts=0,last_error='',available_at=?,completed_at=NULL WHERE status='dead'`, ts(now))
	return err
}
