CREATE TABLE IF NOT EXISTS authorization_snapshots (
  scope VARCHAR(16) NOT NULL,
  resource_id VARCHAR(64) NOT NULL,
  terms_json TEXT NOT NULL,
  PRIMARY KEY(scope,resource_id)
);
CREATE TABLE IF NOT EXISTS license_installations (
  license_id VARCHAR(64) NOT NULL,
  instance_id VARCHAR(64) NOT NULL,
  binding_id VARCHAR(64) NOT NULL,
  secret_hash VARCHAR(64) NOT NULL,
  device_hash VARCHAR(64) NOT NULL,
  label VARCHAR(160) NOT NULL,
  status VARCHAR(16) NOT NULL,
  last_seen_at VARCHAR(40) NOT NULL,
  created_at VARCHAR(40) NOT NULL,
  PRIMARY KEY(license_id,instance_id)
);
CREATE TABLE IF NOT EXISTS license_seat_leases (
  id VARCHAR(64) PRIMARY KEY,
  license_id VARCHAR(64) NOT NULL,
  instance_id VARCHAR(64) NOT NULL,
  session_id VARCHAR(64) NOT NULL,
  expires_at VARCHAR(40) NOT NULL,
  borrowed INTEGER NOT NULL,
  UNIQUE(license_id,instance_id,session_id)
);
CREATE TABLE IF NOT EXISTS license_claim_tickets (
  token_hash VARCHAR(64) PRIMARY KEY,
  license_id VARCHAR(64) NOT NULL,
  expires_at VARCHAR(40) NOT NULL,
  claimed_by VARCHAR(64) NOT NULL
);
CREATE TABLE IF NOT EXISTS release_targets (
  release_id VARCHAR(64) PRIMARY KEY,
  version VARCHAR(32) NOT NULL,
  target_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS binding_subjects (
  binding_id VARCHAR(64) NOT NULL,
  kind VARCHAR(24) NOT NULL,
  subject_hash VARCHAR(64) NOT NULL,
  PRIMARY KEY(binding_id,kind,subject_hash)
);
CREATE INDEX IF NOT EXISTS binding_subject_lookup ON binding_subjects(kind,subject_hash);
