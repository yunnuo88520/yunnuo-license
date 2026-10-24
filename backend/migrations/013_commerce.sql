CREATE TABLE IF NOT EXISTS product_groups (id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS product_group_members (
 group_id TEXT NOT NULL REFERENCES product_groups(id), product_id TEXT NOT NULL REFERENCES products(id), PRIMARY KEY(group_id,product_id)
);
CREATE TABLE IF NOT EXISTS card_packages (
 id TEXT PRIMARY KEY, product_id TEXT NOT NULL REFERENCES products(id), name TEXT NOT NULL,
 duration_days INTEGER NOT NULL, is_permanent INTEGER NOT NULL, base_cost BIGINT NOT NULL,
 use_quota INTEGER NOT NULL, enabled INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_tiers (id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS agent_tier_prices (
 tier_id TEXT NOT NULL REFERENCES agent_tiers(id), package_id TEXT NOT NULL REFERENCES card_packages(id), unit_cost BIGINT NOT NULL, PRIMARY KEY(tier_id,package_id)
);
CREATE TABLE IF NOT EXISTS agent_accounts (
 agent_id TEXT PRIMARY KEY REFERENCES agents(id), tier_id TEXT NOT NULL DEFAULT '', balance BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS agent_package_grants (
 agent_id TEXT NOT NULL REFERENCES agents(id), package_id TEXT NOT NULL REFERENCES card_packages(id), enabled INTEGER NOT NULL, override_cost BIGINT,
 PRIMARY KEY(agent_id,package_id)
);
CREATE TABLE IF NOT EXISTS agent_money_ledgers (
 id TEXT PRIMARY KEY, agent_id TEXT NOT NULL REFERENCES agents(id), request_id TEXT NOT NULL, kind TEXT NOT NULL,
 amount BIGINT NOT NULL, balance_after BIGINT NOT NULL, batch_id TEXT NOT NULL, package_id TEXT NOT NULL,
 unit_cost BIGINT NOT NULL, quantity INTEGER NOT NULL, remark TEXT NOT NULL, actor_id TEXT NOT NULL, created_at TEXT NOT NULL,
 UNIQUE(agent_id,request_id)
);
CREATE TABLE IF NOT EXISTS agent_package_orders (
 agent_id TEXT NOT NULL REFERENCES agents(id), request_id TEXT NOT NULL, fingerprint TEXT NOT NULL,
 batch_id TEXT NOT NULL REFERENCES card_batches(id), PRIMARY KEY(agent_id,request_id)
);
