CREATE TABLE IF NOT EXISTS customer_users (
 id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
 display_name TEXT NOT NULL, status TEXT NOT NULL, session_version INTEGER NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS license_owners (
 license_id TEXT PRIMARY KEY REFERENCES licenses(id), user_id TEXT NOT NULL REFERENCES customer_users(id), assigned_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_license_owners_user ON license_owners(user_id);
CREATE TABLE IF NOT EXISTS product_binding_policies (
 product_id TEXT NOT NULL REFERENCES products(id), bind_mode TEXT NOT NULL,
 allowed INTEGER NOT NULL DEFAULT 0, change_limit INTEGER NOT NULL DEFAULT 0, cooldown_hours INTEGER NOT NULL DEFAULT 0,
 allow_owner_transfer INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(product_id,bind_mode)
);
CREATE TABLE IF NOT EXISTS binding_changes (
 id TEXT PRIMARY KEY, license_id TEXT NOT NULL REFERENCES licenses(id), binding_id TEXT NOT NULL,
 request_id TEXT NOT NULL, value_hash TEXT NOT NULL, kind TEXT NOT NULL, created_at TEXT NOT NULL,
 UNIQUE(license_id,request_id)
);
CREATE INDEX IF NOT EXISTS idx_binding_changes_license ON binding_changes(license_id,created_at);
CREATE TABLE IF NOT EXISTS operation_logs (
 id TEXT PRIMARY KEY, request_id TEXT NOT NULL, category TEXT NOT NULL,
 actor_type TEXT NOT NULL, actor_id TEXT NOT NULL, role TEXT NOT NULL,
 object_id TEXT NOT NULL, method TEXT NOT NULL, route TEXT NOT NULL, client_ip TEXT NOT NULL,
 http_status INTEGER NOT NULL, result TEXT NOT NULL, error_code TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_operation_logs_created ON operation_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_operation_logs_category ON operation_logs(category,created_at);
