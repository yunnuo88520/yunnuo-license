package store

import (
	"context"
	"database/sql"
	"encoding/json"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"time"
)

type commerceQuery interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (tx *Tx) GetProduct(ctx context.Context, id string) (domain.Product, error) {
	return scanProduct(tx.tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, id))
}

func (tx *Tx) LockAgent(ctx context.Context, id string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE agents SET id=id WHERE id=?`, id)
	return err
}
func (tx *Tx) LockProduct(ctx context.Context, id string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE products SET id=id WHERE id=?`, id)
	return err
}
func readPackage(row scanner) (domain.CardPackage, error) {
	var termsJSON string
	var p domain.CardPackage
	var permanent, quota, enabled int
	err := row.Scan(&p.ID, &p.ProductID, &p.Name, &p.DurationDays, &permanent, &p.BaseCost, &quota, &enabled, &termsJSON)
	if err != nil {
		return p, err
	}
	p.AuthorizationTerms, err = decodeTerms(termsJSON)
	p.IsPermanent = permanent == 1
	p.UseQuota = quota == 1
	p.Enabled = enabled == 1
	return p, err
}

const packageWriteCols = `id,product_id,name,duration_days,is_permanent,base_cost,use_quota,enabled`
const packageCols = packageWriteCols + `,COALESCE((SELECT terms_json FROM authorization_snapshots WHERE scope='package' AND resource_id=card_packages.id),'')`

