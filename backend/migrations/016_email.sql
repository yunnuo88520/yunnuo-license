CREATE TABLE IF NOT EXISTS email_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  enabled INTEGER NOT NULL DEFAULT 0,
  smtp_host TEXT NOT NULL DEFAULT '',
  smtp_port INTEGER NOT NULL DEFAULT 465,
  smtp_username TEXT NOT NULL DEFAULT '',
  smtp_password_enc TEXT NOT NULL DEFAULT '',
  from_address TEXT NOT NULL DEFAULT '',
  from_name TEXT NOT NULL DEFAULT '',
  encryption TEXT NOT NULL DEFAULT 'ssl',
  site_url TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
INSERT OR IGNORE INTO email_settings (
  id, enabled, smtp_host, smtp_port, smtp_username, smtp_password_enc, from_address, from_name, encryption, site_url, updated_at
) VALUES (1, 0, '', 465, '', '', '', '', 'ssl', '', '1970-01-01T00:00:00Z');

CREATE TABLE IF NOT EXISTS email_logs (
  id TEXT PRIMARY KEY,
  to_address TEXT NOT NULL,
  subject TEXT NOT NULL,
  kind TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_email_logs_agent ON email_logs(agent_id, created_at);
CREATE INDEX IF NOT EXISTS idx_email_logs_created ON email_logs(created_at);
