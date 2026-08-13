PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS agents (
  id TEXT PRIMARY KEY,
  agent_no TEXT NOT NULL UNIQUE,
  parent_agent_id TEXT,
  name TEXT NOT NULL,
  contact_name TEXT,
  phone TEXT,
  email TEXT,
  level INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL,
  settlement_mode TEXT NOT NULL,
  default_discount_rate REAL NOT NULL DEFAULT 1,
  credit_limit INTEGER NOT NULL DEFAULT 0,
  remark TEXT,
  created_by TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  disabled_at TEXT,
  FOREIGN KEY (parent_agent_id) REFERENCES agents(id)
);

CREATE INDEX IF NOT EXISTS idx_agents_parent ON agents(parent_agent_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);

CREATE TABLE IF NOT EXISTS agent_users (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  username TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  display_name TEXT,
  phone TEXT,
  email TEXT,
  role TEXT NOT NULL,
  status TEXT NOT NULL,
  last_login_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (agent_id) REFERENCES agents(id),
  UNIQUE (agent_id, username)
);

CREATE INDEX IF NOT EXISTS idx_agent_users_agent_status ON agent_users(agent_id, status);

CREATE TABLE IF NOT EXISTS agent_product_policies (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  can_generate INTEGER NOT NULL,
  can_export_plain_code INTEGER NOT NULL,
  allowed_duration_days TEXT NOT NULL,
  allow_permanent INTEGER NOT NULL,
  max_batch_quantity INTEGER NOT NULL,
  discount_rate REAL NOT NULL DEFAULT 1,
  settlement_price INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (agent_id) REFERENCES agents(id),
  FOREIGN KEY (product_id) REFERENCES products(id),
  UNIQUE (agent_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_policies_product_status ON agent_product_policies(product_id, status);

CREATE TABLE IF NOT EXISTS agent_quota_ledgers (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  duration_days INTEGER NOT NULL,
  is_permanent INTEGER NOT NULL,
  change_type TEXT NOT NULL,
  change_quantity INTEGER NOT NULL,
  balance_after INTEGER NOT NULL,
  related_batch_id TEXT,
  related_card_id TEXT,
  operator_type TEXT NOT NULL,
  operator_id TEXT,
  remark TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (agent_id) REFERENCES agents(id),
  FOREIGN KEY (product_id) REFERENCES products(id),
  FOREIGN KEY (related_batch_id) REFERENCES card_batches(id),
  FOREIGN KEY (related_card_id) REFERENCES cards(id)
);

CREATE INDEX IF NOT EXISTS idx_agent_quota_scope ON agent_quota_ledgers(agent_id, product_id, duration_days, is_permanent);
CREATE INDEX IF NOT EXISTS idx_agent_quota_batch ON agent_quota_ledgers(related_batch_id);
