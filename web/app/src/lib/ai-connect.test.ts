import { describe, expect, it } from 'vitest';
import { AI_APPS, FAMILIES, agentPrompt, familyOf, aiURL, canSignIn, claudeCodeCommand, claudeDesktopConfig, cursorLink, isAIKey, onlyLocal, vscodeLink } from './ai-connect.ts';

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
	it('offers browser sign-in only where AI apps accept it', () => {
		expect(canSignIn('https://media.example/api/v1/mcp')).toBe(true);
		expect(canSignIn('http://localhost:18730/api/v1/mcp')).toBe(true);
		expect(canSignIn('http://10.10.25.25:18729/api/v1/mcp')).toBe(false);
		expect(canSignIn('http://100.87.70.86:18729/api/v1/mcp')).toBe(false);
	});
	it('knows which addresses the internet cannot reach', () => {
		expect(onlyLocal('http://media.example/api/v1/mcp')).toBe(true);
		expect(onlyLocal('https://192.168.1.4/api/v1/mcp')).toBe(true);
		expect(onlyLocal('https://100.87.70.86/api/v1/mcp')).toBe(true);
		expect(onlyLocal('https://media.example/api/v1/mcp')).toBe(false);
	});
	it('signs Claude Code in without a key on https, and fills the key on http', () => {
		expect(claudeCodeCommand('https://m.example/api/v1/mcp')).toBe('claude mcp add --transport http --scope user holyarrs https://m.example/api/v1/mcp');
		expect(claudeCodeCommand('http://h:1/api/v1/mcp', 'ha_k')).toBe(
			'claude mcp add --transport http --scope user holyarrs http://h:1/api/v1/mcp --header "Authorization: Bearer ha_k"'
		);
	});
	it('bridges Claude Desktop over plain http with the key in env, not in the arguments', () => {
		const cfg = JSON.parse(claudeDesktopConfig('http://h:1/api/v1/mcp', 'ha_k')).mcpServers.holyarrs;
		expect(cfg.args).toContain('--allow-http');
		expect(cfg.args).toContain('Authorization:${AUTH_HEADER}');
		expect(cfg.env.AUTH_HEADER).toBe('Bearer ha_k');
		expect(JSON.parse(claudeDesktopConfig('https://m.example/api/v1/mcp')).mcpServers.holyarrs.args).not.toContain('--allow-http');
	});
	it('never puts a key in an agent prompt and keeps install links on the device', () => {
		expect(agentPrompt('Codex', 'https://m.example/api/v1/mcp')).not.toMatch(/Bearer|ha_/);
		const c = { url: 'https://m.example/api/v1/mcp', signIn: true };
		const cfg = JSON.parse(atob(decodeURIComponent(cursorLink(c).split('config=')[1])));
		expect(cfg).toEqual({ url: c.url });
		expect(cursorLink(c)).toMatch(/^cursor:\/\//);
		expect(JSON.parse(decodeURIComponent(vscodeLink(c).split('?')[1]))).toEqual({ name: 'holyarrs', type: 'http', url: c.url });
		const keyed = { url: 'http://h:1/api/v1/mcp', key: 'ha_k', signIn: false };
		expect(JSON.parse(atob(decodeURIComponent(cursorLink(keyed).split('config=')[1]))).headers.Authorization).toBe('Bearer ha_k');
	});
	it('every app has steps for both kinds of address', () => {
		for (const a of AI_APPS) {
			for (const signIn of [true, false]) {
				const steps = a.steps({ url: 'https://m.example/api/v1/mcp', signIn });
				expect(steps.length).toBeGreaterThan(0);
				for (const s of steps) expect(s.title).toBeTruthy();
			}
		}
		expect(AI_APPS.map((a) => a.name)).toEqual(['Claude Code', 'Claude app', 'ChatGPT', 'Codex', 'Cursor', 'VS Code', 'Gemini', 'Perplexity']);
	});
	it('has one tab per website guide, and every app in it has steps with the address filled in', () => {
		expect(FAMILIES.map((f) => f.id)).toEqual([
			'claude', 'chatgpt-codex', 'gemini', 'copilot', 'chat-apps', 'code-editors', 'vscode-add-ons',
			'terminal-tools', 'self-hosted-chat', 'desktop-apps', 'local-agents', 'local-runners', 'any-app'
		]);
		const ids = new Set<string>();
		for (const f of FAMILIES) {
			expect(f.apps.length).toBeGreaterThan(0);
			for (const a of f.apps) {
				expect(ids.has(a.id), a.id).toBe(false);
				ids.add(a.id);
				expect(a.guide).toMatch(/^[a-z-]+\/$/);
				for (const signIn of [true, false]) {
					const steps = a.steps({ url: 'https://m.example/api/v1/mcp', key: 'hak_test', signIn });
					const all = steps.map((s) => (s.text ?? '') + (s.code ?? '')).join('\n');
					expect(all, a.id).not.toMatch(/\{url\}|\{key\}/);
					// Cloud apps sign in: a key never shows in their steps.
					if (a.cloud) expect(all, a.id).not.toContain('hak_test');
				}
			}
		}
		expect(familyOf('lm-studio').id).toBe('desktop-apps');
		expect(familyOf('nope').id).toBe('claude');
	});
});
