CREATE TABLE IF NOT EXISTS agent_session_versions (user_id TEXT PRIMARY KEY REFERENCES agent_users(id), version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS account_password_resets (
 id TEXT PRIMARY KEY, token_hash TEXT NOT NULL UNIQUE, account_kind TEXT NOT NULL, user_id TEXT NOT NULL,
 credential_hash TEXT NOT NULL, expires_at TEXT NOT NULL, consumed_at TEXT, created_by TEXT NOT NULL, reason TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_account_resets_subject ON account_password_resets(account_kind,user_id);
