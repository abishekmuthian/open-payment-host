-- Change user_id column from INTEGER to TEXT to support large custom_id values
-- SQLite doesn't support ALTER COLUMN TYPE, so we need to recreate the table

-- Create new table with TEXT user_id (matching actual database schema)
CREATE TABLE subscriptions_new (
    id integer primary key autoincrement,
    created_at text,
    updated_at text,
    txn_id text,
    txn_type text,
    transaction_subject text,
    business text,
    custom text,
    invoice text,
    receipt_ID text,
    first_name text,
    handling_amount real,
    item_number integer,
    item_name text,
    last_name text,
    mc_currency text,
    mc_fee real,
    mc_gross real,
    payer_email text,
    payer_id text,
    payer_status text,
    payment_date text,
    payment_fee real,
    payment_gross real,
    payment_status text,
    payment_type text,
    protection_eligibility text,
    quantity integer,
    receiver_id text,
    receiver_email text,
    residence_country text,
    shipping real,
    tax real,
    address_country text,
    test_ipn integer,
    address_status text,
    address_street text,
    notify_version real,
    address_city text,
    verify_sign text,
    address_state text,
    charset text,
    address_name text,
    address_country_code text,
    address_zip integer,
    subscr_id text,
    user_id text,
    test_pdt integer,
    pg text,
    payer_phone text
);

-- Copy data from old table, converting user_id INTEGER to TEXT
INSERT INTO subscriptions_new SELECT 
    id, created_at, updated_at, txn_id, txn_type, transaction_subject,
    business, custom, invoice, receipt_ID, first_name, handling_amount,
    item_number, item_name, last_name, mc_currency, mc_fee, mc_gross,
    payer_email, payer_id, payer_status, payment_date, payment_fee,
    payment_gross, payment_status, payment_type, protection_eligibility,
    quantity, receiver_id, receiver_email, residence_country, shipping,
    tax, address_country, test_ipn, address_status, address_street,
    notify_version, address_city, verify_sign, address_state, charset,
    address_name, address_country_code, address_zip, subscr_id,
    CAST(user_id AS TEXT), test_pdt, pg, payer_phone
FROM subscriptions;

-- Drop old table
DROP TABLE subscriptions;

-- Rename new table
ALTER TABLE subscriptions_new RENAME TO subscriptions;
