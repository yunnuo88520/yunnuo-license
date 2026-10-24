CREATE TABLE IF NOT EXISTS product_security_policies (
 product_id VARCHAR(64) PRIMARY KEY, enabled INT NOT NULL, detect_mismatch INT NOT NULL, accept_reports INT NOT NULL,
 action VARCHAR(32) NOT NULL, message TEXT NOT NULL, support_url TEXT NOT NULL, FOREIGN KEY(product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS security_cases (
 id VARCHAR(64) PRIMARY KEY, product_id VARCHAR(64) NOT NULL, license_id VARCHAR(64) NOT NULL,
 signal_type VARCHAR(64) NOT NULL, subject TEXT NOT NULL, evidence TEXT NOT NULL, status VARCHAR(32) NOT NULL, action VARCHAR(32) NOT NULL,
 occurrences INT NOT NULL, first_seen VARCHAR(40) NOT NULL, last_seen VARCHAR(40) NOT NULL, review_reason TEXT NOT NULL, reviewer VARCHAR(64) NOT NULL,
 UNIQUE(product_id,license_id,signal_type), FOREIGN KEY(product_id) REFERENCES products(id), FOREIGN KEY(license_id) REFERENCES licenses(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS security_appeals (
 id VARCHAR(64) PRIMARY KEY, case_id VARCHAR(64) NOT NULL, user_id VARCHAR(64) NOT NULL,
 reason TEXT NOT NULL, status VARCHAR(32) NOT NULL, created_at VARCHAR(40) NOT NULL, UNIQUE(case_id,user_id),
 FOREIGN KEY(case_id) REFERENCES security_cases(id), FOREIGN KEY(user_id) REFERENCES customer_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
