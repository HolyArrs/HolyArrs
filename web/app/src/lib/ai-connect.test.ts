import { describe, expect, it } from 'vitest';
import { aiURL, claudeCodeCommand, claudeDesktopConfig, isAIKey } from './ai-connect.ts';

describe('ai-connect', () => {
	it('tells AI keys from other keys', () => {
		const base = { id: 1, label: 'x', hint: 'abcd', created_at: '' };
		expect(isAIKey({ ...base, scopes: ['mcp'] })).toBe(true);
		expect(isAIKey({ ...base, scopes: ['all'] })).toBe(false);
		expect(isAIKey(base)).toBe(false);
	});
	it('builds the address with or without a base path', () => {
		expect(aiURL('http://10.10.25.25:18730')).toBe('http://10.10.25.25:18730/api/v1/mcp');
		expect(aiURL('https://media.example/', '/holyarrs/')).toBe('https://media.example/holyarrs/api/v1/mcp');
	});
	it('writes setup text for Claude Code and Claude Desktop', () => {
		const url = 'http://h:1/api/v1/mcp';
		expect(claudeCodeCommand(url, 'ha_k')).toBe('claude mcp add --transport http holyarrs http://h:1/api/v1/mcp --header "Authorization: Bearer ha_k"');
		const cfg = JSON.parse(claudeDesktopConfig(url, 'ha_k'));
		expect(cfg.mcpServers.holyarrs.args).toEqual(['-y', 'mcp-remote', url, '--header', 'Authorization: Bearer ha_k']);
	});
});
