CREATE TABLE IF NOT EXISTS product_developers (
  product_id TEXT PRIMARY KEY REFERENCES products(id),
  user_id TEXT NOT NULL REFERENCES admin_users(id)
);
CREATE INDEX IF NOT EXISTS idx_product_developers_user ON product_developers(user_id);
