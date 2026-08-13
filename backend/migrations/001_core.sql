PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS products (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  code TEXT NOT NULL UNIQUE,
  app_key TEXT NOT NULL UNIQUE,
  public_key_pem TEXT NOT NULL,
  private_key_encrypted TEXT NOT NULL,
  bind_mode TEXT NOT NULL,
  max_bind_count INTEGER NOT NULL,
  bind_conflict_strategy TEXT NOT NULL,
  offline_mode TEXT NOT NULL,
  offline_grace_days INTEGER NOT NULL,
  expire_grace_days INTEGER NOT NULL,
  unbind_limit INTEGER NOT NULL,
  unbind_cooldown_hours INTEGER NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS card_batches (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL,
  agent_id TEXT,
  name TEXT NOT NULL,
  quantity INTEGER NOT NULL,
  duration_days INTEGER NOT NULL,
  is_permanent INTEGER NOT NULL,
  source TEXT NOT NULL,
  status TEXT NOT NULL,
  export_count INTEGER NOT NULL DEFAULT 0,
  created_by TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (product_id) REFERENCES products(id)
);

CREATE TABLE IF NOT EXISTS cards (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL,
  batch_id TEXT NOT NULL,
  agent_id TEXT,
  code_hash TEXT NOT NULL UNIQUE,
  code_encrypted TEXT NOT NULL,
  code_prefix TEXT NOT NULL,
  duration_days INTEGER NOT NULL,
  is_permanent INTEGER NOT NULL,
  status TEXT NOT NULL,
  order_no TEXT,
  activated_license_id TEXT,
  activated_at TEXT,
  consumed_at TEXT,
  voided_at TEXT,
  void_reason TEXT,
  created_by TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (product_id) REFERENCES products(id),
  FOREIGN KEY (batch_id) REFERENCES card_batches(id)
);

CREATE INDEX IF NOT EXISTS idx_cards_product_status ON cards(product_id, status);
CREATE INDEX IF NOT EXISTS idx_cards_agent_status ON cards(agent_id, status);
CREATE INDEX IF NOT EXISTS idx_cards_batch ON cards(product_id, batch_id);

CREATE TABLE IF NOT EXISTS licenses (
  id TEXT PRIMARY KEY,
  license_no TEXT NOT NULL UNIQUE,
  product_id TEXT NOT NULL,
  card_id TEXT NOT NULL UNIQUE,
  agent_id TEXT,
  customer_id TEXT,
  account_ref TEXT,
  status TEXT NOT NULL,
  issued_at TEXT NOT NULL,
  activated_at TEXT NOT NULL,
  expired_at TEXT,
  last_verify_at TEXT,
  last_heartbeat_at TEXT,
  revoked_at TEXT,
  revoked_reason TEXT,
  offline_token_version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (product_id) REFERENCES products(id),
  FOREIGN KEY (card_id) REFERENCES cards(id)
);

CREATE INDEX IF NOT EXISTS idx_licenses_product_status ON licenses(product_id, status);
CREATE INDEX IF NOT EXISTS idx_licenses_agent_status ON licenses(agent_id, status);
CREATE INDEX IF NOT EXISTS idx_licenses_expired_at ON licenses(expired_at);

CREATE TABLE IF NOT EXISTS license_bindings (
  id TEXT PRIMARY KEY,
  license_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  bind_mode TEXT NOT NULL,
  bind_value_hash TEXT NOT NULL,
  bind_value_encrypted TEXT NOT NULL,
  display_name TEXT,
  status TEXT NOT NULL,
  first_seen_ip TEXT,
  last_seen_ip TEXT,
  user_agent TEXT,
  last_heartbeat_at TEXT,
  activated_at TEXT NOT NULL,
  unbound_at TEXT,
  kicked_at TEXT,
  revoked_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (license_id) REFERENCES licenses(id),
  FOREIGN KEY (product_id) REFERENCES products(id),
  UNIQUE (license_id, bind_mode, bind_value_hash)
);

CREATE INDEX IF NOT EXISTS idx_bindings_license_status ON license_bindings(license_id, status);
CREATE INDEX IF NOT EXISTS idx_bindings_value_hash ON license_bindings(bind_value_hash);
CREATE INDEX IF NOT EXISTS idx_bindings_heartbeat ON license_bindings(last_heartbeat_at);

CREATE TABLE IF NOT EXISTS renewals (
  id TEXT PRIMARY KEY,
  license_id TEXT NOT NULL,
  old_expired_at TEXT,
  new_expired_at TEXT,
  card_id TEXT NOT NULL,
  duration_days INTEGER NOT NULL,
  created_by TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY (license_id) REFERENCES licenses(id),
  FOREIGN KEY (card_id) REFERENCES cards(id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  actor_type TEXT NOT NULL,
  actor_id TEXT,
  agent_id TEXT,
  product_id TEXT,
  license_id TEXT,
  card_id TEXT,
  binding_id TEXT,
  action TEXT NOT NULL,
  client_ip TEXT,
  user_agent TEXT,
  request_id TEXT,
  result TEXT NOT NULL,
  error_code TEXT,
  extra_json TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_product_created ON audit_logs(product_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_license_created ON audit_logs(license_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_agent_created ON audit_logs(agent_id, created_at);
