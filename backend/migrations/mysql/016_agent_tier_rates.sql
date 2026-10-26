CREATE TABLE IF NOT EXISTS agent_tier_settings (
 tier_id VARCHAR(64) PRIMARY KEY,
 cost_rate_bps INT NOT NULL CHECK(cost_rate_bps BETWEEN 1 AND 10000),
 sort_order INT NOT NULL DEFAULT 100,
 FOREIGN KEY(tier_id) REFERENCES agent_tiers(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS commerce_initializations (name VARCHAR(64) PRIMARY KEY) ENGINE=InnoDB;
