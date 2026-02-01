-- Add junction table for scans to API definitions many-to-many relationship
-- This allows users to specify API definitions to scan along with regular URL crawling

CREATE TABLE IF NOT EXISTS scan_api_definitions (
    scan_id BIGINT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    api_definition_id UUID NOT NULL REFERENCES api_definitions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (scan_id, api_definition_id)
);

CREATE INDEX IF NOT EXISTS idx_scan_api_definitions_scan_id ON scan_api_definitions(scan_id);
CREATE INDEX IF NOT EXISTS idx_scan_api_definitions_api_definition_id ON scan_api_definitions(api_definition_id);
