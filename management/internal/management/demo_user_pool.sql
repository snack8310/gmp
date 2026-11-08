CREATE TABLE IF NOT EXISTS gmp_demo_user_pool (
  member_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  uid VARCHAR(128) NOT NULL,
  region VARCHAR(64) NOT NULL,
  lifecycle VARCHAR(32) NOT NULL,
  inactive_days INT NOT NULL,
  tier VARCHAR(32) NOT NULL,
  UNIQUE KEY uq_gmp_demo_user_pool_uid (uid)
) ENGINE=InnoDB;

INSERT IGNORE INTO gmp_demo_user_pool (uid, region, lifecycle, inactive_days, tier) VALUES
  ('uid_demo_001', 'north', 'active', 0, 'standard'),
  ('uid_demo_002', 'south', 'cooling', 8, 'silver'),
  ('uid_demo_003', 'east', 'inactive', 31, 'gold'),
  ('uid_demo_004', 'west', 'active', 2, 'standard');
