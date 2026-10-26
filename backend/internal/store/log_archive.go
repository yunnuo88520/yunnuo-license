package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ArchiveLogBatch holds selected old rows until an encrypted archive has been
// safely published. A failed export never deletes data. Financial ledgers,
// licenses and ownership history are deliberately outside this allow-list.
func (s *Store) ArchiveLogBatch(ctx context.Context, table string, before time.Time, limit int, publish func([]json.RawMessage) error) (int, error) {
	if table != "operation_logs" && table != "audit_logs" && table != "email_logs" {
		return 0, fmt.Errorf("unsupported log table")
	}
	if limit < 1 || limit > 5000 {
		return 0, fmt.Errorf("batch must contain 1..5000 records")
	}
	count := 0
	err := s.WithTx(ctx, func(tx *Tx) error {
		suffix := ""
		if tx.driver == "mysql" {
			suffix = " FOR UPDATE"
		}
		rows, err := tx.tx.QueryContext(ctx, `SELECT * FROM `+table+` WHERE created_at<? ORDER BY created_at,id LIMIT ?`+suffix, ts(before), limit)
		if err != nil {
			return err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return err
		}
		var records []json.RawMessage
		var ids []string
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err = rows.Scan(pointers...); err != nil {
				rows.Close()
				return err
			}
			record := map[string]any{}
			for i, key := range columns {
				if bytes, ok := values[i].([]byte); ok {
					values[i] = string(bytes)
				}
				record[key] = values[i]
			}
			id, ok := record["id"].(string)
			if !ok {
				rows.Close()
				return fmt.Errorf("unexpected log ID")
			}
			raw, e := json.Marshal(record)
			if e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
			records = append(records, raw)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err = publish(records); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err = tx.tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE id=? AND created_at<?`, id, ts(before)); err != nil {
				return err
			}
		}
		count = len(ids)
		return nil
	})
	return count, err
}
