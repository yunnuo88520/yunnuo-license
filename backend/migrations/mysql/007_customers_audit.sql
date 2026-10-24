CREATE TABLE IF NOT EXISTS customer_users (
 id VARCHAR(64) PRIMARY KEY, username VARCHAR(255) NOT NULL UNIQUE, password_hash TEXT NOT NULL,
 display_name VARCHAR(255) NOT NULL, status VARCHAR(255) NOT NULL, session_version INT NOT NULL, created_at VARCHAR(40) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS license_owners (
 license_id VARCHAR(64) PRIMARY KEY, user_id VARCHAR(64) NOT NULL, assigned_at VARCHAR(40) NOT NULL, INDEX idx_license_owners_user (user_id), FOREIGN KEY (license_id) REFERENCES licenses(id), FOREIGN KEY (user_id) REFERENCES customer_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS product_binding_policies (
 product_id VARCHAR(64) NOT NULL, bind_mode VARCHAR(32) NOT NULL,
 allowed INT NOT NULL DEFAULT 0, change_limit INT NOT NULL DEFAULT 0, cooldown_hours INT NOT NULL DEFAULT 0,
 allow_owner_transfer INT NOT NULL DEFAULT 0, PRIMARY KEY(product_id,bind_mode), FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS binding_changes (
 id VARCHAR(64) PRIMARY KEY, license_id VARCHAR(64) NOT NULL, binding_id VARCHAR(64) NOT NULL,
 request_id VARCHAR(64) NOT NULL, value_hash VARCHAR(96) NOT NULL, kind VARCHAR(255) NOT NULL, created_at VARCHAR(40) NOT NULL,
 FOREIGN KEY (license_id) REFERENCES licenses(id), UNIQUE(license_id,request_id), INDEX idx_binding_changes_license (license_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS operation_logs (
 id VARCHAR(64) PRIMARY KEY, request_id VARCHAR(64) NOT NULL, category VARCHAR(96) NOT NULL,
 actor_type VARCHAR(96) NOT NULL, actor_id VARCHAR(64) NOT NULL, role VARCHAR(96) NOT NULL,
 object_id VARCHAR(96) NOT NULL, method VARCHAR(96) NOT NULL, route TEXT NOT NULL, client_ip VARCHAR(96) NOT NULL,
 http_status INT NOT NULL, result VARCHAR(96) NOT NULL, error_code VARCHAR(96) NOT NULL, created_at VARCHAR(40) NOT NULL, INDEX idx_operation_logs_created (created_at), INDEX idx_operation_logs_category (category,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


