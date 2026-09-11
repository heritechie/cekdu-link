-- +goose Up

CREATE TABLE links (
    id              uuid         NOT NULL DEFAULT gen_random_uuid(),
    campaign_id     uuid         NOT NULL,
    partner_id      uuid         NULL,
    source          text         NULL,
    code            varchar(64)  NOT NULL,
    destination_url text         NOT NULL,
    click_count     bigint       NOT NULL DEFAULT 0,
    status          varchar(16)  NOT NULL DEFAULT 'active',
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT links_pkey PRIMARY KEY (id),
    CONSTRAINT links_code_key UNIQUE (code),
    CONSTRAINT links_code_not_empty CHECK (code <> ''),
    CONSTRAINT links_destination_url_not_empty CHECK (destination_url <> ''),
    CONSTRAINT links_click_count_non_negative CHECK (click_count >= 0)
);

-- +goose Down

DROP TABLE links;