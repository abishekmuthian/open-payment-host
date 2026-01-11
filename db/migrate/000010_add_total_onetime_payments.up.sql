-- Add total_onetime_payments column to products table
ALTER TABLE products ADD COLUMN total_onetime_payments INTEGER DEFAULT 0;
