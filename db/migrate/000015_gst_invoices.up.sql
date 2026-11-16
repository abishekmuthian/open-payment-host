ALTER TABLE products ADD COLUMN gst_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE products ADD COLUMN gst_code TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN gst_rate TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN gst_inclusive INTEGER NOT NULL DEFAULT 1;
ALTER TABLE products ADD COLUMN gst_export INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payment_attempts ADD COLUMN invoice_context TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_attempts ADD COLUMN invoice_billing TEXT NOT NULL DEFAULT '';
CREATE TABLE invoice_settings (id INTEGER PRIMARY KEY CHECK(id=1), value TEXT NOT NULL);
CREATE TABLE invoice_billing (id TEXT PRIMARY KEY, secret_hash TEXT NOT NULL, value TEXT NOT NULL, expires_at BIGINT NOT NULL);
CREATE TABLE invoice_sequences (kind TEXT NOT NULL, financial_year TEXT NOT NULL, next_number BIGINT NOT NULL, PRIMARY KEY(kind,financial_year));
CREATE TABLE invoices (
 id TEXT PRIMARY KEY, kind TEXT NOT NULL, gateway TEXT NOT NULL, transaction_id TEXT NOT NULL,
 refund_id TEXT NOT NULL DEFAULT '', attempt_id TEXT NOT NULL, original_id TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, issued_at TEXT NOT NULL DEFAULT '', number TEXT NOT NULL DEFAULT '',
 snapshot TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending', last_error TEXT NOT NULL DEFAULT '',
 pdf BLOB, email_done INTEGER NOT NULL DEFAULT 0, attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt_at BIGINT NOT NULL DEFAULT 0, lease_until BIGINT NOT NULL DEFAULT 0,
 UNIQUE(kind,gateway,transaction_id,refund_id)
);
CREATE UNIQUE INDEX invoice_number_unique ON invoices(number) WHERE number<>'';
CREATE INDEX invoice_attempt ON invoices(attempt_id,kind,created_at);
CREATE INDEX invoice_pending ON invoices(status,next_attempt_at);
