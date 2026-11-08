CREATE TABLE IF NOT EXISTS gmp_management_execution_records (
  execution_hash BINARY(32) NOT NULL PRIMARY KEY,
  execution_key LONGTEXT NOT NULL,
  campaign_id LONGTEXT NOT NULL,
  treatment_id LONGTEXT NOT NULL,
  uid LONGTEXT NOT NULL,
  action_id LONGTEXT NOT NULL,
  placements JSON NOT NULL,
  state VARCHAR(32) NOT NULL,
  attempts INT NOT NULL,
  external_ref LONGTEXT NOT NULL
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS gmp_management_execution_receipts (
  execution_hash BINARY(32) NOT NULL PRIMARY KEY,
  arrived BOOLEAN NOT NULL,
  outcome LONGTEXT NOT NULL,
  external_ref LONGTEXT NOT NULL,
  attributes JSON NOT NULL,
  CONSTRAINT fk_gmp_management_execution_receipt_record
    FOREIGN KEY (execution_hash) REFERENCES gmp_management_execution_records (execution_hash) ON DELETE CASCADE
) ENGINE=InnoDB;
