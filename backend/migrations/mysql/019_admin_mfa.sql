CREATE TABLE IF NOT EXISTS admin_mfa (
 user_id VARCHAR(64) PRIMARY KEY, secret_enc TEXT NOT NULL,
 enabled INT NOT NULL, pending_at VARCHAR(40) NOT NULL, last_step BIGINT NOT NULL,
 recovery_hashes TEXT NOT NULL,
 FOREIGN KEY(user_id) REFERENCES admin_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
