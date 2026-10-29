-- Phase 1 schema. Execute inside a dedicated campus_events database.
-- DDL is not transactional in MySQL. IF NOT EXISTS permits retry after failure.
CREATE TABLE IF NOT EXISTS users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    username VARCHAR(64) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(16) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    organizer_id BIGINT UNSIGNED NOT NULL,
    title VARCHAR(120) NOT NULL,
    description TEXT NOT NULL,
    location VARCHAR(200) NOT NULL,
    capacity INT NOT NULL,
    registered_count INT NOT NULL DEFAULT 0,
    status VARCHAR(16) NOT NULL,
    registration_starts_at DATETIME(3) NOT NULL,
    registration_ends_at DATETIME(3) NOT NULL,
    starts_at DATETIME(3) NOT NULL,
    ends_at DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_events_status_id (status, id),
    KEY idx_events_organizer_id (organizer_id, id),
    CONSTRAINT fk_events_organizer FOREIGN KEY (organizer_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
