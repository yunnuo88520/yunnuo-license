CREATE TABLE IF NOT EXISTS application_credentials (
  product_id VARCHAR(64) PRIMARY KEY,
  secret_hash VARCHAR(64) NOT NULL,
  version INT NOT NULL,
  updated_at VARCHAR(40) NOT NULL,
  FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
