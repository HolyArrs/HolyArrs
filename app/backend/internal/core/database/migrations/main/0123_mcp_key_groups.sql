-- The tick boxes of an AI (mcp) key: which groups of actions its tools may
-- take (see internal/ai/mcpkeys). A new key starts with all but Delete and
-- Admin; a key made before this keeps every group, so it can still do all
-- it could before.
ALTER TABLE api_keys ADD COLUMN mcp_groups TEXT NOT NULL DEFAULT '["see","add","control","settings"]';
UPDATE api_keys SET mcp_groups = '["see","add","control","settings","delete","admin"]';
