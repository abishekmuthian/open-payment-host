ALTER TABLE payment_outbox DROP COLUMN mailing_last_error;
ALTER TABLE payment_outbox DROP COLUMN mailing_next_attempt_at;
ALTER TABLE payment_outbox DROP COLUMN mailing_attempts;
ALTER TABLE payment_outbox DROP COLUMN webhook_last_error;
ALTER TABLE payment_outbox DROP COLUMN webhook_next_attempt_at;
ALTER TABLE payment_outbox DROP COLUMN webhook_attempts;
