CREATE TABLE gmp_management_experiment_colorings (
    assignment_hash BINARY(32) NOT NULL,
    assignment_id LONGTEXT NOT NULL,
    key_hash BINARY(32) NOT NULL,
    assignment_key LONGTEXT NOT NULL,
    bucket INT UNSIGNED NOT NULL,
    algorithm LONGTEXT NOT NULL,
    created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (assignment_hash, key_hash)
) ENGINE=InnoDB;
