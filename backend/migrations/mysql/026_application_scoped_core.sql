-- Application-scoped authorization core. Legacy tables are retained as a
-- read-only archive; production authorization uses only the new scoped tables.
CREATE TABLE IF NOT EXISTS applications (
 application_id VARCHAR(64) PRIMARY KEY,
 name VARCHAR(160) NOT NULL,
 owner_developer_id VARCHAR(96) NOT NULL,
 status VARCHAR(64) NOT NULL,
 app_secret_hash VARCHAR(64) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 INDEX applications_owner(owner_developer_id,status,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_signing_keys (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 kid VARCHAR(160) NOT NULL,
 key_version INTEGER NOT NULL,
 status VARCHAR(64) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 UNIQUE(application_id,kid),
 UNIQUE(application_id,key_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_plans (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 name VARCHAR(160) NOT NULL,
 model VARCHAR(64) NOT NULL,
 enabled INTEGER NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_sales_channels (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 kind VARCHAR(64) NOT NULL,
 enabled INTEGER NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_sales_orders (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 plan_id VARCHAR(96) NOT NULL,
 channel_id VARCHAR(96) NULL,
 external_id VARCHAR(190) NOT NULL,
 customer_ref VARCHAR(190) NOT NULL,
 owner_user_id VARCHAR(96) NULL,
 agent_id VARCHAR(96) NULL,
 status VARCHAR(64) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,plan_id) REFERENCES application_plans(application_id,id),
 FOREIGN KEY(application_id,channel_id) REFERENCES application_sales_channels(application_id,id),
 INDEX app_orders_customer(application_id,customer_ref,created_at,id),
 INDEX app_orders_agent(application_id,agent_id,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_licenses (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 plan_id VARCHAR(96) NOT NULL,
 customer_ref VARCHAR(190) NOT NULL,
 owner_user_id VARCHAR(96) NULL,
 order_id VARCHAR(96) NULL,
 status VARCHAR(64) NOT NULL,
 renewal_only INTEGER NOT NULL,
 binding_type VARCHAR(64) NOT NULL,
 binding_hash VARCHAR(64) NOT NULL,
 expires_at VARCHAR(40) NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,plan_id) REFERENCES application_plans(application_id,id),
 FOREIGN KEY(application_id,order_id) REFERENCES application_sales_orders(application_id,id),
 INDEX app_license_owner(application_id,owner_user_id,created_at,id),
 INDEX app_license_deadline(application_id,status,expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_activations (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 license_id VARCHAR(96) NOT NULL,
 public_key_hash VARCHAR(64) NOT NULL,
 session_id VARCHAR(64) NOT NULL,
 binding_type VARCHAR(64) NOT NULL,
 binding_hash VARCHAR(64) NOT NULL,
 lease_expires_at VARCHAR(40) NULL,
 deactivated_at VARCHAR(40) NULL,
 borrowed INTEGER NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,license_id) REFERENCES application_licenses(application_id,id),
 UNIQUE(application_id,license_id,public_key_hash,session_id),
 INDEX app_activation_live(application_id,license_id,deactivated_at,lease_expires_at),
 INDEX app_activation_history(application_id,license_id,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_card_keys (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 plan_id VARCHAR(96) NOT NULL,
 batch_id VARCHAR(96) NOT NULL,
 key_hash VARCHAR(64) NOT NULL,
 status VARCHAR(64) NOT NULL,
 bound_license_id VARCHAR(96) NOT NULL,
 redeemed_license_id VARCHAR(96) NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,plan_id) REFERENCES application_plans(application_id,id),
 FOREIGN KEY(application_id,bound_license_id) REFERENCES application_licenses(application_id,id),
 FOREIGN KEY(application_id,redeemed_license_id) REFERENCES application_licenses(application_id,id),
 UNIQUE(application_id,key_hash),
 INDEX app_card_renewals(application_id,redeemed_license_id),
 INDEX app_cards_status(application_id,status,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_binding_rules (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 kind VARCHAR(64) NOT NULL,
 enabled INTEGER NOT NULL,
 license_id VARCHAR(96) NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,license_id) REFERENCES application_licenses(application_id,id),
 INDEX app_binding_rules(application_id,kind,enabled,license_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_binding_anchors (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 license_id VARCHAR(96) NOT NULL,
 group_id VARCHAR(96) NOT NULL,
 kind VARCHAR(64) NOT NULL,
 binding_hash VARCHAR(64) NOT NULL,
 status VARCHAR(64) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,license_id) REFERENCES application_licenses(application_id,id),
 UNIQUE(application_id,license_id,kind,binding_hash),
 INDEX app_anchor_groups(application_id,license_id,status,group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_installation_invitations (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 license_id VARCHAR(96) NOT NULL,
 kind VARCHAR(64) NOT NULL,
 token_hash VARCHAR(64) NOT NULL,
 owner_user_id VARCHAR(96) NULL,
 expires_at VARCHAR(40) NOT NULL,
 used_at VARCHAR(40) NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,license_id) REFERENCES application_licenses(application_id,id),
 UNIQUE(application_id,token_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_pending_installations (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 license_id VARCHAR(96) NULL,
 public_key_hash VARCHAR(64) NOT NULL,
 binding_type VARCHAR(64) NOT NULL,
 binding_hash VARCHAR(64) NOT NULL,
 status VARCHAR(64) NOT NULL,
 expires_at VARCHAR(40) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,license_id) REFERENCES application_licenses(application_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_request_nonces (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 credential_id VARCHAR(160) NOT NULL,
 nonce_hash VARCHAR(64) NOT NULL,
 expires_at VARCHAR(40) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 UNIQUE(application_id,credential_id,nonce_hash),
 INDEX app_nonce_expiry(application_id,expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_webhook_receipts (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 channel_id VARCHAR(96) NOT NULL,
 external_id VARCHAR(190) NOT NULL,
 order_id VARCHAR(96) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,channel_id) REFERENCES application_sales_channels(application_id,id),
 FOREIGN KEY(application_id,order_id) REFERENCES application_sales_orders(application_id,id),
 UNIQUE(application_id,channel_id,external_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_usage_receipts (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 license_id VARCHAR(96) NOT NULL,
 activation_id VARCHAR(96) NOT NULL,
 idempotency_key VARCHAR(64) NOT NULL,
 period VARCHAR(64) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,license_id) REFERENCES application_licenses(application_id,id),
 FOREIGN KEY(application_id,activation_id) REFERENCES application_activations(application_id,id),
 UNIQUE(application_id,license_id,idempotency_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_agent_grants (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 agent_id VARCHAR(96) NOT NULL,
 plan_id VARCHAR(96) NOT NULL,
 enabled INTEGER NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,plan_id) REFERENCES application_plans(application_id,id),
 UNIQUE(application_id,agent_id,plan_id),
 INDEX app_grants_agent(application_id,agent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_money_events (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 agent_id VARCHAR(96) NOT NULL,
 order_id VARCHAR(96) NOT NULL,
 request_id VARCHAR(160) NOT NULL,
 kind VARCHAR(64) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 FOREIGN KEY(application_id,order_id) REFERENCES application_sales_orders(application_id,id),
 UNIQUE(application_id,agent_id,request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_releases (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 version VARCHAR(64) NOT NULL,
 target_key VARCHAR(64) NOT NULL,
 size BIGINT NOT NULL,
 status VARCHAR(64) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 UNIQUE(application_id,version,target_key),
 INDEX app_release_status(application_id,status,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS application_audit_events (
 application_id VARCHAR(64) NOT NULL,
 id VARCHAR(96) NOT NULL,
 data_json LONGTEXT NOT NULL,
 created_at VARCHAR(40) NOT NULL,
 updated_at VARCHAR(40) NOT NULL,
 sequence BIGINT NOT NULL,
 event_type VARCHAR(160) NOT NULL,
 reason_code VARCHAR(64) NOT NULL,
 entity_id VARCHAR(160) NOT NULL,
 result VARCHAR(64) NOT NULL,
 PRIMARY KEY(application_id,id),
 FOREIGN KEY(application_id) REFERENCES applications(application_id),
 UNIQUE(application_id,sequence),
 INDEX app_audit_diagnostics(application_id,result,event_type,reason_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;


CREATE TABLE IF NOT EXISTS application_audit_heads (
 application_id VARCHAR(64) PRIMARY KEY,
 sequence BIGINT NOT NULL,
 event_hash VARCHAR(64) NOT NULL,
 FOREIGN KEY(application_id) REFERENCES applications(application_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

DROP TRIGGER IF EXISTS application_audit_no_update;
CREATE TRIGGER application_audit_no_update BEFORE UPDATE ON application_audit_events FOR EACH ROW BEGIN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='application audit is append only'; END;

DROP TRIGGER IF EXISTS application_audit_no_delete;
CREATE TRIGGER application_audit_no_delete BEFORE DELETE ON application_audit_events FOR EACH ROW BEGIN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='application audit is append only'; END;
