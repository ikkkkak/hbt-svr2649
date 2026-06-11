-- Optional private host notes (never intended for guests / public API consumers).
-- Visible only to listing owner and organization members on authorized reads.

ALTER TABLE property_sales ADD COLUMN host_private_note TEXT;
ALTER TABLE landmarks ADD COLUMN host_private_note TEXT;
ALTER TABLE properties ADD COLUMN host_private_note TEXT;

COMMENT ON COLUMN property_sales.host_private_note IS 'Host-only private note; omit from public responses.';
COMMENT ON COLUMN landmarks.host_private_note IS 'Host-only private note; omit from public responses.';
COMMENT ON COLUMN properties.host_private_note IS 'Host-only private note; omit from public responses.';
