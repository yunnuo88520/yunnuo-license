CREATE TABLE IF NOT EXISTS captcha_settings (
  id TINYINT PRIMARY KEY,
  login_enabled TINYINT NOT NULL DEFAULT 0,
  updated_at VARCHAR(40) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO captcha_settings (id, login_enabled, updated_at) VALUES (1, 0, '1970-01-01T00:00:00Z');

CREATE TABLE IF NOT EXISTS captcha_challenges (
  id VARCHAR(64) PRIMARY KEY,
  code_hash CHAR(64) NOT NULL,
  expires_at VARCHAR(40) NOT NULL,
  consumed_at VARCHAR(40),
  INDEX idx_captcha_challenges_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
