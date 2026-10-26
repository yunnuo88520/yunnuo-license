package store

import (
	"context"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"time"
)

// CheckReadiness tests the durable audit write required by business requests.
// The transaction is always rolled back and never pollutes the audit trail.
func (s *Store) CheckReadiness(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO operation_logs(id,request_id,category,actor_type,actor_id,role,object_id,method,route,client_ip,http_status,result,error_code,created_at) VALUES(?,?,'operations','system','','','','PROBE','/readyz','',0,'pending','',?)`, yncrypto.NewID("probe"), yncrypto.NewID("probe"), ts(time.Now()))
	return err
}
