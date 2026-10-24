CREATE TABLE IF NOT EXISTS product_license_policies (
  product_id VARCHAR(64) PRIMARY KEY,
  license_mode VARCHAR(32) NOT NULL DEFAULT 'subscription',
  update_duration_days INT NOT NULL DEFAULT 0,
  FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS license_entitlements (
  license_id VARCHAR(64) PRIMARY KEY,
  license_mode VARCHAR(32) NOT NULL DEFAULT 'subscription',
  updates_expired_at VARCHAR(40),
  FOREIGN KEY (license_id) REFERENCES licenses(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
