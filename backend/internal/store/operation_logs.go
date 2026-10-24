package store

import (
	"context"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func (s *Store) BeginOperation(ctx context.Context, l domain.OperationLog) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO operation_logs (id,request_id,category,actor_type,actor_id,role,object_id,method,route,client_ip,http_status,result,error_code,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, l.ID, l.RequestID, l.Category, l.ActorType, l.ActorID, l.Role, l.ObjectID, l.Method, l.Route, l.ClientIP, l.HTTPStatus, l.Result, l.ErrorCode, ts(l.CreatedAt))
	return err
}
func (s *Store) FinishOperation(ctx context.Context, l domain.OperationLog) error {
	_, err := s.db.ExecContext(ctx, `UPDATE operation_logs SET actor_type=?,actor_id=?,role=?,object_id=?,http_status=?,result=?,error_code=? WHERE id=?`, l.ActorType, l.ActorID, l.Role, l.ObjectID, l.HTTPStatus, l.Result, l.ErrorCode, l.ID)
	return err
}
func (s *Store) Operations(ctx context.Context, category, actor, result, query, from, to string, page, size int) ([]domain.OperationLog, int, error) {
	where := []string{"1=1"}
	args := []any{}
	for _, p := range []struct{ col, value string }{{"category", category}, {"actor_type", actor}, {"result", result}} {
		if p.value != "" {
			where = append(where, p.col+"=?")
			args = append(args, p.value)
		}
	}
	if query != "" {
		where = append(where, "(actor_id LIKE ? OR object_id LIKE ? OR route LIKE ? OR request_id LIKE ? OR error_code LIKE ?)")
		for range 5 {
			args = append(args, "%"+query+"%")
		}
	}
	if from != "" {
		where = append(where, "created_at>=?")
		args = append(args, from)
	}
	if to != "" {
		where = append(where, "created_at<?")
		args = append(args, to)
	}
	clause := strings.Join(where, " AND ")
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_logs WHERE `+clause, args...).Scan(&count); err != nil {
		return nil, 0, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := s.db.QueryContext(ctx, `SELECT id,request_id,category,actor_type,actor_id,role,object_id,method,route,client_ip,http_status,result,error_code,created_at FROM operation_logs WHERE `+clause+` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []domain.OperationLog{}
	for rows.Next() {
		var l domain.OperationLog
		var created string
		if err := rows.Scan(&l.ID, &l.RequestID, &l.Category, &l.ActorType, &l.ActorID, &l.Role, &l.ObjectID, &l.Method, &l.Route, &l.ClientIP, &l.HTTPStatus, &l.Result, &l.ErrorCode, &created); err != nil {
			return nil, 0, err
		}
		l.CreatedAt = parseTS(created)
		items = append(items, l)
	}
	return items, count, rows.Err()
}

func (s *Store) ImportOperation(ctx context.Context, l domain.OperationLog) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_logs WHERE id=?`, l.ID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.BeginOperation(ctx, l)
}
