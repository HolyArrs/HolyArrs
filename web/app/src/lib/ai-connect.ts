// AI connections (Settings > AI connections): which keys are AI keys, and
// the setup guide for each AI app. On an https address (or this computer)
// apps sign in through the browser and nothing is pasted; on a plain http
// address most local apps refuse browser sign-in, so the guide makes a key
// for the app and fills it in. Cloud apps (Claude app, ChatGPT, Perplexity)
// reach HolyArrs from the internet, so they need a public https address.
import type { ApiKey } from './types.ts';
import { APP_DATA, type AppData, type AppStep } from './ai-apps.ts';

/** The scope an AI key holds, and nothing else. */
export const AI_SCOPE = 'mcp';

/** isAIKey: the key reaches only the AI route. */
export function isAIKey(k: ApiKey): boolean {
	return (k.scopes ?? []).includes(AI_SCOPE);
}

/** aiURL is the address an AI app connects to, from the page's own origin and base path. */
export function aiURL(origin: string, base = ''): string {
	return origin.replace(/\/+$/, '') + base.replace(/\/+$/, '') + '/api/v1/mcp';
}

const KEY_HOLDER = '<your AI key>';

/** canSignIn: apps accept browser sign-in only on https or this computer. */
export function canSignIn(url: string): boolean {
	try {
		const u = new URL(url);
		return u.protocol === 'https:' || ['localhost', '127.0.0.1', '[::1]'].includes(u.hostname);
	} catch {
		return false;
	}
}

/** onlyLocal: an address the internet cannot reach (private, Tailscale or loopback IP, or plain http). */
export function onlyLocal(url: string): boolean {
	try {
		const u = new URL(url);
		const h = u.hostname;
		return (
			u.protocol !== 'https:' ||
			h === 'localhost' ||
			h.startsWith('[') ||
			/^(10|127)\./.test(h) ||
			/^192\.168\./.test(h) ||
			/^172\.(1[6-9]|2\d|3[01])\./.test(h) ||
			/^100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\./.test(h) ||
			h.endsWith('.local') ||
			h.endsWith('.lan') ||
			h.endsWith('.home.arpa')
		);
	} catch {
		return true;
	}
}

/** Where the full guides live: the HolyArrs website, one page per family. */
export const GUIDES_URL = 'https://holyarrs.coderluii.dev/docs/ai/';

/** The first thing to ask once connected; the same in every app. */
export const TRY_FIRST = "Use HolyArrs: what's downloading right now, and what's coming out this week?";

export type Step = AppStep;

export type Ctx = { url: string; key?: string; signIn: boolean };

export type AIApp = {
	id: string;
	name: string;
	/** Its guide page under GUIDES_URL. */
	guide: string;
	/** Connects from the maker's servers: needs a public https address. */
	cloud?: boolean;
	/** Takes browser sign-in (false: always an AI key). */
	signIn?: boolean;
	/** One line: what the app needs (plan, version). */
	needs?: string;
	note?: string;
	/** A short prompt the app's own agent can follow to set itself up. */
	agent?: boolean;
	/** How to see that it worked. */
	check?: string;
	/** Not run against HolyArrs yet: the steps follow the maker's docs. */
	untried?: boolean;
	/** The app does not register itself: the guide makes it a client ID. */
	clientIdHelper?: boolean;
	steps: (c: Ctx) => Step[];
};

const bearer = (c: Ctx) => `Bearer ${c.key ?? KEY_HOLDER}`;
const json = (v: unknown) => JSON.stringify(v, null, 2);

/** agentPrompt is pasted into an agent app; it never holds a key. */
export function agentPrompt(app: string, url: string): string {
	return [
		`Set up the HolyArrs MCP server for me in ${app}:`,
		`1. Add a remote MCP server named "holyarrs" at ${url} (Streamable HTTP), for all my projects.`,
		'2. Start its sign-in; I will sign in to HolyArrs in the browser and press Allow.',
		'3. Tell me when its tools show up.'
	].join('\n');
}

