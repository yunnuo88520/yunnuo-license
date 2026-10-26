CREATE TABLE IF NOT EXISTS release_storage_lock(id INT PRIMARY KEY) ENGINE=InnoDB;
INSERT IGNORE INTO release_storage_lock(id) VALUES(1);
