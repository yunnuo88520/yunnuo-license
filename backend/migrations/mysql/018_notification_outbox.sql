CREATE TABLE IF NOT EXISTS notification_outbox (
 id VARCHAR(80) PRIMARY KEY, dedup_key VARCHAR(191) NOT NULL UNIQUE, payload_enc MEDIUMTEXT NOT NULL,
 kind VARCHAR(40) NOT NULL, status VARCHAR(20) NOT NULL, attempts INT NOT NULL DEFAULT 0,
 available_at VARCHAR(40) NOT NULL, lease_until VARCHAR(40) NOT NULL, last_error VARCHAR(100) NOT NULL,
 created_at VARCHAR(40) NOT NULL, completed_at VARCHAR(40) NULL,
 INDEX idx_outbox_pending(status,available_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
