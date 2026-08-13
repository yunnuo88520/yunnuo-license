PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS product_keys (
  product_id TEXT NOT NULL,
  key_version INTEGER NOT NULL,
  public_key_pem TEXT NOT NULL,
  created_by TEXT,
  created_at TEXT NOT NULL,
  PRIMARY KEY (product_id, key_version),
  FOREIGN KEY (product_id) REFERENCES products(id)
);

CREATE INDEX IF NOT EXISTS idx_product_keys_created
  ON product_keys(product_id, created_at DESC);

INSERT OR IGNORE INTO product_keys (
  product_id, key_version, public_key_pem, created_by, created_at
)
SELECT id, 1, public_key_pem, 'migration', created_at
FROM products;
