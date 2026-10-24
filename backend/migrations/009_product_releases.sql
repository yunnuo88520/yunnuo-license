CREATE TABLE IF NOT EXISTS product_update_settings (
  product_id TEXT PRIMARY KEY REFERENCES products(id),
  enabled INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS product_releases (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL REFERENCES products(id),
  version TEXT NOT NULL,
  notes TEXT NOT NULL,
  status TEXT NOT NULL,
  filename TEXT NOT NULL,
  size INTEGER NOT NULL,
  sha256 TEXT NOT NULL,
  key_version INTEGER NOT NULL,
  manifest_token TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (product_id, version)
);
