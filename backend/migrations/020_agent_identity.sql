CREATE TABLE IF NOT EXISTS agent_user_identities (
  member_id TEXT PRIMARY KEY REFERENCES agent_users(id),
  user_id TEXT NOT NULL UNIQUE REFERENCES customer_users(id)
);
CREATE TABLE IF NOT EXISTS account_identity_lock (id INTEGER PRIMARY KEY);
INSERT OR IGNORE INTO account_identity_lock(id) VALUES(1);