func (s *Store) Packages(ctx context.Context) ([]domain.CardPackage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+packageCols+` FROM card_packages ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.CardPackage{}
	for rows.Next() {
		p, e := readPackage(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
func (s *Store) Package(ctx context.Context, id string) (domain.CardPackage, error) {
	return readPackage(s.db.QueryRowContext(ctx, `SELECT `+packageCols+` FROM card_packages WHERE id=?`, id))
}
func (tx *Tx) Package(ctx context.Context, id string) (domain.CardPackage, error) {
	return readPackage(tx.tx.QueryRowContext(ctx, `SELECT `+packageCols+` FROM card_packages WHERE id=?`, id))
}
func (tx *Tx) CreatePackage(ctx context.Context, p domain.CardPackage) error {
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO card_packages (`+packageWriteCols+`) VALUES (?,?,?,?,?,?,?,?)`, p.ID, p.ProductID, p.Name, p.DurationDays, boolInt(p.IsPermanent), p.BaseCost, boolInt(p.UseQuota), boolInt(p.Enabled))
	if err != nil {
		return err
	}
	return tx.SaveTerms(ctx, "package", p.ID, p.AuthorizationTerms)
}
func (tx *Tx) SetPackage(ctx context.Context, id string, enabled bool, cost int64) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE card_packages SET enabled=?,base_cost=? WHERE id=?`, boolInt(enabled), cost, id)
	return err
}
func (s *Store) Groups(ctx context.Context) ([]domain.ProductGroup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name FROM product_groups ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	items := []domain.ProductGroup{}
	for rows.Next() {
		var g domain.ProductGroup
		if err = rows.Scan(&g.ID, &g.Name); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range items {
		r, e := s.db.QueryContext(ctx, `SELECT product_id FROM product_group_members WHERE group_id=? ORDER BY product_id`, items[i].ID)
		if e != nil {
			return nil, e
		}
		items[i].ProductIDs = []string{}
		for r.Next() {
			var id string
			if e = r.Scan(&id); e != nil {
				r.Close()
				return nil, e
			}
			items[i].ProductIDs = append(items[i].ProductIDs, id)
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return nil, e
		}
	}
	return items, nil
}
func (tx *Tx) SaveGroup(ctx context.Context, g domain.ProductGroup) error {
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM product_group_members WHERE group_id=?`, g.ID); err != nil {
		return err
	}
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM product_groups WHERE id=?`, g.ID); err != nil {
		return err
	}
	if _, err := tx.tx.ExecContext(ctx, `INSERT INTO product_groups (id,name) VALUES (?,?)`, g.ID, g.Name); err != nil {
		return err
	}
	for _, id := range g.ProductIDs {
		if _, err := tx.tx.ExecContext(ctx, `INSERT INTO product_group_members (group_id,product_id) VALUES (?,?)`, g.ID, id); err != nil {
			return err
		}
	}
	return nil
}
func (tx *Tx) GroupPackages(ctx context.Context, id string) ([]domain.CardPackage, error) {
	if _, err := tx.tx.ExecContext(ctx, `UPDATE product_groups SET id=id WHERE id=?`, id); err != nil {
		return nil, err
	}
	var exists string
	if err := tx.tx.QueryRowContext(ctx, `SELECT id FROM product_groups WHERE id=?`, id).Scan(&exists); err != nil {
		return nil, err
	}
	rows, err := tx.tx.QueryContext(ctx, `SELECT `+packageCols+` FROM card_packages WHERE enabled=1 AND product_id IN (SELECT product_id FROM product_group_members WHERE group_id=?) ORDER BY product_id,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.CardPackage{}
	for rows.Next() {
		p, e := readPackage(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
func (s *Store) Tiers(ctx context.Context) ([]domain.AgentTier, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+tierColumns+tierJoin+` ORDER BY COALESCE(s.sort_order,100),t.name,t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.AgentTier{}
	for rows.Next() {
		t, e := scanTier(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, t)
	}
	return items, rows.Err()
}
func (tx *Tx) CreateTier(ctx context.Context, t domain.AgentTier) error {
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO agent_tiers (id,name) VALUES (?,?)`, t.ID, t.Name)
	if err != nil {
		return err
	}
	return tx.saveTierSettings(ctx, t)
}
func (tx *Tx) SetTierPrice(ctx context.Context, tier, pkg string, cost *int64) error {
	if err := tx.lockTierReference(ctx, tier); err != nil {
		return err
	}
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM agent_tier_prices WHERE tier_id=? AND package_id=?`, tier, pkg); err != nil {
		return err
	}
	if cost == nil {
		return nil
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO agent_tier_prices (tier_id,package_id,unit_cost) VALUES (?,?,?)`, tier, pkg, *cost)
	return err
}
func readAccount(ctx context.Context, q commerceQuery, id string) (domain.AgentAccount, error) {
	a := domain.AgentAccount{AgentID: id}
	err := q.QueryRowContext(ctx, `SELECT tier_id,balance FROM agent_accounts WHERE agent_id=?`, id).Scan(&a.TierID, &a.Balance)
	if IsNotFound(err) {
		err = nil
	}
	return a, err
}
func (s *Store) Account(ctx context.Context, id string) (domain.AgentAccount, error) {
	return readAccount(ctx, s.db, id)
}
func (tx *Tx) Account(ctx context.Context, id string) (domain.AgentAccount, error) {
	return readAccount(ctx, tx.tx, id)
}
func (tx *Tx) SaveAccount(ctx context.Context, a domain.AgentAccount) error {
	if a.TierID != "" {
		if err := tx.lockTierReference(ctx, a.TierID); err != nil {
			return err
		}
	}
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM agent_accounts WHERE agent_id=?`, a.AgentID); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO agent_accounts (agent_id,tier_id,balance) VALUES (?,?,?)`, a.AgentID, a.TierID, a.Balance)
	return err
}
func (tx *Tx) SaveGrant(ctx context.Context, g domain.PackageGrant) error {
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM agent_package_grants WHERE agent_id=? AND package_id=?`, g.AgentID, g.PackageID); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO agent_package_grants (agent_id,package_id,enabled,override_cost) VALUES (?,?,?,?)`, g.AgentID, g.PackageID, boolInt(g.Enabled), g.OverrideCost)
	return err
}
func (tx *Tx) EnsurePackagePolicy(ctx context.Context, agent string, p domain.CardPackage, max int, export bool) error {
	policy, err := tx.GetAgentProductPolicy(ctx, agent, p.ProductID)
	if err != nil && !IsNotFound(err) {
		return err
	}
	days := policy.AllowedDurationDays
	found := false
	for _, d := range days {
		found = found || d == p.DurationDays
	}
	if !p.IsPermanent && !found {
		days = append(days, p.DurationDays)
	}
	if policy.ID != "" {
		_, err = tx.tx.ExecContext(ctx, `UPDATE agent_product_policies SET can_generate=1,allowed_duration_days=?,allow_permanent=?,max_batch_quantity=?,can_export_plain_code=?,status='active' WHERE agent_id=? AND product_id=?`, jsonInts(days), boolInt(p.IsPermanent || policy.AllowPermanent), max, boolInt(export), agent, p.ProductID)
		return err
	}
	_, err = tx.tx.ExecContext(ctx, `INSERT INTO agent_product_policies (id,agent_id,product_id,can_generate,can_export_plain_code,allowed_duration_days,allow_permanent,max_batch_quantity,discount_rate,settlement_price,status,created_at,updated_at) VALUES (?,?,?,1,?,?,?,?,1,0,'active',?,?)`, yncrypto.NewID("apol"), agent, p.ProductID, boolInt(export), jsonInts(days), boolInt(p.IsPermanent), max, ts(time.Now()), ts(time.Now()))
	return err
}
func quotePackage(ctx context.Context, q commerceQuery, agent string, p domain.CardPackage) (domain.PackageQuote, error) {
	var enabled int
	var override sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT enabled,override_cost FROM agent_package_grants WHERE agent_id=? AND package_id=?`, agent, p.ID).Scan(&enabled, &override)
	quote := domain.PackageQuote{Package: p, UnitCost: p.BaseCost, PriceSource: "package"}
	if err != nil {
		return quote, err
	}
	if enabled != 1 || !p.Enabled {
		return quote, sql.ErrNoRows
	}
	if override.Valid {
		quote.UnitCost = override.Int64
		quote.PriceSource = "agent"
		return quote, nil
	}
	var tierPrice int64
	err = q.QueryRowContext(ctx, `SELECT unit_cost FROM agent_tier_prices WHERE package_id=? AND tier_id=(SELECT tier_id FROM agent_accounts WHERE agent_id=?)`, p.ID, agent).Scan(&tierPrice)
	if err == nil {
		quote.UnitCost = tierPrice
		quote.PriceSource = "tier"
		return quote, nil
	}
	if !IsNotFound(err) {
		return quote, err
	}
	var rate int64
	err = q.QueryRowContext(ctx, `SELECT COALESCE(s.cost_rate_bps,10000) FROM agent_tiers t LEFT JOIN agent_tier_settings s ON s.tier_id=t.id WHERE t.id=(SELECT tier_id FROM agent_accounts WHERE agent_id=?)`, agent).Scan(&rate)
	if IsNotFound(err) {
		return quote, nil
	}
	if err != nil {
		return quote, err
	}
	quote.UnitCost = (p.BaseCost*rate + 9999) / 10000
	quote.PriceSource = "tier_ratio"
	return quote, nil
}
func (s *Store) Quote(ctx context.Context, agent string, p domain.CardPackage) (domain.PackageQuote, error) {
	return quotePackage(ctx, s.db, agent, p)
}
func (tx *Tx) Quote(ctx context.Context, agent string, p domain.CardPackage) (domain.PackageQuote, error) {
	return quotePackage(ctx, tx.tx, agent, p)
}

const moneyCols = `id,agent_id,request_id,kind,amount,balance_after,batch_id,package_id,unit_cost,quantity,remark,actor_id,created_at`

func readMoney(row scanner) (domain.MoneyLedger, error) {
	var l domain.MoneyLedger
	var created string
	err := row.Scan(&l.ID, &l.AgentID, &l.RequestID, &l.Kind, &l.Amount, &l.BalanceAfter, &l.BatchID, &l.PackageID, &l.UnitCost, &l.Quantity, &l.Remark, &l.ActorID, &created)
	l.CreatedAt = parseTS(created)
	return l, err
}
func (tx *Tx) MoneyByRequest(ctx context.Context, agent, request string) (domain.MoneyLedger, error) {
	return readMoney(tx.tx.QueryRowContext(ctx, `SELECT `+moneyCols+` FROM agent_money_ledgers WHERE agent_id=? AND request_id=?`, agent, request))
}
func (tx *Tx) InsertMoney(ctx context.Context, l domain.MoneyLedger) error {
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO agent_money_ledgers (`+moneyCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, l.ID, l.AgentID, l.RequestID, l.Kind, l.Amount, l.BalanceAfter, l.BatchID, l.PackageID, l.UnitCost, l.Quantity, l.Remark, l.ActorID, ts(l.CreatedAt))
	return err
}
func (s *Store) MoneyLedgers(ctx context.Context, agent string) ([]domain.MoneyLedger, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+moneyCols+` FROM agent_money_ledgers WHERE agent_id=? ORDER BY created_at DESC,id DESC LIMIT 500`, agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.MoneyLedger{}
	for rows.Next() {
		l, e := readMoney(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, l)
	}
	return items, rows.Err()
}
func (tx *Tx) PackageOrder(ctx context.Context, agent, request string) (fingerprint, batch string, err error) {
	err = tx.tx.QueryRowContext(ctx, `SELECT fingerprint,batch_id FROM agent_package_orders WHERE agent_id=? AND request_id=?`, agent, request).Scan(&fingerprint, &batch)
	return
}
func (tx *Tx) InsertPackageOrder(ctx context.Context, agent, request, fingerprint, batch string) error {
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO agent_package_orders (agent_id,request_id,fingerprint,batch_id) VALUES (?,?,?,?)`, agent, request, fingerprint, batch)
	return err
}

func jsonInts(values []int) string { b, _ := json.Marshal(values); return string(b) }
