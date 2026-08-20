CREATE TABLE IF NOT EXISTS site_settings (
  id TINYINT PRIMARY KEY,
  site_name VARCHAR(80) NOT NULL,
  browser_title VARCHAR(80) NOT NULL,
  logo_data_url LONGTEXT NOT NULL,
  favicon_data_url LONGTEXT NOT NULL,
  updated_at VARCHAR(40) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO site_settings (
  id, site_name, browser_title, logo_data_url, favicon_data_url, updated_at
) VALUES (1, '允诺云授权', '允诺云授权', '', '', '1970-01-01T00:00:00Z');
