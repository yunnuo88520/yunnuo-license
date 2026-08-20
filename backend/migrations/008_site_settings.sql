CREATE TABLE IF NOT EXISTS site_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  site_name TEXT NOT NULL,
  browser_title TEXT NOT NULL,
  logo_data_url TEXT NOT NULL DEFAULT '',
  favicon_data_url TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

INSERT OR IGNORE INTO site_settings (
  id, site_name, browser_title, logo_data_url, favicon_data_url, updated_at
) VALUES (1, '允诺云授权', '允诺云授权', '', '', '1970-01-01T00:00:00Z');
