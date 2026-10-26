CREATE TABLE IF NOT EXISTS agent_tier_settings (
 tier_id TEXT PRIMARY KEY REFERENCES agent_tiers(id),
 cost_rate_bps INTEGER NOT NULL CHECK(cost_rate_bps BETWEEN 1 AND 10000),
 sort_order INTEGER NOT NULL DEFAULT 100
);
CREATE TABLE IF NOT EXISTS commerce_initializations (name TEXT PRIMARY KEY);
