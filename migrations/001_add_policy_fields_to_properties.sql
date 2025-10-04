-- Migration: Add policy fields to properties table
-- Date: 2024-01-XX
-- Description: Add new policy-related fields to the properties table

ALTER TABLE properties ADD COLUMN booking_mode VARCHAR(50) DEFAULT 'instant';
ALTER TABLE properties ADD COLUMN secure_compound_acknowledged BOOLEAN DEFAULT FALSE;
ALTER TABLE properties ADD COLUMN equipment_violation_policy_accepted BOOLEAN DEFAULT FALSE;
ALTER TABLE properties ADD COLUMN user_safety_policy_accepted BOOLEAN DEFAULT FALSE;
ALTER TABLE properties ADD COLUMN property_policy_accepted BOOLEAN DEFAULT FALSE;

-- Add indexes for better query performance
CREATE INDEX idx_properties_booking_mode ON properties(booking_mode);
CREATE INDEX idx_properties_policy_accepted ON properties(secure_compound_acknowledged, equipment_violation_policy_accepted, user_safety_policy_accepted, property_policy_accepted);
