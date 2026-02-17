-- +goose Up
CREATE TABLE IF NOT EXISTS tickets (
    id          BIGSERIAL PRIMARY KEY,
    session_id  VARCHAR(64)  NOT NULL,
    client_id   VARCHAR(64)  NOT NULL,
    operator_id VARCHAR(64),
    status      VARCHAR(32)  NOT NULL,
    priority    VARCHAR(32),
    region      VARCHAR(64),
    subject     VARCHAR(255),
    notes       TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    closed_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_tickets_session_id ON tickets (session_id);
CREATE INDEX IF NOT EXISTS idx_tickets_client_id ON tickets (client_id);
CREATE INDEX IF NOT EXISTS idx_tickets_operator_id ON tickets (operator_id);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets (status);
CREATE INDEX IF NOT EXISTS idx_tickets_region ON tickets (region);

-- +goose Down
DROP TABLE IF EXISTS tickets;
