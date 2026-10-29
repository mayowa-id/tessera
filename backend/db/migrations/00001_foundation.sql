-- +goose Up
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- +goose Down
-- Keep this shared extension installed; later scheduling migrations may depend on it.
