-- Initial schema intentionally minimal but aligned with the selected architecture.

CREATE TABLE IF NOT EXISTS wallets (
    id              UUID PRIMARY KEY,
    player_id       TEXT NOT NULL,
    balance         BIGINT NOT NULL DEFAULT 0,
    currency        CHAR(3) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT wallets_balance_non_negative CHECK (balance >= 0)
);

CREATE TABLE IF NOT EXISTS wager_transactions (
    id                        UUID PRIMARY KEY,
    provider_id               TEXT NOT NULL,
    external_transaction_id   TEXT NOT NULL,
    wallet_id                 UUID NOT NULL REFERENCES wallets(id),
    player_id                 TEXT NOT NULL,
    round_id                  TEXT NOT NULL,
    game_id                   TEXT NOT NULL,
    kind                      TEXT NOT NULL,
    amount                    BIGINT NOT NULL,
    currency                  CHAR(3) NOT NULL,
    reference_transaction_id  TEXT NULL,
    status                    TEXT NOT NULL,
    payload_hash              TEXT NOT NULL,
    result_balance            BIGINT NULL,
    occurred_at               TIMESTAMPTZ NOT NULL,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT wager_provider_external_unique
      UNIQUE (provider_id, external_transaction_id)
);

CREATE TABLE IF NOT EXISTS ledger_entries (
    id                    UUID PRIMARY KEY,
    wallet_id             UUID NOT NULL REFERENCES wallets(id),
    wager_transaction_id  UUID NULL REFERENCES wager_transactions(id),
    entry_type            TEXT NOT NULL,
    amount                BIGINT NOT NULL,
    currency              CHAR(3) NOT NULL,
    balance_after         BIGINT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS inbox_messages (
    message_id     TEXT PRIMARY KEY,
    payload_hash   TEXT NOT NULL,
    status         TEXT NOT NULL,
    processed_at   TIMESTAMPTZ NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS outbox_events (
    id             UUID PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id   TEXT NOT NULL,
    event_type     TEXT NOT NULL,
    payload        JSONB NOT NULL,
    status         TEXT NOT NULL DEFAULT 'PENDING',
    attempts       INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NULL,
    published_at   TIMESTAMPTZ NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending
ON outbox_events (status, next_attempt_at, created_at);

CREATE INDEX IF NOT EXISTS idx_ledger_wallet_created
ON ledger_entries (wallet_id, created_at);
