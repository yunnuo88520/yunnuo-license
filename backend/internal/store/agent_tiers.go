package store

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

const tierColumns = `t.id,t.name,COALESCE(s.cost_rate_bps,10000),COALESCE(s.sort_order,100),(SELECT COUNT(*) FROM agent_accounts a WHERE a.tier_id=t.id)`
const tierJoin = ` FROM agent_tiers t LEFT JOIN agent_tier_settings s ON s.tier_id=t.id`

func scanTier(row scanner) (domain.AgentTier, error) {
	var t domain.AgentTier
	err := row.Scan(&t.ID, &t.Name, &t.CostRateBPS, &t.SortOrder, &t.AgentCount)
	return t, err
}

func (s *Store) ensureDefaultAgentTiers(ctx context.Context) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		insert := `INSERT OR IGNORE INTO commerce_initializations(name) VALUES('agent_tiers_v1')`
		if s.driver == "mysql" {
			insert = `INSERT IGNORE INTO commerce_initializations(name) VALUES('agent_tiers_v1')`
		}
		result, err := tx.tx.ExecContext(ctx, insert)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		for _, t := range []domain.AgentTier{
			{ID: "tier_standard", Name: "普通代理", CostRateBPS: 10000, SortOrder: 10},
			{ID: "tier_advanced", Name: "高级代理", CostRateBPS: 9000, SortOrder: 20},
			{ID: "tier_gold", Name: "金牌代理", CostRateBPS: 8000, SortOrder: 30},
		} {
			if err := tx.CreateTier(ctx, t); err != nil {
				return err
			}
		}
		return nil
	})
}

func (tx *Tx) Tier(ctx context.Context, id string) (domain.AgentTier, error) {
	return scanTier(tx.tx.QueryRowContext(ctx, `SELECT `+tierColumns+tierJoin+` WHERE t.id=?`, id))
}
func (tx *Tx) LockTier(ctx context.Context, id string) error {
	_, err := tx.tx.ExecContext(ctx, `UPDATE agent_tiers SET id=id WHERE id=?`, id)
	return err
}
func (tx *Tx) saveTierSettings(ctx context.Context, t domain.AgentTier) error {
	if _, err := tx.tx.ExecContext(ctx, `DELETE FROM agent_tier_settings WHERE tier_id=?`, t.ID); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(ctx, `INSERT INTO agent_tier_settings(tier_id,cost_rate_bps,sort_order) VALUES(?,?,?)`, t.ID, t.CostRateBPS, t.SortOrder)
	return err
}
func (tx *Tx) UpdateTier(ctx context.Context, t domain.AgentTier) error {
	if _, err := tx.tx.ExecContext(ctx, `UPDATE agent_tiers SET name=? WHERE id=?`, t.Name, t.ID); err != nil {
		return err
	}
	return tx.saveTierSettings(ctx, t)
}
func (tx *Tx) DeleteTier(ctx context.Context, id string) error {
	for _, table := range []string{"agent_tier_prices", "agent_tier_settings"} {
		if _, err := tx.tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE tier_id=?`, id); err != nil {
			return err
		}
	}
	_, err := tx.tx.ExecContext(ctx, `DELETE FROM agent_tiers WHERE id=?`, id)
	return err
}

// Lock the current tier before the first snapshot read in an issuance transaction.
// MySQL shared row locks allow concurrent sales but serialize edits/deletions.
// SQLite already reserves its writer at BEGIN IMMEDIATE.
func (tx *Tx) LockAgentTierPricing(ctx context.Context, agent string) error {
	query := `SELECT t.id FROM agent_tiers t JOIN agent_accounts a ON a.tier_id=t.id WHERE a.agent_id=?`
	if tx.driver == "mysql" {
		query += ` FOR SHARE`
	}
	var id string
	err := tx.tx.QueryRowContext(ctx, query, agent).Scan(&id)
	if IsNotFound(err) {
		return nil
	}
	return err
}

func (tx *Tx) lockTierReference(ctx context.Context, id string) error {
	query := `SELECT id FROM agent_tiers WHERE id=?`
	if tx.driver == "mysql" {
		query += ` FOR SHARE`
	}
	var found string
	return tx.tx.QueryRowContext(ctx, query, id).Scan(&found)
}
