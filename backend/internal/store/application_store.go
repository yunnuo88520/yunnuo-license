package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
)

type applicationRegistry struct{ store *Store }

func (s *Store) LicensingRegistry() core.Registry { return &applicationRegistry{s} }

type privateRecord struct {
	Data    json.RawMessage   `json:"record"`
	Private map[string]string `json:"private,omitempty"`
}

func marshalCoreRecord(value any) ([]byte, error) {
	data, e := json.Marshal(value)
	if e != nil {
		return nil, e
	}
	private := map[string]string{}
	switch v := value.(type) {
	case core.Application:
		private["secret_hash"] = v.SecretHash
	case core.SigningKey:
		private["private_cipher"] = v.PrivateCipher
	case core.CardKey:
		private["key_hash"] = v.KeyHash
	case core.Invitation:
		private["token_hash"] = v.TokenHash
	case core.SalesChannel:
		private["secret_cipher"] = v.SecretCipher
	case core.Release:
		private["storage_path"] = v.StoragePath
	}
	return json.Marshal(privateRecord{data, private})
}
func unmarshalCoreRecord[T any](raw string) (T, error) {
	var result T
	var envelope privateRecord
	if e := json.Unmarshal([]byte(raw), &envelope); e != nil {
		return result, e
	}
	decoder := json.NewDecoder(strings.NewReader(string(envelope.Data)))
	decoder.UseNumber()
	if e := decoder.Decode(&result); e != nil {
		return result, e
	}
	switch v := any(&result).(type) {
	case *core.Application:
		v.SecretHash = envelope.Private["secret_hash"]
	case *core.SigningKey:
		v.PrivateCipher = envelope.Private["private_cipher"]
	case *core.CardKey:
		v.KeyHash = envelope.Private["key_hash"]
	case *core.Invitation:
		v.TokenHash = envelope.Private["token_hash"]
	case *core.SalesChannel:
		v.SecretCipher = envelope.Private["secret_cipher"]
	case *core.Release:
		v.StoragePath = envelope.Private["storage_path"]
	}
	return result, nil
}
func (r *applicationRegistry) GetApplication(ctx context.Context, id string) (core.Application, error) {
	var raw string
	e := r.store.db.QueryRowContext(ctx, "SELECT data_json FROM applications WHERE application_id=?", id).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return core.Application{}, core.ErrNotFound
	}
	if e != nil {
		return core.Application{}, e
	}
	a, e := unmarshalCoreRecord[core.Application](raw)
	if e == nil && a.ApplicationID != id {
		return core.Application{}, core.ErrScopeMismatch
	}
	return a, e
}
func (r *applicationRegistry) ListApplications(ctx context.Context, developer string) ([]core.Application, error) {
	query := "SELECT data_json FROM applications"
	var args []any
	if developer != "" {
		query += " WHERE owner_developer_id=?"
		args = append(args, developer)
	}
	query += " ORDER BY created_at DESC,application_id"
	rows, e := r.store.db.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := []core.Application{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		a, e := unmarshalCoreRecord[core.Application](raw)
		if e != nil {
			return nil, e
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
func (r *applicationRegistry) Scoped(ctx context.Context) (core.Repository, error) {
	scope, e := core.ScopeFrom(ctx)
	if e != nil {
		return nil, e
	}
	return &scopedApplicationRepository{store: r.store, scope: scope}, nil
}
func (r *applicationRegistry) CreateApplication(ctx context.Context, a core.Application, key core.SigningKey, event core.AuditEvent, sign core.AuditSigner) error {
	if a.ApplicationID == "" || a.ID != a.ApplicationID || event.Actor.Kind == "" {
		return core.ErrScopeRequired
	}
	scoped, e := core.BindScope(ctx, a.ApplicationID, event.Actor, true)
	if e != nil {
		return e
	}
	scope, _ := core.ScopeFrom(scoped)
	return (&scopedApplicationRepository{store: r.store, scope: scope, creating: true}).transaction(scoped, func(tx *applicationTransaction) error {
		raw, e := marshalCoreRecord(a)
		if e != nil {
			return e
		}
		if _, e = tx.runner.ExecContext(scoped, "INSERT INTO applications(application_id,name,owner_developer_id,status,app_secret_hash,data_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", a.ApplicationID, a.Name, a.DeveloperID, a.Status, a.SecretHash, string(raw), coreTS(a.CreatedAt), coreTS(a.UpdatedAt)); e != nil {
			return e
		}
		tx.mutations++
		if _, e = tx.runner.ExecContext(scoped, "INSERT INTO application_audit_heads(application_id,sequence,event_hash) VALUES(?,0,'')", a.ApplicationID); e != nil {
			return e
		}
		if e = tx.Keys().Insert(scoped, key); e != nil {
			return e
		}
		return tx.AppendAudit(scoped, event, sign)
	})
}

type coreRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type scopedApplicationRepository struct {
	store    *Store
	scope    core.Scope
	creating bool
}

func (r *scopedApplicationRepository) check(ctx context.Context, write bool) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	scope, e := core.ScopeFrom(ctx)
	if e != nil {
		return e
	}
	a, b := scope.Actor(), r.scope.Actor()
	if scope.ApplicationID() != r.scope.ApplicationID() || a.Kind != b.Kind || a.ID != b.ID || a.Role != b.Role {
		return core.ErrScopeMismatch
	}
	if write && (!scope.Writable() || !r.scope.Writable()) {
		return core.ErrReadOnly
	}
	return nil
}
func (r *scopedApplicationRepository) View(ctx context.Context, fn func(core.View) error) error {
	if e := r.check(ctx, false); e != nil {
		return e
	}
	options := &sql.TxOptions{ReadOnly: true}
	if r.store.driver == "mysql" {
		options.Isolation = sql.LevelRepeatableRead
	}
	tx, e := r.store.db.BeginTx(ctx, options)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	unit := &applicationTransaction{repo: r, runner: tx}
	if _, e = unit.Application(ctx); e != nil {
		return e
	}
	if e = fn(&applicationView{base: unit}); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *scopedApplicationRepository) Update(ctx context.Context, fn func(core.Transaction) error) error {
	return r.transaction(ctx, func(tx *applicationTransaction) error { return fn(tx) })
}
func (r *scopedApplicationRepository) transaction(ctx context.Context, fn func(*applicationTransaction) error) error {
	if e := r.check(ctx, true); e != nil {
		return e
	}
	var options *sql.TxOptions
	if r.store.driver == "mysql" {
		options = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	tx, e := r.store.db.BeginTx(ctx, options)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	unit := &applicationTransaction{repo: r, runner: tx, writable: true}
	if !r.creating {
		if e = unit.LockApplication(ctx); e != nil {
			return e
		}
		if _, e = unit.Application(ctx); e != nil {
			return e
		}
	}
	if e = fn(unit); e != nil {
		return e
	}
	if unit.mutations > 0 && unit.audits == 0 {
		return core.ErrAuditRequired
	}
	return tx.Commit()
}

type applicationTransaction struct {
	repo              *scopedApplicationRepository
	runner            coreRunner
	writable          bool
	mutations, audits int
}
type applicationView struct{ base *applicationTransaction }

func (t *applicationTransaction) Application(ctx context.Context) (core.Application, error) {
	if e := t.repo.check(ctx, false); e != nil {
		return core.Application{}, e
	}
	var raw string
	e := t.runner.QueryRowContext(ctx, "SELECT data_json FROM applications WHERE application_id=?", t.repo.scope.ApplicationID()).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return core.Application{}, core.ErrNotFound
	}
	if e != nil {
		return core.Application{}, e
	}
	a, e := unmarshalCoreRecord[core.Application](raw)
	if a.ApplicationID != t.repo.scope.ApplicationID() {
		return core.Application{}, core.ErrScopeMismatch
	}
	return a, e
}
func (v *applicationView) Application(ctx context.Context) (core.Application, error) {
	return v.base.Application(ctx)
}
func (t *applicationTransaction) LockApplication(ctx context.Context) error {
	if e := t.checkWrite(ctx); e != nil {
		return e
	}
	_, e := t.runner.ExecContext(ctx, "UPDATE applications SET updated_at=updated_at WHERE application_id=?", t.repo.scope.ApplicationID())
	return e
}
func (t *applicationTransaction) SaveApplication(ctx context.Context, a core.Application) error {
	if e := t.checkWrite(ctx); e != nil {
		return e
	}
	if a.ApplicationID != t.repo.scope.ApplicationID() || a.ID != a.ApplicationID {
		return core.ErrScopeMismatch
	}
	raw, e := marshalCoreRecord(a)
	if e != nil {
		return e
	}
	_, e = t.runner.ExecContext(ctx, "UPDATE applications SET name=?,owner_developer_id=?,status=?,app_secret_hash=?,data_json=?,updated_at=? WHERE application_id=?", a.Name, a.DeveloperID, a.Status, a.SecretHash, string(raw), coreTS(a.UpdatedAt), a.ApplicationID)
	if e == nil {
		t.mutations++
	}
	return e
}
func (t *applicationTransaction) checkWrite(ctx context.Context) error {
	if !t.writable {
		return core.ErrReadOnly
	}
	return t.repo.check(ctx, true)
}
func coreTS(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }
func optionalString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func optionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return coreTS(*t)
}

type coreTableSpec struct {
	name     string
	fields   map[string]string
	optional map[string]bool
}

func tableSpec(name string, fields ...string) coreTableSpec {
	spec := coreTableSpec{name: name, fields: map[string]string{}, optional: map[string]bool{}}
	for _, field := range fields {
		parts := strings.Split(field, ":")
		column, member := parts[0], parts[1]
		if strings.HasSuffix(member, "?") {
			member = strings.TrimSuffix(member, "?")
			spec.optional[column] = true
		}
		spec.fields[column] = member
	}
	return spec
}

var coreSpecs = map[string]coreTableSpec{
	"keys":        tableSpec("application_signing_keys", "kid:KID", "key_version:Version", "status:Status"),
	"plans":       tableSpec("application_plans", "name:Name", "model:Terms.Model", "enabled:Enabled"),
	"licenses":    tableSpec("application_licenses", "plan_id:PlanID", "customer_ref:CustomerRef", "owner_user_id:OwnerUserID?", "order_id:OrderID?", "status:Status", "binding_type:BindingType", "binding_hash:BindingHash", "expires_at:ExpiresAt?", "renewal_only:RenewalOnly"),
	"activations": tableSpec("application_activations", "license_id:LicenseID", "public_key_hash:PublicKeyHash", "session_id:SessionID", "binding_type:BindingType", "binding_hash:BindingHash", "lease_expires_at:LeaseExpiresAt?", "deactivated_at:DeactivatedAt?", "borrowed:Borrowed"),
	"cards":       tableSpec("application_card_keys", "plan_id:PlanID", "batch_id:BatchID", "key_hash:KeyHash", "status:Status", "bound_license_id:BoundLicenseID", "redeemed_license_id:RedeemedLicenseID?"),
	"rules":       tableSpec("application_binding_rules", "kind:Kind", "enabled:Enabled", "license_id:LicenseID?"),
	"anchors":     tableSpec("application_binding_anchors", "license_id:LicenseID", "group_id:GroupID", "kind:Kind", "binding_hash:BindingHash", "status:Status"),
	"invitations": tableSpec("application_installation_invitations", "license_id:LicenseID", "kind:Kind", "token_hash:TokenHash", "owner_user_id:OwnerUserID?", "expires_at:ExpiresAt", "used_at:UsedAt?"),
	"pending":     tableSpec("application_pending_installations", "license_id:LicenseID?", "public_key_hash:PublicKeyHash", "binding_type:BindingType", "binding_hash:BindingHash", "status:Status", "expires_at:ExpiresAt"),
	"nonces":      tableSpec("application_request_nonces", "credential_id:CredentialID", "nonce_hash:NonceHash", "expires_at:ExpiresAt"),
	"channels":    tableSpec("application_sales_channels", "kind:Kind", "enabled:Enabled"),
	"orders":      tableSpec("application_sales_orders", "plan_id:PlanID", "channel_id:ChannelID?", "external_id:ExternalID", "customer_ref:CustomerRef", "owner_user_id:OwnerUserID?", "agent_id:AgentID?", "status:Status"),
	"webhooks":    tableSpec("application_webhook_receipts", "channel_id:ChannelID", "external_id:ExternalID", "order_id:OrderID"),
	"usage":       tableSpec("application_usage_receipts", "license_id:LicenseID", "activation_id:ActivationID", "idempotency_key:IdempotencyKey", "period:Period"),
	"agents":      tableSpec("application_agent_grants", "agent_id:AgentID", "plan_id:PlanID", "enabled:Enabled"),
	"money":       tableSpec("application_money_events", "agent_id:AgentID", "order_id:OrderID", "request_id:RequestID", "kind:Kind"),
	"releases":    tableSpec("application_releases", "version:Version", "target_key:TargetKey", "status:Status", "size:Size"),
	"audit":       tableSpec("application_audit_events", "sequence:Sequence", "event_type:EventType", "reason_code:ReasonCode", "entity_id:EntityID", "result:Result"),
}

type coreReader[T core.Entity] struct {
	tx   *applicationTransaction
	spec coreTableSpec
}
type coreWriter[T core.Entity] struct{ coreReader[T] }

func reader[T core.Entity](tx *applicationTransaction, name string) core.ReadTable[T] {
	return &coreReader[T]{tx, coreSpecs[name]}
}
func writer[T core.Entity](tx *applicationTransaction, name string) core.WriteTable[T] {
	return &coreWriter[T]{coreReader[T]{tx, coreSpecs[name]}}
}
func (r *coreReader[T]) where(ctx context.Context, f core.Filter) (string, []any, error) {
	if e := r.tx.repo.check(ctx, false); e != nil {
		return "", nil, e
	}
	clauses := []string{"application_id=?"}
	args := []any{r.tx.repo.scope.ApplicationID()}
	fields := map[string]string{"bound_license_id": f.BoundLicenseID, "redeemed_license_id": f.RedeemedLicenseID, "id": f.ID, "status": f.Status, "plan_id": f.PlanID, "license_id": f.LicenseID, "customer_ref": f.CustomerRef, "owner_user_id": f.OwnerUserID, "batch_id": f.BatchID, "key_hash": f.KeyHash, "binding_type": f.BindingType, "binding_hash": f.BindingHash, "public_key_hash": f.PublicKeyHash, "session_id": f.SessionID, "kind": f.Kind, "group_id": f.GroupID, "channel_id": f.ChannelID, "external_id": f.ExternalID, "agent_id": f.AgentID, "nonce_hash": f.NonceHash, "credential_id": f.CredentialID, "idempotency_key": f.IdempotencyKey, "period": f.Period, "event_type": f.EventType, "result": f.Result, "reason_code": f.ReasonCode, "request_id": f.RequestID}
	names := []string{}
	for name, value := range fields {
		if value != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if name != "id" && r.spec.fields[name] == "" {
			return "", nil, fmt.Errorf("licensing: unsupported %s filter", name)
		}
		clauses = append(clauses, name+"=?")
		args = append(args, fields[name])
	}
	if f.ExcludeRenewals {
		if r.spec.fields["renewal_only"] == "" {
			return "", nil, errors.New("licensing: renewal filter requires licenses")
		}
		clauses = append(clauses, "renewal_only=0")
	}
	if f.ExactSessionID != nil {
		if r.spec.fields["session_id"] == "" {
			return "", nil, errors.New("licensing: session filter requires activations")
		}
		clauses = append(clauses, "session_id=?")
		args = append(args, *f.ExactSessionID)
	}
	if f.Borrowed != nil {
		if r.spec.fields["borrowed"] == "" {
			return "", nil, errors.New("licensing: borrowed filter requires activations")
		}
		clauses = append(clauses, "borrowed=?")
		args = append(args, *f.Borrowed)
	}
	if f.ActiveAt != nil {
		if r.spec.fields["deactivated_at"] == "" {
			return "", nil, errors.New("licensing: active filter requires activations")
		}
		clauses = append(clauses, "deactivated_at IS NULL")
		if f.LeaseOnly {
			clauses = append(clauses, "lease_expires_at>?")
		} else {
			clauses = append(clauses, "(lease_expires_at IS NULL OR lease_expires_at>?)")
		}
		args = append(args, coreTS(*f.ActiveAt))
	}
	if f.OnlyGlobal {
		if r.spec.fields["license_id"] == "" {
			return "", nil, errors.New("licensing: global filter requires rules")
		}
		clauses = append(clauses, "license_id IS NULL")
	}
	if f.ExpiresAfter != nil {
		if r.spec.fields["expires_at"] == "" {
			return "", nil, errors.New("licensing: expiry filter unavailable")
		}
		clauses = append(clauses, "expires_at>=?")
		args = append(args, coreTS(*f.ExpiresAfter))
	}
	if f.Before != nil {
		if r.spec.fields["expires_at"] == "" {
			return "", nil, errors.New("licensing: expiry filter unavailable")
		}
		clauses = append(clauses, "expires_at IS NOT NULL AND expires_at<?")
		args = append(args, coreTS(*f.Before))
	}
	if f.From != nil {
		clauses = append(clauses, "created_at>=?")
		args = append(args, coreTS(*f.From))
	}
	return strings.Join(clauses, " AND "), args, nil
}
func (r *coreReader[T]) Get(ctx context.Context, id string) (T, error) {
	var zero T
	if id == "" {
		return zero, core.ErrNotFound
	}
	items, e := r.Find(ctx, core.Filter{ID: id, Limit: 1})
	if e != nil {
		return zero, e
	}
	if len(items) == 0 {
		return zero, core.ErrNotFound
	}
	return items[0], nil
}
func (r *coreReader[T]) Find(ctx context.Context, f core.Filter) ([]T, error) {
	where, args, e := r.where(ctx, f)
	if e != nil {
		return nil, e
	}
	limit := f.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 10000 || f.Offset < 0 {
		return nil, errors.New("licensing: invalid pagination")
	}
	order := "created_at DESC,id"
	if f.Ascending {
		order = "created_at,id"
	}
	if r.spec.name == "application_audit_events" {
		order = "sequence DESC"
		if f.Ascending {
			order = "sequence"
		}
	}
	args = append(args, limit, f.Offset)
	rows, e := r.tx.runner.QueryContext(ctx, "SELECT id,data_json FROM "+r.spec.name+" WHERE "+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := []T{}
	for rows.Next() {
		var id, raw string
		if e = rows.Scan(&id, &raw); e != nil {
			return nil, e
		}
		v, e := unmarshalCoreRecord[T](raw)
		if e != nil {
			return nil, e
		}
		record := v.Identity()
		if record.ApplicationID != r.tx.repo.scope.ApplicationID() || record.ID != id {
			return nil, core.ErrScopeMismatch
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func (r *coreReader[T]) Count(ctx context.Context, f core.Filter) (int64, error) {
	where, args, e := r.where(ctx, f)
	if e != nil {
		return 0, e
	}
	var count int64
	e = r.tx.runner.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+r.spec.name+" WHERE "+where, args...).Scan(&count)
	return count, e
}
func (w *coreWriter[T]) prepare(ctx context.Context, v T) (T, error) {
	if e := w.tx.checkWrite(ctx); e != nil {
		return v, e
	}
	record := v.Identity()
	if record.ApplicationID != "" && record.ApplicationID != w.tx.repo.scope.ApplicationID() {
		return v, core.ErrScopeMismatch
	}
	if record.ID == "" {
		return v, errors.New("licensing: record ID required")
	}
	record.ApplicationID = w.tx.repo.scope.ApplicationID()
	reflect.ValueOf(&v).Elem().FieldByName("Record").Set(reflect.ValueOf(record))
	return v, nil
}
func coreProjection(value any, member string, optional bool) any {
	v := reflect.ValueOf(value)
	for _, name := range strings.Split(member, ".") {
		v = v.FieldByName(name)
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if t, ok := v.Interface().(time.Time); ok {
		return coreTS(t)
	}
	if optional && v.Kind() == reflect.String {
		return optionalString(v.String())
	}
	return v.Interface()
}
func (w *coreWriter[T]) Insert(ctx context.Context, value T) error {
	v, e := w.prepare(ctx, value)
	if e != nil {
		return e
	}
	raw, e := marshalCoreRecord(v)
	if e != nil {
		return e
	}
	record := v.Identity()
	names := []string{}
	for name := range w.spec.fields {
		names = append(names, name)
	}
	sort.Strings(names)
	columns := []string{"application_id", "id", "data_json", "created_at", "updated_at"}
	args := []any{record.ApplicationID, record.ID, string(raw), coreTS(record.CreatedAt), coreTS(record.UpdatedAt)}
	for _, name := range names {
		columns = append(columns, name)
		args = append(args, coreProjection(v, w.spec.fields[name], w.spec.optional[name]))
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	_, e = w.tx.runner.ExecContext(ctx, "INSERT INTO "+w.spec.name+" ("+strings.Join(columns, ",")+") VALUES ("+marks+")", args...)
	if e == nil {
		w.tx.mutations++
	}
	return e
}
func (w *coreWriter[T]) Save(ctx context.Context, value T) error {
	v, e := w.prepare(ctx, value)
	if e != nil {
		return e
	}
	if _, e = w.Get(ctx, v.Identity().ID); e != nil {
		return e
	}
	raw, e := marshalCoreRecord(v)
	if e != nil {
		return e
	}
	names := []string{}
	for name := range w.spec.fields {
		names = append(names, name)
	}
	sort.Strings(names)
	sets := []string{"data_json=?", "updated_at=?"}
	args := []any{string(raw), coreTS(v.Identity().UpdatedAt)}
	for _, name := range names {
		sets = append(sets, name+"=?")
		args = append(args, coreProjection(v, w.spec.fields[name], w.spec.optional[name]))
	}
	args = append(args, w.tx.repo.scope.ApplicationID(), v.Identity().ID)
	_, e = w.tx.runner.ExecContext(ctx, "UPDATE "+w.spec.name+" SET "+strings.Join(sets, ",")+" WHERE application_id=? AND id=?", args...)
	if e == nil {
		w.tx.mutations++
	}
	return e
}
func (w *coreWriter[T]) Delete(ctx context.Context, id string) error {
	if e := w.tx.checkWrite(ctx); e != nil {
		return e
	}
	if _, e := w.Get(ctx, id); e != nil {
		return e
	}
	_, e := w.tx.runner.ExecContext(ctx, "DELETE FROM "+w.spec.name+" WHERE application_id=? AND id=?", w.tx.repo.scope.ApplicationID(), id)
	if e == nil {
		w.tx.mutations++
	}
	return e
}
func (w *coreWriter[T]) Lock(ctx context.Context, id string) error {
	if e := w.tx.checkWrite(ctx); e != nil {
		return e
	}
	if _, e := w.tx.runner.ExecContext(ctx, "UPDATE "+w.spec.name+" SET updated_at=updated_at WHERE application_id=? AND id=?", w.tx.repo.scope.ApplicationID(), id); e != nil {
		return e
	}
	_, e := w.Get(ctx, id)
	return e
}

func (t *applicationTransaction) AppendAudit(ctx context.Context, event core.AuditEvent, sign core.AuditSigner) error {
	if e := t.checkWrite(ctx); e != nil {
		return e
	}
	if sign == nil {
		return core.ErrAuditRequired
	}
	app := t.repo.scope.ApplicationID()
	if event.ApplicationID != "" && event.ApplicationID != app {
		return core.ErrScopeMismatch
	}
	if _, e := t.runner.ExecContext(ctx, "UPDATE application_audit_heads SET sequence=sequence WHERE application_id=?", app); e != nil {
		return e
	}
	var head core.AuditHead
	if e := t.runner.QueryRowContext(ctx, "SELECT sequence,event_hash FROM application_audit_heads WHERE application_id=?", app).Scan(&head.Sequence, &head.Hash); e != nil {
		return e
	}
	event.ApplicationID = app
	event.Sequence = head.Sequence + 1
	event.PreviousHash = head.Hash
	event.Actor = t.repo.scope.Actor()
	if event.ID == "" {
		event.ID = "event_" + strconv.FormatInt(event.Sequence, 10)
	}
	hash, e := sign(event)
	if e != nil {
		return e
	}
	event.EventHash = hash
	// No public write table is exposed for audit events. Database triggers also
	// reject updates/deletes, including direct SQL outside this adapter.
	internal := &coreWriter[core.AuditEvent]{coreReader[core.AuditEvent]{t, coreSpecs["audit"]}}
	if e = internal.Insert(ctx, event); e != nil {
		return e
	}
	if _, e = t.runner.ExecContext(ctx, "UPDATE application_audit_heads SET sequence=?,event_hash=? WHERE application_id=?", event.Sequence, hash, app); e != nil {
		return e
	}
	t.audits++
	return nil
}
func (v *applicationView) AuditHead(ctx context.Context) (core.AuditHead, error) {
	if e := v.base.repo.check(ctx, false); e != nil {
		return core.AuditHead{}, e
	}
	var h core.AuditHead
	e := v.base.runner.QueryRowContext(ctx, "SELECT sequence,event_hash FROM application_audit_heads WHERE application_id=?", v.base.repo.scope.ApplicationID()).Scan(&h.Sequence, &h.Hash)
	return h, e
}
func (v *applicationView) Diagnostics(ctx context.Context, now time.Time) (core.Diagnostics, error) {
	var d core.Diagnostics
	var e error
	d.VerifyFailures = map[string]int64{}
	if d.LicenseTotal, e = v.Licenses().Count(ctx, core.Filter{ExcludeRenewals: true}); e != nil {
		return d, e
	}
	if d.ActiveActivations, e = v.Activations().Count(ctx, core.Filter{ActiveAt: &now}); e != nil {
		return d, e
	}
	deadline := now.AddDate(0, 0, 7)
	if d.ExpiringLicenses, e = v.Licenses().Count(ctx, core.Filter{Status: "active", ExcludeRenewals: true, Before: &deadline, ExpiresAfter: &now}); e != nil {
		return d, e
	}
	table := &coreReader[core.AuditEvent]{v.base, coreSpecs["audit"]}
	where, args, e := table.where(ctx, core.Filter{Result: "failed"})
	if e != nil {
		return d, e
	}
	rows, e := v.base.runner.QueryContext(ctx, "SELECT reason_code,COUNT(*) FROM application_audit_events WHERE "+where+" AND event_type IN ('verified','activated','request_authentication','update_authorization') GROUP BY reason_code", args...)
	if e != nil {
		return d, e
	}
	defer rows.Close()
	for rows.Next() {
		var reason string
		var count int64
		if e = rows.Scan(&reason, &count); e != nil {
			return d, e
		}
		d.VerifyFailures[reason] = count
	}
	return d, rows.Err()
}

func (v *applicationView) Keys() core.ReadTable[core.SigningKey] {
	return reader[core.SigningKey](v.base, "keys")
}
func (t *applicationTransaction) Keys() core.WriteTable[core.SigningKey] {
	return writer[core.SigningKey](t, "keys")
}

func (v *applicationView) Plans() core.ReadTable[core.Plan] {
	return reader[core.Plan](v.base, "plans")
}
func (t *applicationTransaction) Plans() core.WriteTable[core.Plan] {
	return writer[core.Plan](t, "plans")
}

func (v *applicationView) Licenses() core.ReadTable[core.License] {
	return reader[core.License](v.base, "licenses")
}
func (t *applicationTransaction) Licenses() core.WriteTable[core.License] {
	return writer[core.License](t, "licenses")
}

func (v *applicationView) Activations() core.ReadTable[core.Activation] {
	return reader[core.Activation](v.base, "activations")
}
func (t *applicationTransaction) Activations() core.WriteTable[core.Activation] {
	return writer[core.Activation](t, "activations")
}

func (v *applicationView) Cards() core.ReadTable[core.CardKey] {
	return reader[core.CardKey](v.base, "cards")
}
func (t *applicationTransaction) Cards() core.WriteTable[core.CardKey] {
	return writer[core.CardKey](t, "cards")
}

func (v *applicationView) Rules() core.ReadTable[core.BindingRule] {
	return reader[core.BindingRule](v.base, "rules")
}
func (t *applicationTransaction) Rules() core.WriteTable[core.BindingRule] {
	return writer[core.BindingRule](t, "rules")
}

func (v *applicationView) Anchors() core.ReadTable[core.BindingAnchor] {
	return reader[core.BindingAnchor](v.base, "anchors")
}
func (t *applicationTransaction) Anchors() core.WriteTable[core.BindingAnchor] {
	return writer[core.BindingAnchor](t, "anchors")
}

func (v *applicationView) Invitations() core.ReadTable[core.Invitation] {
	return reader[core.Invitation](v.base, "invitations")
}
func (t *applicationTransaction) Invitations() core.WriteTable[core.Invitation] {
	return writer[core.Invitation](t, "invitations")
}

func (v *applicationView) PendingInstallations() core.ReadTable[core.PendingInstallation] {
	return reader[core.PendingInstallation](v.base, "pending")
}
func (t *applicationTransaction) PendingInstallations() core.WriteTable[core.PendingInstallation] {
	return writer[core.PendingInstallation](t, "pending")
}

func (v *applicationView) Nonces() core.ReadTable[core.RequestNonce] {
	return reader[core.RequestNonce](v.base, "nonces")
}
func (t *applicationTransaction) Nonces() core.WriteTable[core.RequestNonce] {
	return writer[core.RequestNonce](t, "nonces")
}

func (v *applicationView) Channels() core.ReadTable[core.SalesChannel] {
	return reader[core.SalesChannel](v.base, "channels")
}
func (t *applicationTransaction) Channels() core.WriteTable[core.SalesChannel] {
	return writer[core.SalesChannel](t, "channels")
}

func (v *applicationView) Orders() core.ReadTable[core.SalesOrder] {
	return reader[core.SalesOrder](v.base, "orders")
}
func (t *applicationTransaction) Orders() core.WriteTable[core.SalesOrder] {
	return writer[core.SalesOrder](t, "orders")
}

func (v *applicationView) Webhooks() core.ReadTable[core.WebhookReceipt] {
	return reader[core.WebhookReceipt](v.base, "webhooks")
}
func (t *applicationTransaction) Webhooks() core.WriteTable[core.WebhookReceipt] {
	return writer[core.WebhookReceipt](t, "webhooks")
}

func (v *applicationView) Usage() core.ReadTable[core.UsageReceipt] {
	return reader[core.UsageReceipt](v.base, "usage")
}
func (t *applicationTransaction) Usage() core.WriteTable[core.UsageReceipt] {
	return writer[core.UsageReceipt](t, "usage")
}

func (v *applicationView) AgentGrants() core.ReadTable[core.AgentGrant] {
	return reader[core.AgentGrant](v.base, "agents")
}
func (t *applicationTransaction) AgentGrants() core.WriteTable[core.AgentGrant] {
	return writer[core.AgentGrant](t, "agents")
}

func (v *applicationView) MoneyEvents() core.ReadTable[core.MoneyEvent] {
	return reader[core.MoneyEvent](v.base, "money")
}
func (t *applicationTransaction) MoneyEvents() core.WriteTable[core.MoneyEvent] {
	return writer[core.MoneyEvent](t, "money")
}

func (v *applicationView) Releases() core.ReadTable[core.Release] {
	return reader[core.Release](v.base, "releases")
}
func (t *applicationTransaction) Releases() core.WriteTable[core.Release] {
	return writer[core.Release](t, "releases")
}

func (v *applicationView) Audit() core.ReadTable[core.AuditEvent] {
	return reader[core.AuditEvent](v.base, "audit")
}

// Agent identity and funding are explicitly platform-scoped. These narrow
// operations keep the balance debit inside the application's issuing
// transaction without exposing an arbitrary SQL escape hatch.
func (t *applicationTransaction) AgentAccount(ctx context.Context, id string) (core.AgentAccount, error) {
	if e := t.repo.check(ctx, false); e != nil {
		return core.AgentAccount{}, e
	}
	if t.writable {
		if _, e := t.runner.ExecContext(ctx, "UPDATE agents SET updated_at=updated_at WHERE id=?", id); e != nil {
			return core.AgentAccount{}, e
		}
		if _, e := t.runner.ExecContext(ctx, "UPDATE agent_accounts SET balance=balance WHERE agent_id=?", id); e != nil {
			return core.AgentAccount{}, e
		}
	}
	a := core.AgentAccount{AgentID: id, CostRateBPS: 10000}
	var tier sql.NullString
	e := t.runner.QueryRowContext(ctx, "SELECT a.status,c.balance,c.tier_id FROM agents a JOIN agent_accounts c ON c.agent_id=a.id WHERE a.id=?", id).Scan(&a.Status, &a.BalanceCents, &tier)
	if errors.Is(e, sql.ErrNoRows) {
		return a, core.ErrNotFound
	}
	if e != nil {
		return a, e
	}
	a.TierID = tier.String
	if a.TierID != "" {
		if t.writable {
			if _, e = t.runner.ExecContext(ctx, "UPDATE agent_tiers SET id=id WHERE id=?", a.TierID); e != nil {
				return a, e
			}
		}
		e = t.runner.QueryRowContext(ctx, "SELECT COALESCE(s.cost_rate_bps,10000) FROM agent_tiers a LEFT JOIN agent_tier_settings s ON s.tier_id=a.id WHERE a.id=?", a.TierID).Scan(&a.CostRateBPS)
		if e != nil {
			return a, e
		}
	}
	return a, nil
}
func (v *applicationView) AgentAccount(ctx context.Context, id string) (core.AgentAccount, error) {
	return v.base.AgentAccount(ctx, id)
}
func (t *applicationTransaction) DebitAgent(ctx context.Context, id string, amount int64) (int64, error) {
	if e := t.checkWrite(ctx); e != nil {
		return 0, e
	}
	if amount < 0 {
		return 0, errors.New("negative debit")
	}
	a, e := t.AgentAccount(ctx, id)
	if e != nil {
		return 0, e
	}
	if a.Status != "active" {
		return 0, core.Error{Code: "AGENT_DISABLED", Message: "代理已停用", Status: 403}
	}
	if a.BalanceCents < amount {
		return 0, core.Error{Code: "INSUFFICIENT_BALANCE", Message: "余额不足，请联系管理员入账", Status: 409}
	}
	result, e := t.runner.ExecContext(ctx, "UPDATE agent_accounts SET balance=balance-? WHERE agent_id=? AND balance>=?", amount, id, amount)
	if e != nil {
		return 0, e
	}
	if amount > 0 {
		n, e := result.RowsAffected()
		if e != nil {
			return 0, e
		}
		if n != 1 {
			return 0, core.ErrConflict
		}
	}
	t.mutations++
	return a.BalanceCents - amount, nil
}

// Capacity accounting is an explicitly privileged aggregate; no foreign
// application rows are exposed. The global guard serializes quota reservations.
func (t *applicationTransaction) ReleaseCapacity(ctx context.Context) (core.ReleaseCapacity, error) {
	var out core.ReleaseCapacity
	if e := t.checkWrite(ctx); e != nil {
		return out, e
	}
	if _, e := t.runner.ExecContext(ctx, "UPDATE release_storage_lock SET id=id WHERE id=1"); e != nil {
		return out, e
	}
	if e := t.runner.QueryRowContext(ctx, "SELECT COALESCE(SUM(size),0) FROM application_releases WHERE status<>'deleted'").Scan(&out.TotalBytes); e != nil {
		return out, e
	}
	var legacy int64
	if e := t.runner.QueryRowContext(ctx, "SELECT COALESCE(SUM(size),0) FROM product_releases WHERE status<>'deleted'").Scan(&legacy); e != nil {
		return out, e
	}
	out.TotalBytes += legacy
	if e := t.runner.QueryRowContext(ctx, "SELECT COALESCE(SUM(size),0),COUNT(*) FROM application_releases WHERE application_id=? AND status<>'deleted'", t.repo.scope.ApplicationID()).Scan(&out.ApplicationBytes, &out.Versions); e != nil {
		return out, e
	}
	app, e := t.Application(ctx)
	if e != nil {
		return out, e
	}
	if app.DeveloperID != "" {
		e = t.runner.QueryRowContext(ctx, "SELECT COALESCE(SUM(size),0) FROM application_releases WHERE status<>'deleted' AND application_id IN (SELECT application_id FROM applications WHERE owner_developer_id=?)", app.DeveloperID).Scan(&out.DeveloperBytes)
	}
	return out, e
}

func (t *applicationTransaction) CountBindingGroups(ctx context.Context, id string) (int64, error) {
	if e := t.repo.check(ctx, false); e != nil {
		return 0, e
	}
	var n int64
	e := t.runner.QueryRowContext(ctx, "SELECT COUNT(DISTINCT group_id) FROM application_binding_anchors WHERE application_id=? AND license_id=? AND status='active'", t.repo.scope.ApplicationID(), id).Scan(&n)
	return n, e
}
func (t *applicationTransaction) RetireBindings(ctx context.Context, id string, now time.Time) error {
	if e := t.checkWrite(ctx); e != nil {
		return e
	}
	stamp := coreTS(now)
	result, e := t.runner.ExecContext(ctx, "UPDATE application_binding_anchors SET status='replaced',updated_at=?,data_json=JSON_SET(data_json,'$.record.status','replaced','$.record.updated_at',?) WHERE application_id=? AND license_id=? AND status='active'", stamp, stamp, t.repo.scope.ApplicationID(), id)
	return t.bulkMutation(result, e)
}
func (t *applicationTransaction) DeactivateAll(ctx context.Context, id string, now time.Time) error {
	if e := t.checkWrite(ctx); e != nil {
		return e
	}
	stamp := coreTS(now)
	result, e := t.runner.ExecContext(ctx, "UPDATE application_activations SET deactivated_at=?,updated_at=?,data_json=JSON_SET(data_json,'$.record.deactivated_at',?,'$.record.updated_at',?,'$.record.deactivation_reason','administrative') WHERE application_id=? AND license_id=? AND deactivated_at IS NULL", stamp, stamp, stamp, stamp, t.repo.scope.ApplicationID(), id)
	return t.bulkMutation(result, e)
}
func (t *applicationTransaction) InvalidateInvitations(ctx context.Context, id string, now time.Time) error {
	if e := t.checkWrite(ctx); e != nil {
		return e
	}
	stamp := coreTS(now)
	result, e := t.runner.ExecContext(ctx, "UPDATE application_installation_invitations SET used_at=?,updated_at=?,data_json=JSON_SET(data_json,'$.record.used_at',?,'$.record.updated_at',?,'$.record.public_key_hash','') WHERE application_id=? AND license_id=?", stamp, stamp, stamp, stamp, t.repo.scope.ApplicationID(), id)
	return t.bulkMutation(result, e)
}
func (t *applicationTransaction) bulkMutation(result sql.Result, e error) error {
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e == nil && n > 0 {
		t.mutations++
	}
	return e
}

func (t *applicationTransaction) CreditAgent(ctx context.Context, id string, amount int64) (int64, error) {
	if e := t.checkWrite(ctx); e != nil {
		return 0, e
	}
	if amount < 0 {
		return 0, errors.New("negative refund credit")
	}
	a, e := t.AgentAccount(ctx, id)
	if e != nil {
		return 0, e
	}
	if amount > 1_000_000_000_000-a.BalanceCents {
		return 0, core.Error{Code: "BALANCE_LIMIT", Message: "退款将超过代理余额上限，请先处理余额", Status: 409}
	}
	result, e := t.runner.ExecContext(ctx, "UPDATE agent_accounts SET balance=balance+? WHERE agent_id=? AND balance<=?", amount, id, 1_000_000_000_000-amount)
	if e != nil {
		return 0, e
	}
	if amount > 0 {
		n, e := result.RowsAffected()
		if e != nil {
			return 0, e
		}
		if n != 1 {
			return 0, core.ErrConflict
		}
	}
	t.mutations++
	return a.BalanceCents + amount, nil
}
