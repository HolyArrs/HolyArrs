import { describe, expect, it } from 'vitest';
import { GROUPS, GUIDES_URL, ROWS, agentPrompt, aiURL, canSignIn, codeParts, deeplinkHref, isAIKey, onlyLocal, pickFor, stepParts, testedLine, whereText } from './ai-connect.ts';
import { APP_DATA } from './ai-apps.ts';

const https = { url: 'https://m.example/api/v1/mcp', signIn: true };
const home = { url: 'http://192.168.1.10:18730/api/v1/mcp', key: 'hak_test', signIn: false };
const app = (id: string) => ROWS.find((r) => r.id === id)!.app;
const all = (steps: { text: string; code?: string }[]) => steps.map((s) => s.text + '\n' + (s.code ?? '')).join('\n');
const PAGES = ['claude-app', 'chatgpt', 'gemini-app', 'claude-code', 'codex', 'cursor', 'claude-desktop-home', 'local-ai'];

describe('ai-connect', () => {
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
	it('offers browser sign-in only where AI apps accept it', () => {
		expect(canSignIn('https://media.example/api/v1/mcp')).toBe(true);
		expect(canSignIn('http://localhost:18730/api/v1/mcp')).toBe(true);
		expect(canSignIn('http://192.168.1.10:18729/api/v1/mcp')).toBe(false);
	});
	it('knows which addresses the internet cannot reach', () => {
		expect(onlyLocal('http://media.example/api/v1/mcp')).toBe(true);
		expect(onlyLocal('https://100.64.0.10/api/v1/mcp')).toBe(true);
		expect(onlyLocal('https://media.example/api/v1/mcp')).toBe(false);
	});
	it('lays out the table like the website: three groups, then Any other app', () => {
		expect(GROUPS.map(([g]) => g)).toEqual(['chat', 'agents', 'computer']);
		expect(ROWS.map((r) => r.name)).toEqual([
			'Claude app', 'ChatGPT', 'Gemini app', 'Grok', 'Perplexity',
			'Claude Code', 'Codex', 'Cursor', 'VS Code with Copilot', 'Gemini CLI',
			'Claude Desktop at home', 'Open WebUI, LM Studio, Ollama', 'Any other app'
		]);
		expect(ROWS.length).toBe(APP_DATA.filter((d) => d.table).length + 1);
		expect(APP_DATA.length).toBe(56);
	});
	it('links each row only to an existing guide page or a main-page anchor', () => {
		for (const r of ROWS) {
			const m = r.guide.match(/^https:\/\/holyarrs\.coderluii\.dev\/docs\/ai\/(?:([a-z0-9-]+)\/|#(apps|any-other-app))$/);
			expect(m, r.guide).toBeTruthy();
			if (m![1]) expect(PAGES, r.id).toContain(m![1]);
		}
		expect(ROWS.at(-1)!.guide).toBe(GUIDES_URL + '#any-other-app');
	});
	it("opens the linked app's own steps, under the row of its guide or on their own", () => {
		const lm = pickFor('lm-studio')!;
		expect(lm.row!.id).toBe('open-webui');
		expect(lm.app.name).toBe('LM Studio');
		expect(all(lm.app.steps(https))).toContain('Edit mcp.json');
		expect(pickFor('codex-ide')!.app.id).toBe('codex-ide');
		expect(pickFor('codex-ide')!.row!.name).toBe('Codex');
		const w = pickFor('windsurf')!;
		expect(w.row).toBeUndefined();
		expect(w.app.name).toBe('Windsurf');
		for (const d of APP_DATA) expect(pickFor(d.slug)!.app.id, d.slug).toBe(d.slug);
		expect(pickFor('cursor')!.row!.id).toBe('cursor');
		expect(pickFor('any-other-app')!.row!.name).toBe('Any other app');
		expect(pickFor('nope')).toBeUndefined();
		expect(pickFor('')).toBeUndefined();
	});
	it('fills the address into a row and marks its code', () => {
		expect(codeParts('Add to Cursor, or `~/.cursor/mcp.json`')).toEqual([{ text: 'Add to Cursor, or ' }, { text: '~/.cursor/mcp.json', code: true }]);
	});
	it('fits the "where" cell to the address: no one-click button or header-less command on a home address', () => {
		for (const r of ROWS) expect(whereText(r, home.url, false), r.id).not.toMatch(/hak_|<address>/);
	});
	it('signs Claude Code in without a key on https, and fills the key on a home address', () => {
		const signIn = all(app('claude-code').steps(https));
		expect(signIn).toContain('claude mcp add --transport http --scope user holyarrs https://m.example/api/v1/mcp');
		expect(signIn).not.toMatch(/Bearer/);
		const key = all(app('claude-code').steps(home));
		expect(key).toContain('Authorization: Bearer hak_test');
		expect(key).not.toContain('In HolyArrs');
	});
	it('bolds labels and marks commands like the website', () => {
		expect(stepParts('Start claude, run /mcp, pick holyarrs and choose Authenticate.', ['Authenticate'])).toEqual([
			{ text: 'Start claude, run ' },
			{ text: '/mcp', code: true },
			{ text: ', pick holyarrs and choose ' },
			{ text: 'Authenticate', b: true },
			{ text: '.' }
		]);
		expect(stepParts('Press Allow.')).toEqual([{ text: 'Press ' }, { text: 'Allow', b: true }, { text: '.' }]);
		expect(stepParts('Open ~/.cursor/mcp.json.')).toEqual([{ text: 'Open ' }, { text: '~/.cursor/mcp.json', code: true }, { text: '.' }]);
	});
	it("gives Cursor's one-click button only on sign-in, built from the address", () => {
		const s = app('cursor').steps(https);
		expect(s[0].deeplink).toBe('cursor');
		expect(app('cursor').steps(home).some((x) => x.deeplink)).toBe(false);
		const c = deeplinkHref('cursor', https.url);
		expect(c.startsWith('cursor://anysphere.cursor-deeplink/mcp/install?name=holyarrs&config=')).toBe(true);
		expect(JSON.parse(atob(c.split('config=')[1]))).toEqual({ url: https.url });
		expect(decodeURIComponent(deeplinkHref('vscode', https.url).replace('vscode:mcp/install?', ''))).toBe(JSON.stringify({ name: 'holyarrs', type: 'http', url: https.url }));
	});
	it('gives the Gemini app a client ID form in place of its HolyArrs step', () => {
		const g = app('gemini-app');
		expect(g.clientIdHelper).toBe(true);
		expect(all(g.steps(https))).not.toContain('In HolyArrs');
		expect(app('claude-app').clientIdHelper).toBeFalsy();
	});
	it('says when the steps were tested', () => {
		expect(testedLine(app('claude-code'))).toBe('Tested with HolyArrs on 3 Nov 2026');
		expect(testedLine(app('cursor'))).toBe("Not tested with HolyArrs yet. These steps follow Cursor's own help pages.");
		expect(testedLine(ROWS.at(-1)!.app)).toBe('');
	});
	it('puts a key in the agent prompt only when one is given', () => {
		const o = 'https://m.example';
		expect(agentPrompt(o)).toBe(
			'Read https://holyarrs.coderluii.dev/docs/ai/ai.md and connect yourself to my HolyArrs at https://m.example. Only add the HolyArrs MCP server. Ask me before you change anything else.'
		);
		expect(agentPrompt(o)).not.toMatch(/hak_/);
		expect(agentPrompt(o, { home: true })).toMatch(/I'll give you an AI key when you ask\.$/);
		expect(agentPrompt(o, { key: 'hak_test', home: true })).toContain('Use this AI key: hak_test. Only add');
	});
	it('fills every row on both kinds of address', () => {
		for (const r of ROWS) {
			for (const c of [https, home]) {
				const steps = r.app.steps(c);
				expect(steps.length, r.id).toBeGreaterThan(0);
				expect(all(steps), r.id).not.toMatch(/\{url\}|\{key\}/);
				for (const s of steps) expect(s.parts.map((p) => p.text).join(''), r.id).toBe(s.text);
			}
		}
	});
	it('keeps placeholders only in steps, where they are filled in', () => {
		for (const d of APP_DATA) expect(`${d.needs} ${d.check} ${d.note}`, d.slug).not.toMatch(/\{url\}|\{key\}/);
	});
});
