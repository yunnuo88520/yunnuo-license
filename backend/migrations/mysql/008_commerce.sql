CREATE TABLE IF NOT EXISTS product_groups (id VARCHAR(64) PRIMARY KEY, name VARCHAR(160) NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS product_group_members (
 group_id VARCHAR(64) NOT NULL, product_id VARCHAR(64) NOT NULL, PRIMARY KEY(group_id,product_id), FOREIGN KEY(group_id) REFERENCES product_groups(id), FOREIGN KEY(product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS card_packages (
 id VARCHAR(64) PRIMARY KEY, product_id VARCHAR(64) NOT NULL, name VARCHAR(160) NOT NULL,
 duration_days INT NOT NULL, is_permanent INT NOT NULL, base_cost BIGINT NOT NULL, use_quota INT NOT NULL, enabled INT NOT NULL,
 FOREIGN KEY(product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS agent_tiers (id VARCHAR(64) PRIMARY KEY, name VARCHAR(160) NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS agent_tier_prices (
 tier_id VARCHAR(64) NOT NULL, package_id VARCHAR(64) NOT NULL, unit_cost BIGINT NOT NULL, PRIMARY KEY(tier_id,package_id), FOREIGN KEY(tier_id) REFERENCES agent_tiers(id), FOREIGN KEY(package_id) REFERENCES card_packages(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS agent_accounts (
 agent_id VARCHAR(64) PRIMARY KEY, tier_id VARCHAR(64) NOT NULL DEFAULT '', balance BIGINT NOT NULL DEFAULT 0, FOREIGN KEY(agent_id) REFERENCES agents(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS agent_package_grants (
 agent_id VARCHAR(64) NOT NULL, package_id VARCHAR(64) NOT NULL, enabled INT NOT NULL, override_cost BIGINT,
 PRIMARY KEY(agent_id,package_id), FOREIGN KEY(agent_id) REFERENCES agents(id), FOREIGN KEY(package_id) REFERENCES card_packages(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS agent_money_ledgers (
 id VARCHAR(64) PRIMARY KEY, agent_id VARCHAR(64) NOT NULL, request_id VARCHAR(64) NOT NULL, kind VARCHAR(32) NOT NULL,
 amount BIGINT NOT NULL, balance_after BIGINT NOT NULL, batch_id VARCHAR(64) NOT NULL, package_id VARCHAR(64) NOT NULL,
 unit_cost BIGINT NOT NULL, quantity INT NOT NULL, remark TEXT NOT NULL, actor_id VARCHAR(64) NOT NULL, created_at VARCHAR(40) NOT NULL,
 UNIQUE(agent_id,request_id), FOREIGN KEY(agent_id) REFERENCES agents(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS agent_package_orders (
 agent_id VARCHAR(64) NOT NULL, request_id VARCHAR(64) NOT NULL, fingerprint TEXT NOT NULL,
 batch_id VARCHAR(64) NOT NULL, PRIMARY KEY(agent_id,request_id), FOREIGN KEY(agent_id) REFERENCES agents(id), FOREIGN KEY(batch_id) REFERENCES card_batches(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
