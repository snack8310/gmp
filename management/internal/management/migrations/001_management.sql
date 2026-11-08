CREATE TABLE IF NOT EXISTS gmp_management_schema_migrations (
  migration_id VARCHAR(128) NOT NULL PRIMARY KEY,
  applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS gmp_management_write_lock (
  lock_id TINYINT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=InnoDB;

INSERT IGNORE INTO gmp_management_write_lock (lock_id) VALUES (1);

CREATE TABLE IF NOT EXISTS gmp_management_sequences (
  sequence_name VARCHAR(32) NOT NULL PRIMARY KEY,
  next_value BIGINT UNSIGNED NOT NULL
) ENGINE=InnoDB;

INSERT IGNORE INTO gmp_management_sequences (sequence_name, next_value) VALUES ('campaign', 0), ('treatment', 0);

CREATE TABLE IF NOT EXISTS gmp_management_campaigns (
  campaign_pk BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  id_hash BINARY(32) NOT NULL,
  id_value LONGTEXT NOT NULL,
  name LONGTEXT NOT NULL,
  audience_type VARCHAR(16) NULL,
  audience_group_id LONGTEXT NULL,
  audience_bundle_id LONGTEXT NULL,
  UNIQUE KEY uq_gmp_management_campaign_hash (id_hash)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS gmp_management_treatments (
  treatment_pk BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  campaign_pk BIGINT UNSIGNED NOT NULL,
  id_hash BINARY(32) NOT NULL,
  id_value LONGTEXT NOT NULL,
  name LONGTEXT NOT NULL,
  action_id LONGTEXT NOT NULL,
  slot_id LONGTEXT NULL,
  UNIQUE KEY uq_gmp_management_treatment_hash (campaign_pk, id_hash),
  CONSTRAINT fk_gmp_management_treatment_campaign
    FOREIGN KEY (campaign_pk) REFERENCES gmp_management_campaigns (campaign_pk) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS gmp_management_flows (
  campaign_pk BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  flow_id LONGTEXT NOT NULL,
  CONSTRAINT fk_gmp_management_flow_campaign
    FOREIGN KEY (campaign_pk) REFERENCES gmp_management_campaigns (campaign_pk) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS gmp_management_flow_entries (
  entry_pk BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  campaign_pk BIGINT UNSIGNED NOT NULL,
  entry_order INT UNSIGNED NOT NULL,
  group_id LONGTEXT NOT NULL,
  kind VARCHAR(16) NOT NULL,
  CONSTRAINT fk_gmp_management_entry_campaign
    FOREIGN KEY (campaign_pk) REFERENCES gmp_management_campaigns (campaign_pk) ON DELETE CASCADE,
  UNIQUE KEY uq_gmp_management_entry_order (campaign_pk, entry_order)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS gmp_management_flow_entry_treatments (
  entry_pk BIGINT UNSIGNED NOT NULL,
  treatment_order INT UNSIGNED NOT NULL,
  treatment_id LONGTEXT NOT NULL,
  PRIMARY KEY (entry_pk, treatment_order),
  CONSTRAINT fk_gmp_management_entry_treatment_entry
    FOREIGN KEY (entry_pk) REFERENCES gmp_management_flow_entries (entry_pk) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS gmp_management_flow_edges (
  edge_pk BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  campaign_pk BIGINT UNSIGNED NOT NULL,
  edge_order INT UNSIGNED NOT NULL,
  id_value LONGTEXT NOT NULL,
  from_treatment_id LONGTEXT NOT NULL,
  to_treatment_id LONGTEXT NOT NULL,
  condition_label LONGTEXT NOT NULL,
  CONSTRAINT fk_gmp_management_edge_campaign
    FOREIGN KEY (campaign_pk) REFERENCES gmp_management_campaigns (campaign_pk) ON DELETE CASCADE,
  UNIQUE KEY uq_gmp_management_edge_order (campaign_pk, edge_order)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS gmp_management_audience_references (
  reference_pk BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  identity_hash BINARY(32) NOT NULL,
  reference_type VARCHAR(16) NOT NULL,
  bundle_id LONGTEXT NOT NULL,
  group_id LONGTEXT NOT NULL,
  name LONGTEXT NOT NULL,
  bundle_name LONGTEXT NOT NULL,
  available BOOLEAN NOT NULL,
  UNIQUE KEY uq_gmp_management_audience_reference_hash (identity_hash)
) ENGINE=InnoDB;
