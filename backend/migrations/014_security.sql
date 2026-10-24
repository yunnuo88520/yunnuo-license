CREATE TABLE IF NOT EXISTS product_security_policies (
 product_id TEXT PRIMARY KEY REFERENCES products(id), enabled INTEGER NOT NULL, detect_mismatch INTEGER NOT NULL,
 accept_reports INTEGER NOT NULL, action TEXT NOT NULL, message TEXT NOT NULL, support_url TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS security_cases (
 id TEXT PRIMARY KEY, product_id TEXT NOT NULL REFERENCES products(id), license_id TEXT NOT NULL REFERENCES licenses(id),
 signal_type TEXT NOT NULL, subject TEXT NOT NULL, evidence TEXT NOT NULL, status TEXT NOT NULL, action TEXT NOT NULL,
 occurrences INTEGER NOT NULL, first_seen TEXT NOT NULL, last_seen TEXT NOT NULL, review_reason TEXT NOT NULL, reviewer TEXT NOT NULL,
 UNIQUE(product_id,license_id,signal_type)
);
CREATE TABLE IF NOT EXISTS security_appeals (
 id TEXT PRIMARY KEY, case_id TEXT NOT NULL REFERENCES security_cases(id), user_id TEXT NOT NULL REFERENCES customer_users(id),
 reason TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE(case_id,user_id)
);
