CREATE TABLE IF NOT EXISTS product_update_settings (
  product_id VARCHAR(64) PRIMARY KEY,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS product_releases (
  id VARCHAR(64) PRIMARY KEY,
  product_id VARCHAR(64) NOT NULL,
  version VARCHAR(64) NOT NULL,
  notes TEXT NOT NULL,
  status VARCHAR(16) NOT NULL,
  filename VARCHAR(255) NOT NULL,
  size BIGINT NOT NULL,
  sha256 VARCHAR(64) NOT NULL,
  key_version INT NOT NULL,
  manifest_token TEXT NOT NULL,
  created_at VARCHAR(40) NOT NULL,
  UNIQUE KEY product_release_version (product_id, version),
  FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
