ALTER TABLE payment_outbox ADD COLUMN webhook_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payment_outbox ADD COLUMN webhook_next_attempt_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE payment_outbox ADD COLUMN webhook_last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_outbox ADD COLUMN mailing_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payment_outbox ADD COLUMN mailing_next_attempt_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE payment_outbox ADD COLUMN mailing_last_error TEXT NOT NULL DEFAULT '';
