-- +goose Up
CREATE TABLE IF NOT EXISTS hugs_model_meta (
    model_id   TEXT PRIMARY KEY,
    tags       TEXT NOT NULL DEFAULT '',        -- comma-separated freeform tags
    notes      TEXT NOT NULL DEFAULT '',
    first_seen INTEGER NOT NULL DEFAULT 0,     -- unix ts
    last_loaded_at INTEGER NOT NULL DEFAULT 0,
    last_request_at INTEGER NOT NULL DEFAULT 0,
    registered_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS hugs_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE hugs_model_meta;
DROP TABLE hugs_settings;
