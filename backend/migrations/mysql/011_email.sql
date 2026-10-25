CREATE TABLE IF NOT EXISTS email_settings (
  id TINYINT PRIMARY KEY,
  enabled TINYINT NOT NULL DEFAULT 0,
  smtp_host VARCHAR(255) NOT NULL DEFAULT '',
  smtp_port INT NOT NULL DEFAULT 465,
  smtp_username VARCHAR(255) NOT NULL DEFAULT '',
  smtp_password_enc TEXT NOT NULL,
  from_address VARCHAR(255) NOT NULL DEFAULT '',
  from_name VARCHAR(80) NOT NULL DEFAULT '',
  encryption VARCHAR(16) NOT NULL DEFAULT 'ssl',
  site_url VARCHAR(255) NOT NULL DEFAULT '',
  updated_at VARCHAR(40) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO email_settings (
  id, enabled, smtp_host, smtp_port, smtp_username, smtp_password_enc, from_address, from_name, encryption, site_url, updated_at
) VALUES (1, 0, '', 465, '', '', '', '', 'ssl', '', '1970-01-01T00:00:00Z');

CREATE TABLE IF NOT EXISTS email_logs (
  id VARCHAR(64) PRIMARY KEY,
  to_address VARCHAR(255) NOT NULL,
  subject VARCHAR(255) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  agent_id VARCHAR(64) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL,
  error TEXT NOT NULL,
  created_at VARCHAR(40) NOT NULL,
  INDEX idx_email_logs_agent(agent_id, created_at),
  INDEX idx_email_logs_created(created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
