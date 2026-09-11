-- +goose Up

ALTER TABLE links
    DROP COLUMN campaign_id,
    DROP COLUMN partner_id,
    DROP COLUMN source;

-- +goose Down

ALTER TABLE links
    ADD COLUMN campaign_id uuid NOT NULL,
    ADD COLUMN partner_id uuid NULL,
    ADD COLUMN source text NULL;