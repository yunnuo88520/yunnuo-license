CREATE TABLE IF NOT EXISTS product_developers (
  product_id VARCHAR(64) PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  FOREIGN KEY (product_id) REFERENCES products(id),
  FOREIGN KEY (user_id) REFERENCES admin_users(id),
  INDEX idx_product_developers_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
