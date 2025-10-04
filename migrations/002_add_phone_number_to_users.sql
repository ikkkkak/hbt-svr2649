-- Add phone number field to users table
ALTER TABLE users
ADD COLUMN phone_number VARCHAR(20) UNIQUE;

-- Create index for faster phone number lookups
CREATE INDEX idx_users_phone_number ON users(phone_number);
