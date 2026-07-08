-- WhatsApp share card usage analytics (property sale listing cards)
CREATE TABLE IF NOT EXISTS whatsapp_share_usage_events (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id BIGINT NOT NULL DEFAULT 0,
    property_sale_id BIGINT NOT NULL,
    event VARCHAR(24) NOT NULL,
    platform VARCHAR(16),
    property_title VARCHAR(255)
);

CREATE INDEX IF NOT EXISTS idx_whatsapp_share_usage_created_at ON whatsapp_share_usage_events (created_at);
CREATE INDEX IF NOT EXISTS idx_whatsapp_share_usage_user_id ON whatsapp_share_usage_events (user_id);
CREATE INDEX IF NOT EXISTS idx_whatsapp_share_usage_property_sale_id ON whatsapp_share_usage_events (property_sale_id);
CREATE INDEX IF NOT EXISTS idx_whatsapp_share_usage_event ON whatsapp_share_usage_events (event);
