CREATE TABLE IF NOT EXISTS admin_mfa (
 user_id TEXT PRIMARY KEY REFERENCES admin_users(id), secret_enc TEXT NOT NULL,
 enabled INTEGER NOT NULL, pending_at TEXT NOT NULL, last_step INTEGER NOT NULL,
 recovery_hashes TEXT NOT NULL
);
