DROP TABLE payment_outbox;
DROP TABLE payment_transactions;
DROP TABLE payment_events;
DROP TABLE payment_attempts;
ALTER TABLE products DROP COLUMN allowed_redirect_origins;
