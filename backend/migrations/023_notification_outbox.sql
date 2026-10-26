CREATE TABLE IF NOT EXISTS notification_outbox (
 id TEXT PRIMARY KEY, dedup_key TEXT NOT NULL UNIQUE, payload_enc TEXT NOT NULL,
 kind TEXT NOT NULL, status TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0,
 available_at TEXT NOT NULL, lease_until TEXT NOT NULL, last_error TEXT NOT NULL,
 created_at TEXT NOT NULL, completed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_outbox_pending ON notification_outbox(status,available_at);
