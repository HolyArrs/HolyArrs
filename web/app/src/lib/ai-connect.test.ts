import { describe, expect, it } from 'vitest';
import { FAMILIES, agentPrompt, aiURL, canSignIn, familyOf, isAIKey, onlyLocal } from './ai-connect.ts';
import { APP_DATA } from './ai-apps.ts';

const https = { url: 'https://m.example/api/v1/mcp', signIn: true };
const home = { url: 'http://10.10.25.25:18730/api/v1/mcp', key: 'hak_test', signIn: false };
const app = (id: string) => familyOf(id).apps.find((a) => a.id === id)!;
const all = (steps: { text: string; code?: string }[]) => steps.map((s) => s.text + '\n' + (s.code ?? '')).join('\n');

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
	});
	it('knows which addresses the internet cannot reach', () => {
		expect(onlyLocal('http://media.example/api/v1/mcp')).toBe(true);
		expect(onlyLocal('https://100.87.70.86/api/v1/mcp')).toBe(true);
		expect(onlyLocal('https://media.example/api/v1/mcp')).toBe(false);
	});
	it('signs Claude Code in without a key on https, and fills the key on a home address', () => {
		const signIn = all(app('claude-code').steps(https));
		expect(signIn).toContain('claude mcp add --transport http --scope user holyarrs https://m.example/api/v1/mcp');
		expect(signIn).not.toMatch(/Bearer/);
		const key = all(app('claude-code').steps(home));
		expect(key).toContain('Authorization: Bearer hak_test');
		// The Make a key button replaces the "make a key" step.
		expect(key).not.toContain('press Make a key');
	});
	it('never puts a key in an agent prompt', () => {
		expect(agentPrompt('Codex', 'https://m.example/api/v1/mcp')).not.toMatch(/Bearer|hak_/);
	});
	it('has one tab per family, every app filled in, and the website words', () => {
		expect(FAMILIES.map((f) => f.id)).toEqual([
			'claude', 'chatgpt-codex', 'gemini', 'copilot', 'chat-apps', 'code-editors', 'vscode-add-ons',
			'terminal-tools', 'self-hosted-chat', 'desktop-apps', 'local-agents', 'local-runners', 'any-app'
		]);
		expect(FAMILIES.flatMap((f) => f.apps).length).toBe(APP_DATA.length + 1);
		const ids = new Set<string>();
		for (const f of FAMILIES) {
			for (const a of f.apps) {
				expect(ids.has(a.id), a.id).toBe(false);
				ids.add(a.id);
				expect(a.guide).toMatch(/^[a-z0-9-]+\/$/);
				for (const c of [https, home]) {
					const text = all(a.steps(c));
					expect(a.steps(c).length, a.id).toBeGreaterThan(0);
					expect(text, a.id).not.toMatch(/\{url\}|\{key\}/);
					// Apps that only sign in never show a key.
					if (!APP_DATA.find((d) => d.slug === a.id)?.key.length && a.id !== 'any-app') expect(text, a.id).not.toContain('hak_test');
				}
			}
		}
		expect(familyOf('lm-studio').id).toBe('desktop-apps');
		expect(familyOf('nope').id).toBe('claude');
	});
	it('names each app the same way the website key step does', () => {
		for (const d of APP_DATA) {
			if (d.key[0]?.text.startsWith('In HolyArrs')) expect(d.key[0].text).toContain(`pick ${d.name} and press Make a key`);
		}
	});
});
