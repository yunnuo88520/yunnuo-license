package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"strings"
)

func readHistory(ctx context.Context, q commerceQuery, kind, scope, subject string, f domain.HistoryFilter, count bool) (domain.HistoryPage, error) {
	page := domain.HistoryPage{Items: []any{}, Page: f.Page, PageSize: f.PageSize}
	where := []string{"1=1"}
	args := []any{}
	base, columns, sort, stamp := "", "", "", ""
	search := []string{}
	switch kind {
	case "money":
		base = ` FROM agent_money_ledgers m`
		columns = `m.id,m.agent_id,m.request_id,m.kind,m.amount,m.balance_after,m.batch_id,m.package_id,m.unit_cost,m.quantity,m.remark,m.actor_id,m.created_at`
		sort = "m.created_at DESC,m.id DESC"
		stamp = "m.created_at"
		search = []string{"m.id", "m.agent_id", "m.batch_id", "m.package_id", "m.remark", "m.request_id"}
		if scope == "agent" {
			where = append(where, "m.agent_id=?")
			args = append(args, subject)
		} else if f.AgentID != "" {
			where = append(where, "m.agent_id=?")
			args = append(args, f.AgentID)
		}
		if f.Kind != "" {
			where = append(where, "m.kind=?")
			args = append(args, f.Kind)
		}
		if f.ProductID != "" {
			where = append(where, `m.package_id IN (SELECT id FROM card_packages WHERE product_id=?)`)
			args = append(args, f.ProductID)
		}
	case "cases":
		base = ` FROM security_cases c JOIN licenses l ON l.id=c.license_id`
		columns = caseColumns
		sort = "c.last_seen DESC,c.id DESC"
		stamp = "c.last_seen"
		search = []string{"c.id", "l.license_no", "c.subject", "c.evidence", "c.review_reason"}
		if scope == "customer" {
			where = append(where, `c.license_id IN (SELECT license_id FROM license_owners WHERE user_id=?)`)
			args = append(args, subject)
		}
		if f.Status != "" {
			where = append(where, "c.status=?")
			args = append(args, f.Status)
		}
		if f.Kind != "" {
			where = append(where, "c.signal_type=?")
			args = append(args, f.Kind)
		}
		if f.ProductID != "" {
			where = append(where, "c.product_id=?")
			args = append(args, f.ProductID)
		}
		if scope == "customer" {
			search = []string{"c.id", "l.license_no", "c.subject"}
		}
	case "appeals":
		base = ` FROM security_appeals a JOIN security_cases c ON c.id=a.case_id`
		columns = `a.id,a.case_id,a.user_id,a.reason,a.status,a.created_at`
		sort = "a.created_at DESC,a.id DESC"
		stamp = "a.created_at"
		search = []string{"a.id", "a.case_id", "a.reason"}
		if scope == "customer" {
			where = append(where, "a.user_id=?")
			args = append(args, subject)
		}
		if f.Status != "" {
			where = append(where, "a.status=?")
			args = append(args, f.Status)
		}
		if f.ProductID != "" {
			where = append(where, "c.product_id=?")
			args = append(args, f.ProductID)
		}
	default:
		return page, fmt.Errorf("invalid history kind")
	}
	if f.Query != "" {
		conditions := []string{}
		for _, col := range search {
			conditions = append(conditions, col+" LIKE ?")
			args = append(args, "%"+f.Query+"%")
		}
		where = append(where, "("+strings.Join(conditions, " OR ")+")")
	}
	if f.From != "" {
		where = append(where, historyTimeKey(stamp)+">=?")
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, historyTimeKey(stamp)+"<?")
		args = append(args, f.To)
	}
	sort = strings.Replace(sort, stamp, historyTimeKey(stamp), 1)
	clause := " WHERE " + strings.Join(where, " AND ")
	if count {
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*)"+base+clause, args...).Scan(&page.Total); err != nil {
			return page, err
		}
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := q.QueryContext(ctx, "SELECT "+columns+base+clause+" ORDER BY "+sort+" LIMIT ? OFFSET ?", args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		switch kind {
		case "money":
			v, e := readMoney(rows)
			if e != nil {
				return page, e
			}
			page.Items = append(page.Items, v)
		case "cases":
			v, e := readSecurityCase(rows)
			if e != nil {
				return page, e
			}
			page.Items = append(page.Items, v)
		case "appeals":
			v, e := readAppeal(rows)
			if e != nil {
				return page, e
			}
			page.Items = append(page.Items, v)
		}
	}
	return page, rows.Err()
}
func (s *Store) History(ctx context.Context, kind, scope, subject string, f domain.HistoryFilter) (domain.HistoryPage, error) {
	return readHistory(ctx, s.db, kind, scope, subject, f, true)
}

// Read a consistent snapshot in bounded batches. Callers spool output before sending it to clients.
func (s *Store) StreamHistory(ctx context.Context, kind, scope, subject string, f domain.HistoryFilter, write func(any) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	f.Page = 1
	f.PageSize = 500
	first, err := readHistory(ctx, tx, kind, scope, subject, f, true)
	if err != nil {
		return err
	}
	page := first
	for {
		for _, item := range page.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := write(item); err != nil {
				return err
			}
		}
		if f.Page*f.PageSize >= first.Total {
			break
		}
		f.Page++
		page, err = readHistory(ctx, tx, kind, scope, subject, f, false)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) ensureHistoryIndexes(ctx context.Context) error {
	for _, idx := range []struct{ name, table, cols string }{{"idx_money_agent_time", "agent_money_ledgers", "agent_id,created_at,id"}, {"idx_security_product_time", "security_cases", "product_id,last_seen,id"}, {"idx_security_status_time", "security_cases", "status,last_seen,id"}, {"idx_appeals_user_time", "security_appeals", "user_id,created_at,id"}} {
		if s.driver == "sqlite" {
			if _, err := s.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS "+idx.name+" ON "+idx.table+" ("+idx.cols+")"); err != nil {
				return err
			}
			continue
		}
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?`, idx.table, idx.name).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := s.db.ExecContext(ctx, "CREATE INDEX "+idx.name+" ON "+idx.table+" ("+idx.cols+")"); err != nil {
				return err
			}
		}
	}
	return nil
}

func historyTimeKey(col string) string {
	return `CONCAT(SUBSTR(` + col + `,1,19),'.',CASE WHEN SUBSTR(` + col + `,20,1)='.' THEN SUBSTR(CONCAT(REPLACE(SUBSTR(` + col + `,21),'Z',''),'000000000'),1,9) ELSE '000000000' END,'Z')`
}

// ensureAccountEmailColumns 幂等地为管理员与用户账号补充邮箱列
// （ALTER TABLE 无法在重复执行的迁移文件中使用，因此放到启动逻辑里按需执行）。
func (s *Store) ensureAccountEmailColumns(ctx context.Context) error {
	for _, table := range []string{"admin_users", "customer_users"} {
		exists := false
		if s.driver == "sqlite" {
			rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
			if err != nil {
				return err
			}
			for rows.Next() {
				var cid int
				var name, ctype string
				var notNull int
				var dfltValue any
				var pk int
				if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
					rows.Close()
					return err
				}
				if name == "email" {
					exists = true
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		} else {
			if err := s.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name='email'`,
				table).Scan(&exists); err != nil {
				return err
			}
		}
		if !exists {
			colType := "TEXT"
			if s.driver != "sqlite" {
				colType = "VARCHAR(254)"
			}
			if _, err := s.db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN email "+colType+" NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
	}
	return nil
}
