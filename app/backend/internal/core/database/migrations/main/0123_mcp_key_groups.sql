-- The tick boxes of an AI (mcp) key: which groups of actions its tools may
-- take (see internal/ai/mcpkeys). Every key, new or old, starts with all
-- but Delete and Admin, so an existing AI key keeps what it could do.
ALTER TABLE api_keys ADD COLUMN mcp_groups TEXT NOT NULL DEFAULT '["see","add","control","settings"]';
