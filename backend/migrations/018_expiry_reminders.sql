CREATE TABLE IF NOT EXISTS license_expiry_reminders (
  id TEXT PRIMARY KEY,
  license_id TEXT NOT NULL REFERENCES licenses(id),
  user_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  expiry_value TEXT NOT NULL,
  sent_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(license_id, kind, expiry_value)
);

CREATE INDEX IF NOT EXISTS idx_expiry_reminders_user ON license_expiry_reminders(user_id);
