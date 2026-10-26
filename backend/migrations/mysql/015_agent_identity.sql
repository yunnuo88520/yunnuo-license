CREATE TABLE IF NOT EXISTS agent_user_identities (
  member_id VARCHAR(64) PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL UNIQUE,
  FOREIGN KEY (member_id) REFERENCES agent_users(id),
  FOREIGN KEY (user_id) REFERENCES customer_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS account_identity_lock (id INT PRIMARY KEY) ENGINE=InnoDB;
INSERT IGNORE INTO account_identity_lock(id) VALUES(1);
