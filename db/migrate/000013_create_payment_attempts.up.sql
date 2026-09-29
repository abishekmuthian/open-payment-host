-- Secure payment fulfillment: payment attempts and provider event deduplication
CREATE TABLE IF NOT EXISTS payment_attempts (
    id text PRIMARY KEY,
    created_at text,
    updated_at text,
    product_id integer,
    gateway text,
    schedule text,
    country text,
    amount integer,
    currency text,
    price_id text,
    provider_order_id text,
    provider_payment_id text,
    provider_subscription_id text,
    custom_id text,
    redirect_uri text,
    completion_token_hash text,
    cancellation_token_hash text,
    cancellation_token_ciphertext text,
    browser_token_hash text NOT NULL,
    completion_expires_at BIGINT NOT NULL DEFAULT 0,
    cancellation_used INTEGER NOT NULL DEFAULT 0,
    counted INTEGER NOT NULL DEFAULT 0,
    status text,
    expires_at text,
    completed_at text,
    cancelled_at text
);

CREATE TABLE IF NOT EXISTS payment_events (
    id text PRIMARY KEY,
    created_at text,
    gateway text,
    event_id text,
    UNIQUE(gateway, event_id)
);

CREATE UNIQUE INDEX payment_attempt_order ON payment_attempts(gateway, provider_order_id) WHERE provider_order_id <> '';
CREATE UNIQUE INDEX payment_attempt_subscription ON payment_attempts(gateway, provider_subscription_id) WHERE provider_subscription_id <> '';
CREATE UNIQUE INDEX payment_attempt_payment ON payment_attempts(gateway, provider_payment_id) WHERE provider_payment_id <> '';
CREATE TABLE payment_transactions (
    gateway text NOT NULL,
    transaction_id text NOT NULL,
    attempt_id text NOT NULL,
    amount BIGINT NOT NULL,
    currency text NOT NULL,
    refunded BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY(gateway, transaction_id)
);
CREATE TABLE payment_outbox (
    id text PRIMARY KEY,
    attempt_id text NOT NULL,
    event_type text NOT NULL,
    created_at text NOT NULL,
    payload text NOT NULL,
    webhook_done INTEGER NOT NULL DEFAULT 0,
    mailing_done INTEGER NOT NULL DEFAULT 0,
    lease_until BIGINT NOT NULL DEFAULT 0
);
ALTER TABLE products ADD COLUMN allowed_redirect_origins text;
