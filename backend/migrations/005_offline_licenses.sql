PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS offline_licenses (
  id TEXT PRIMARY KEY,
  license_no TEXT NOT NULL UNIQUE,
  product_id TEXT NOT NULL,
  label TEXT,
  machine_code_hash TEXT NOT NULL,
  machine_code_encrypted TEXT NOT NULL,
  machine_code_masked TEXT NOT NULL,
  signed_token_encrypted TEXT NOT NULL,
  token_version INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL,
  issued_at TEXT NOT NULL,
  expired_at TEXT,
  revoked_at TEXT,
  revoked_reason TEXT,
  created_by TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (product_id) REFERENCES products(id)
);

CREATE INDEX IF NOT EXISTS idx_offline_licenses_product_status
  ON offline_licenses(product_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_offline_licenses_machine
  ON offline_licenses(product_id, machine_code_hash);
