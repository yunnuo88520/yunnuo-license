CREATE TABLE IF NOT EXISTS agent_session_versions (
 user_id VARCHAR(64) PRIMARY KEY, version INT NOT NULL, FOREIGN KEY(user_id) REFERENCES agent_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS account_password_resets (
 id VARCHAR(64) PRIMARY KEY, token_hash VARCHAR(64) NOT NULL UNIQUE, account_kind VARCHAR(32) NOT NULL, user_id VARCHAR(64) NOT NULL,
 credential_hash VARCHAR(64) NOT NULL, expires_at VARCHAR(40) NOT NULL, consumed_at VARCHAR(40), created_by VARCHAR(64) NOT NULL, reason TEXT NOT NULL, created_at VARCHAR(40) NOT NULL,
 INDEX idx_account_resets_subject(account_kind,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
