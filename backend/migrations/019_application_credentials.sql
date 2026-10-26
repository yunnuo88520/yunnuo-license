CREATE TABLE IF NOT EXISTS application_credentials (
  product_id TEXT PRIMARY KEY REFERENCES products(id),
  secret_hash TEXT NOT NULL,
  version INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);
