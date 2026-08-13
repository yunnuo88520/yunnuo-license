PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS agent_login_codes (
  agent_id TEXT PRIMARY KEY,
  login_code TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  FOREIGN KEY (agent_id) REFERENCES agents(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_login_codes_code ON agent_login_codes(login_code);
