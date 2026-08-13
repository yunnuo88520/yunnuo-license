CREATE TABLE IF NOT EXISTS products (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  code VARCHAR(64) NOT NULL UNIQUE,
  app_key VARCHAR(128) NOT NULL UNIQUE,
  public_key_pem TEXT NOT NULL,
  private_key_encrypted TEXT NOT NULL,
  bind_mode VARCHAR(32) NOT NULL,
  max_bind_count INT NOT NULL,
  bind_conflict_strategy VARCHAR(32) NOT NULL,
  offline_mode VARCHAR(32) NOT NULL,
  offline_grace_days INT NOT NULL,
  expire_grace_days INT NOT NULL,
  unbind_limit INT NOT NULL,
  unbind_cooldown_hours INT NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS card_batches (
  id VARCHAR(64) PRIMARY KEY,
  product_id VARCHAR(64) NOT NULL,
  agent_id VARCHAR(64),
  name VARCHAR(255) NOT NULL,
  quantity INT NOT NULL,
  duration_days INT NOT NULL,
  is_permanent TINYINT(1) NOT NULL,
  source VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  export_count INT NOT NULL DEFAULT 0,
  created_by VARCHAR(64),
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_batches_product FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS cards (
  id VARCHAR(64) PRIMARY KEY,
  product_id VARCHAR(64) NOT NULL,
  batch_id VARCHAR(64) NOT NULL,
  agent_id VARCHAR(64),
  code_hash VARCHAR(128) NOT NULL UNIQUE,
  code_encrypted TEXT NOT NULL,
  code_prefix VARCHAR(64) NOT NULL,
  duration_days INT NOT NULL,
  is_permanent TINYINT(1) NOT NULL,
  status VARCHAR(32) NOT NULL,
  order_no VARCHAR(128),
  activated_license_id VARCHAR(64),
  activated_at VARCHAR(40),
  consumed_at VARCHAR(40),
  voided_at VARCHAR(40),
  void_reason TEXT,
  created_by VARCHAR(64),
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_cards_product FOREIGN KEY (product_id) REFERENCES products(id),
  CONSTRAINT fk_cards_batch FOREIGN KEY (batch_id) REFERENCES card_batches(id),
  INDEX idx_cards_product_status (product_id, status),
  INDEX idx_cards_agent_status (agent_id, status),
  INDEX idx_cards_batch (product_id, batch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS licenses (
  id VARCHAR(64) PRIMARY KEY,
  license_no VARCHAR(128) NOT NULL UNIQUE,
  product_id VARCHAR(64) NOT NULL,
  card_id VARCHAR(64) NOT NULL UNIQUE,
  agent_id VARCHAR(64),
  customer_id VARCHAR(128),
  account_ref VARCHAR(255),
  status VARCHAR(32) NOT NULL,
  issued_at VARCHAR(40) NOT NULL,
  activated_at VARCHAR(40) NOT NULL,
  expired_at VARCHAR(40),
  last_verify_at VARCHAR(40),
  last_heartbeat_at VARCHAR(40),
  revoked_at VARCHAR(40),
  revoked_reason TEXT,
  offline_token_version INT NOT NULL DEFAULT 1,
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_licenses_product FOREIGN KEY (product_id) REFERENCES products(id),
  CONSTRAINT fk_licenses_card FOREIGN KEY (card_id) REFERENCES cards(id),
  INDEX idx_licenses_product_status (product_id, status),
  INDEX idx_licenses_agent_status (agent_id, status),
  INDEX idx_licenses_expired_at (expired_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS license_bindings (
  id VARCHAR(64) PRIMARY KEY,
  license_id VARCHAR(64) NOT NULL,
  product_id VARCHAR(64) NOT NULL,
  bind_mode VARCHAR(32) NOT NULL,
  bind_value_hash VARCHAR(128) NOT NULL,
  bind_value_encrypted TEXT NOT NULL,
  display_name VARCHAR(255),
  status VARCHAR(32) NOT NULL,
  first_seen_ip VARCHAR(64),
  last_seen_ip VARCHAR(64),
  user_agent TEXT,
  last_heartbeat_at VARCHAR(40),
  activated_at VARCHAR(40) NOT NULL,
  unbound_at VARCHAR(40),
  kicked_at VARCHAR(40),
  revoked_at VARCHAR(40),
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_bindings_license FOREIGN KEY (license_id) REFERENCES licenses(id),
  CONSTRAINT fk_bindings_product FOREIGN KEY (product_id) REFERENCES products(id),
  UNIQUE KEY uq_binding_identity (license_id, bind_mode, bind_value_hash),
  INDEX idx_bindings_license_status (license_id, status),
  INDEX idx_bindings_value_hash (bind_value_hash),
  INDEX idx_bindings_heartbeat (last_heartbeat_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS renewals (
  id VARCHAR(64) PRIMARY KEY,
  license_id VARCHAR(64) NOT NULL,
  old_expired_at VARCHAR(40),
  new_expired_at VARCHAR(40),
  card_id VARCHAR(64) NOT NULL,
  duration_days INT NOT NULL,
  created_by VARCHAR(64),
  created_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_renewals_license FOREIGN KEY (license_id) REFERENCES licenses(id),
  CONSTRAINT fk_renewals_card FOREIGN KEY (card_id) REFERENCES cards(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS audit_logs (
  id VARCHAR(64) PRIMARY KEY,
  actor_type VARCHAR(32) NOT NULL,
  actor_id VARCHAR(64),
  agent_id VARCHAR(64),
  product_id VARCHAR(64),
  license_id VARCHAR(64),
  card_id VARCHAR(64),
  binding_id VARCHAR(64),
  action VARCHAR(128) NOT NULL,
  client_ip VARCHAR(64),
  user_agent TEXT,
  request_id VARCHAR(128),
  result VARCHAR(32) NOT NULL,
  error_code VARCHAR(128),
  extra_json LONGTEXT,
  created_at VARCHAR(40) NOT NULL,
  INDEX idx_audit_product_created (product_id, created_at),
  INDEX idx_audit_license_created (license_id, created_at),
  INDEX idx_audit_agent_created (agent_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS agents (
  id VARCHAR(64) PRIMARY KEY,
  agent_no VARCHAR(128) NOT NULL UNIQUE,
  parent_agent_id VARCHAR(64),
  name VARCHAR(255) NOT NULL,
  contact_name VARCHAR(255),
  phone VARCHAR(64),
  email VARCHAR(255),
  level INT NOT NULL DEFAULT 1,
  status VARCHAR(32) NOT NULL,
  settlement_mode VARCHAR(32) NOT NULL,
  default_discount_rate DECIMAL(10,4) NOT NULL DEFAULT 1,
  credit_limit INT NOT NULL DEFAULT 0,
  remark TEXT,
  created_by VARCHAR(64),
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  disabled_at VARCHAR(40),
  CONSTRAINT fk_agents_parent FOREIGN KEY (parent_agent_id) REFERENCES agents(id),
  INDEX idx_agents_parent (parent_agent_id),
  INDEX idx_agents_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS agent_users (
  id VARCHAR(64) PRIMARY KEY,
  agent_id VARCHAR(64) NOT NULL,
  username VARCHAR(128) NOT NULL,
  password_hash TEXT NOT NULL,
  display_name VARCHAR(255),
  phone VARCHAR(64),
  email VARCHAR(255),
  role VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  last_login_at VARCHAR(40),
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_agent_users_agent FOREIGN KEY (agent_id) REFERENCES agents(id),
  UNIQUE KEY uq_agent_username (agent_id, username),
  INDEX idx_agent_users_agent_status (agent_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS agent_product_policies (
  id VARCHAR(64) PRIMARY KEY,
  agent_id VARCHAR(64) NOT NULL,
  product_id VARCHAR(64) NOT NULL,
  can_generate TINYINT(1) NOT NULL,
  can_export_plain_code TINYINT(1) NOT NULL,
  allowed_duration_days TEXT NOT NULL,
  allow_permanent TINYINT(1) NOT NULL,
  max_batch_quantity INT NOT NULL,
  discount_rate DECIMAL(10,4) NOT NULL DEFAULT 1,
  settlement_price INT NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL,
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_policies_agent FOREIGN KEY (agent_id) REFERENCES agents(id),
  CONSTRAINT fk_policies_product FOREIGN KEY (product_id) REFERENCES products(id),
  UNIQUE KEY uq_agent_product_policy (agent_id, product_id),
  INDEX idx_agent_policies_product_status (product_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS agent_quota_ledgers (
  id VARCHAR(64) PRIMARY KEY,
  agent_id VARCHAR(64) NOT NULL,
  product_id VARCHAR(64) NOT NULL,
  duration_days INT NOT NULL,
  is_permanent TINYINT(1) NOT NULL,
  change_type VARCHAR(32) NOT NULL,
  change_quantity INT NOT NULL,
  balance_after INT NOT NULL,
  related_batch_id VARCHAR(64),
  related_card_id VARCHAR(64),
  operator_type VARCHAR(32) NOT NULL,
  operator_id VARCHAR(64),
  remark TEXT,
  created_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_quota_agent FOREIGN KEY (agent_id) REFERENCES agents(id),
  CONSTRAINT fk_quota_product FOREIGN KEY (product_id) REFERENCES products(id),
  CONSTRAINT fk_quota_batch FOREIGN KEY (related_batch_id) REFERENCES card_batches(id),
  CONSTRAINT fk_quota_card FOREIGN KEY (related_card_id) REFERENCES cards(id),
  INDEX idx_agent_quota_scope (agent_id, product_id, duration_days, is_permanent),
  INDEX idx_agent_quota_batch (related_batch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS admin_users (
  id VARCHAR(64) PRIMARY KEY,
  username VARCHAR(128) NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  display_name VARCHAR(255),
  role VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  session_version INT NOT NULL DEFAULT 1,
  last_login_at VARCHAR(40),
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  INDEX idx_admin_users_status_role (status, role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS agent_login_codes (
  agent_id VARCHAR(64) PRIMARY KEY,
  login_code VARCHAR(32) NOT NULL UNIQUE,
  created_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_login_codes_agent FOREIGN KEY (agent_id) REFERENCES agents(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS offline_licenses (
  id VARCHAR(64) PRIMARY KEY,
  license_no VARCHAR(128) NOT NULL UNIQUE,
  product_id VARCHAR(64) NOT NULL,
  label VARCHAR(255),
  machine_code_hash VARCHAR(128) NOT NULL,
  machine_code_encrypted TEXT NOT NULL,
  machine_code_masked VARCHAR(255) NOT NULL,
  signed_token_encrypted LONGTEXT NOT NULL,
  token_version INT NOT NULL DEFAULT 1,
  status VARCHAR(32) NOT NULL,
  issued_at VARCHAR(40) NOT NULL,
  expired_at VARCHAR(40),
  revoked_at VARCHAR(40),
  revoked_reason TEXT,
  created_by VARCHAR(64),
  created_at VARCHAR(40) NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  CONSTRAINT fk_offline_product FOREIGN KEY (product_id) REFERENCES products(id),
  INDEX idx_offline_licenses_product_status (product_id, status, created_at),
  INDEX idx_offline_licenses_machine (product_id, machine_code_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS product_keys (
  product_id VARCHAR(64) NOT NULL,
  key_version INT NOT NULL,
  public_key_pem TEXT NOT NULL,
  created_by VARCHAR(64),
  created_at VARCHAR(40) NOT NULL,
  PRIMARY KEY (product_id, key_version),
  CONSTRAINT fk_product_keys_product FOREIGN KEY (product_id) REFERENCES products(id),
  INDEX idx_product_keys_created (product_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO product_keys (product_id, key_version, public_key_pem, created_by, created_at)
SELECT id, 1, public_key_pem, 'migration', created_at FROM products;