const fill = (t: string, c: Ctx) => t.replaceAll('{url}', c.url).replaceAll('{key}', c.key ?? KEY_HOLDER);

/** dataApp turns an entry of ai-apps.ts into a tab: sign-in steps where the
 * address allows it, otherwise the key steps (the Make a key button there
 * replaces their first step). */
export function dataApp(d: AppData): AIApp {
	return {
		id: d.slug,
		name: d.name,
		guide: d.slug + '/',
		cloud: d.cloud,
		signIn: d.signin.length > 0,
		needs: d.needs || undefined,
		note: d.note || undefined,
		check: d.check || undefined,
		untried: d.source !== 'live',
		agent: ['claude-code', 'codex-cli', 'gemini-cli'].includes(d.slug),
		clientIdHelper: d.clientIdHelper,
		steps: (c) => {
			const list = c.signIn && d.signin.length ? d.signin : d.key.length ? d.key : d.signin;
			return list
				.filter((s) => !s.text.startsWith('In HolyArrs, open Settings'))
				.map((s) => ({ text: fill(s.text, c), ...(s.code ? { code: fill(s.code, c) } : {}) }));
		}
	};
}

/** Any other app that can add a remote MCP server. */
const anyApp: AIApp = {
	id: 'any-app',
	name: 'Any other app',
	guide: 'other-apps/',
	steps: (c) => [
		{ text: 'Add a remote MCP server named holyarrs, type Streamable HTTP, with this address.', code: c.url },
		c.signIn ? { text: 'When the app asks, sign in. HolyArrs opens in your browser. Press Allow.' } : { text: 'Have the app send this header.', code: `Authorization: ${bearer(c)}` },
		{ text: 'Apps with a settings file mostly take this form.', code: json({ mcpServers: { holyarrs: c.signIn ? { url: c.url } : { url: c.url, headers: { Authorization: bearer(c) } } } }) },
		{
			text: 'App runs only local programs? Use this bridge (needs Node.js).',
			code: `npx -y mcp-remote@latest ${c.url} --transport http-only${c.url.startsWith('http:') ? ' --allow-http' : ''} --header "Authorization: ${bearer(c)}"`
		}
	]
};

export type Family = { id: string; name: string; apps: AIApp[]; local?: boolean };

const data = (family: string) => APP_DATA.filter((d) => d.family === family).map(dataApp);

/** FAMILIES are the tabs in Settings > AI connections; each app links to its guide page. */
export const FAMILIES: Family[] = [
	{ id: 'claude', name: 'Claude', apps: data('claude') },
	{ id: 'chatgpt-codex', name: 'ChatGPT & Codex', apps: data('chatgpt-codex') },
	{ id: 'gemini', name: 'Gemini', apps: data('gemini') },
	{ id: 'copilot', name: 'Copilot', apps: data('copilot') },
	{ id: 'chat-apps', name: 'Grok & more', apps: data('chat-apps') },
	{ id: 'code-editors', name: 'Code editors', apps: data('code-editors') },
	{ id: 'vscode-add-ons', name: 'VS Code add-ons', apps: data('vscode-add-ons') },
	{ id: 'terminal-tools', name: 'Terminal tools', apps: data('terminal-tools') },
	{ id: 'self-hosted-chat', name: 'Self-hosted chat', apps: data('self-hosted-chat'), local: true },
	{ id: 'desktop-apps', name: 'Desktop AI', apps: data('desktop-apps'), local: true },
	{ id: 'local-agents', name: 'Local agents', apps: data('local-agents'), local: true },
	{ id: 'local-runners', name: 'Local runners', apps: data('local-runners'), local: true },
	{ id: 'any-app', name: 'Any other app', apps: [anyApp] }
];

/** familyOf finds the family of an app id (the first family for an unknown id). */
export function familyOf(appId: string): Family {
	return FAMILIES.find((f) => f.apps.some((a) => a.id === appId)) ?? FAMILIES.find((f) => f.id === appId) ?? FAMILIES[0];
}
