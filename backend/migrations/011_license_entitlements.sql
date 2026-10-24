CREATE TABLE IF NOT EXISTS product_license_policies (
  product_id TEXT PRIMARY KEY REFERENCES products(id),
  license_mode TEXT NOT NULL DEFAULT 'subscription',
  update_duration_days INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS license_entitlements (
  license_id TEXT PRIMARY KEY REFERENCES licenses(id),
  license_mode TEXT NOT NULL DEFAULT 'subscription',
  updates_expired_at TEXT
);
