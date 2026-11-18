import { describe, expect, it } from 'vitest';
import { DEFAULT_MCP_GROUPS, MCP_GROUPS, aiKeyBody, aiKeyGroupsBody, aiURL, isAIKey } from './ai-connect.ts';

describe('ai-connect', () => {
	it('starts AI keys with the first four tick boxes, without Delete and Admin', () => {
		expect(DEFAULT_MCP_GROUPS).toEqual(['see', 'add', 'control', 'settings']);
		expect(MCP_GROUPS).toEqual(['see', 'add', 'control', 'settings', 'delete', 'admin', 'approve']);
	});
	it('sends the ticked groups in screen order with the mcp scope', () => {
		expect(aiKeyBody('Claude', DEFAULT_MCP_GROUPS)).toEqual({ label: 'Claude', scopes: ['mcp'], mcp_groups: ['see', 'add', 'control', 'settings'] });
		expect(aiKeyBody('X', ['admin', 'see', 'bogus'])).toEqual({ label: 'X', scopes: ['mcp'], mcp_groups: ['see', 'admin'] });
		expect(aiKeyBody('X', []).mcp_groups).toEqual([]);
		expect(aiKeyGroupsBody(['approve', 'see', 'bogus'])).toEqual({ mcp_groups: ['see', 'approve'] });
	});
	it('tells AI keys from other keys', () => {
		const base = { id: 1, label: 'x', hint: 'abcd', created_at: '' };
		expect(isAIKey({ ...base, scopes: ['mcp'] })).toBe(true);
		expect(isAIKey({ ...base, scopes: ['all'] })).toBe(false);
		expect(isAIKey(base)).toBe(false);
	});
	it('builds the address with or without a base path', () => {
		expect(aiURL('http://192.168.1.10:18730')).toBe('http://192.168.1.10:18730/api/v1/mcp');
		expect(aiURL('https://media.example/', '/holyarrs/')).toBe('https://media.example/holyarrs/api/v1/mcp');
	});
});
