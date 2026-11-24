import type { ApiKey } from '../core/types.ts';

export const AI_SCOPE = 'mcp';

// An AI key's tick boxes (the server's mcp_groups), in screen order. The owner can change them later.
export const MCP_GROUPS = ['see', 'add', 'control', 'settings', 'delete', 'admin', 'approve', 'plugins'] as const;
export type McpGroup = (typeof MCP_GROUPS)[number];
export const DEFAULT_MCP_GROUPS: McpGroup[] = ['see', 'add', 'control', 'settings'];

// The body for POST /auth/keys; groups go in screen order.
export function aiKeyBody(label: string, groups: readonly string[]) {
	return { label, scopes: [AI_SCOPE], mcp_groups: MCP_GROUPS.filter((g) => groups.includes(g)) };
}

// The body for PUT /auth/keys/{id}; only the tick boxes change.
export function aiKeyGroupsBody(groups: readonly string[]) {
	return { mcp_groups: MCP_GROUPS.filter((g) => groups.includes(g)) };
}

export function isAIKey(k: ApiKey): boolean {
	return (k.scopes ?? []).includes(AI_SCOPE);
}

export function aiURL(origin: string, base = ''): string {
	return origin.replace(/\/+$/, '') + base.replace(/\/+$/, '') + '/api/v1/mcp';
}
