CREATE TABLE IF NOT EXISTS risk_blocks (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  value_hash TEXT NOT NULL,
  value_masked TEXT NOT NULL,
  reason TEXT NOT NULL,
  status TEXT NOT NULL,
  created_by TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(kind, product_id, value_hash)
);

CREATE INDEX IF NOT EXISTS idx_risk_blocks_lookup
  ON risk_blocks(kind, value_hash, status, product_id);
CREATE INDEX IF NOT EXISTS idx_risk_blocks_created
  ON risk_blocks(created_at);

CREATE TABLE IF NOT EXISTS risk_alerts (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL,
  license_id TEXT,
  binding_id TEXT,
  alert_type TEXT NOT NULL,
  severity TEXT NOT NULL,
  status TEXT NOT NULL,
  subject_kind TEXT NOT NULL,
  subject_hash TEXT NOT NULL,
  subject_masked TEXT NOT NULL,
  open_marker TEXT,
  detail TEXT NOT NULL,
  occurrence_count INTEGER NOT NULL DEFAULT 1,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  resolved_at TEXT,
  resolved_by TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_risk_alerts_status
  ON risk_alerts(status, severity, last_seen_at);
CREATE INDEX IF NOT EXISTS idx_risk_alerts_subject
  ON risk_alerts(product_id, alert_type, subject_hash, status);
CREATE UNIQUE INDEX IF NOT EXISTS uq_risk_alerts_open
  ON risk_alerts(product_id, alert_type, subject_hash, open_marker);
