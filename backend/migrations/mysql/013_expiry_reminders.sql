CREATE TABLE IF NOT EXISTS license_expiry_reminders (
  id VARCHAR(64) PRIMARY KEY,
  license_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  expiry_value VARCHAR(40) NOT NULL,
  sent_at VARCHAR(40) NOT NULL,
  created_at VARCHAR(40) NOT NULL,
  UNIQUE KEY uq_expiry_reminder (license_id, kind, expiry_value),
  INDEX idx_expiry_reminders_user (user_id),
  FOREIGN KEY (license_id) REFERENCES licenses(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
