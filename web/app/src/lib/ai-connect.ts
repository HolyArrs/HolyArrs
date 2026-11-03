// AI connections (Settings > AI connections): which keys are AI keys, and
// the setup guide for each AI app. On an https address (or this computer)
// apps sign in through the browser and nothing is pasted; on a plain http
// address most local apps refuse browser sign-in, so the guide makes a key
// for the app and fills it in. Cloud apps (Claude app, ChatGPT, Perplexity)
// reach HolyArrs from the internet, so they need a public https address.
import type { ApiKey } from './types.ts';
import { APP_DATA, type AppData } from './ai-apps.ts';

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

export type Step = {
	title: string;
	text?: string;
	code?: string;
	link?: { label: string; href: string };
};

export type Ctx = { url: string; key?: string; signIn: boolean };

export type AIApp = {
	id: string;
	name: string;
	/** Its full guide: a page under GUIDES_URL. */
	guide: string;
	/** Connects from the vendor's cloud: needs a public https address. */
	cloud?: boolean;
	/** Takes browser sign-in (false: always an AI key). */
	signIn?: boolean;
	/** Not run against HolyArrs yet: the steps follow the maker's docs. */
	untried?: boolean;
	/** Takes an AI key in a header (most local apps). */
	key?: boolean;
	/** A short prompt the app's own agent can follow to set itself up. */
	agent?: boolean;
	/** How to see that it worked. */
	check?: string;
	steps: (c: Ctx) => Step[];
};

const bearer = (c: Ctx) => `Bearer ${c.key ?? KEY_HOLDER}`;
const json = (v: unknown) => JSON.stringify(v, null, 2);
const signInStep = (where: string): Step => ({
	title: 'Sign in',
	text: `${where} HolyArrs opens in your browser: sign in if asked, then press Allow.`
});

/** claudeCodeCommand adds HolyArrs to Claude Code for every project. */
export function claudeCodeCommand(url: string, key?: string): string {
	const base = `claude mcp add --transport http --scope user holyarrs ${url}`;
	return key === undefined && canSignIn(url) ? base : `${base} --header "Authorization: Bearer ${key ?? KEY_HOLDER}"`;
}

/** claudeDesktopConfig is the claude_desktop_config.json entry through the mcp-remote bridge (needs Node.js). */
export function claudeDesktopConfig(url: string, key = KEY_HOLDER): string {
	const args = ['-y', 'mcp-remote@latest', url, '--transport', 'http-only'];
	if (url.startsWith('http:')) args.push('--allow-http');
	// No space after the colon: Claude Desktop on Windows splits arguments on spaces.
	args.push('--header', 'Authorization:${AUTH_HEADER}');
	return json({ mcpServers: { holyarrs: { command: 'npx', args, env: { AUTH_HEADER: `Bearer ${key}` } } } });
}

/** agentPrompt is pasted into an agent app; it never holds a key. */
export function agentPrompt(app: string, url: string): string {
	return [
		`Set up the HolyArrs MCP server for me in ${app}:`,
		`1. Add a remote MCP server named "holyarrs" at ${url} (Streamable HTTP), for all my projects.`,
		'2. Start its sign-in; I will sign in to HolyArrs in the browser and press Allow.',
		'3. Tell me when its tools show up.'
	].join('\n');
}

/** cursorLink opens Cursor's install screen; it goes straight to the app, never through a website. */
export function cursorLink(c: Ctx): string {
	const cfg: Record<string, unknown> = { url: c.url };
	if (!c.signIn) cfg.headers = { Authorization: bearer(c) };
	return `cursor://anysphere.cursor-deeplink/mcp/install?name=HolyArrs&config=${encodeURIComponent(btoa(JSON.stringify(cfg)))}`;
}

/** vscodeLink opens VS Code's install screen. */
export function vscodeLink(c: Ctx): string {
	const cfg: Record<string, unknown> = { name: 'holyarrs', type: 'http', url: c.url };
	if (!c.signIn) cfg.headers = { Authorization: bearer(c) };
	return `vscode:mcp/install?${encodeURIComponent(JSON.stringify(cfg))}`;
}

export const AI_APPS: AIApp[] = [
	{
		id: 'claude-code',
		guide: 'claude/',
		name: 'Claude Code',
		key: true,
		agent: true,
		check: 'claude mcp list',
		steps: (c) => [
			{ title: 'Add HolyArrs', text: 'Run this in a terminal:', code: claudeCodeCommand(c.url, c.signIn ? undefined : (c.key ?? KEY_HOLDER)) },
			c.signIn
				? { title: 'Sign in', text: 'In Claude Code type /mcp, pick holyarrs and choose Authenticate. HolyArrs opens in your browser: sign in if asked, then press Allow.' }
				: { title: 'Check it', text: 'Start Claude Code and type /mcp: holyarrs shows as connected.' }
		]
	},
	{
		id: 'claude',
		guide: 'claude/',
		name: 'Claude app',
		cloud: true,
		check: 'The HolyArrs connector shows as connected in Settings, Connectors',
		steps: (c) => [
			{
				title: 'Add a custom connector',
				text: 'In Claude (web, desktop or phone) open Settings, then Connectors, and choose Add custom connector. Name it HolyArrs and paste this address:',
				code: c.url,
				link: { label: 'Open Claude connectors', href: 'https://claude.ai/customize/connectors?modal=add-custom-connector' }
			},
			signInStep('Press Connect.'),
			{ title: 'Start a chat', link: { label: 'Ask Claude', href: `https://claude.ai/new?q=${encodeURIComponent(TRY_FIRST)}` } }
		]
	},
	{
		id: 'chatgpt',
		guide: 'chatgpt-codex/',
		name: 'ChatGPT',
		cloud: true,
		steps: (c) => [
			{
				title: 'Add HolyArrs to ChatGPT',
				text: 'On chatgpt.com open Plugins (on some plans: Settings, Apps and Connectors, with Developer mode on), add a custom MCP server named HolyArrs with this address and choose OAuth:',
				code: c.url,
				link: { label: 'Open ChatGPT', href: 'https://chatgpt.com/plugins' }
			},
			signInStep('Confirm the warning and create it.'),
			{ title: 'Start a chat', text: 'Type @HolyArrs in a chat, or:', link: { label: 'Ask ChatGPT', href: `https://chatgpt.com/?q=${encodeURIComponent(TRY_FIRST)}` } }
		]
	},
	{
		id: 'codex',
		guide: 'chatgpt-codex/',
		name: 'Codex',
		key: true,
		agent: true,
		check: 'codex mcp list',
		steps: (c) =>
			c.signIn
				? [
						{ title: 'Add HolyArrs', text: 'Run this in a terminal (the CLI, the IDE extension and the Codex app share it):', code: `codex mcp add holyarrs --url ${c.url}` },
						signInStep('Codex opens the browser by itself (or run codex mcp login holyarrs).')
					]
				: [
						{
							title: 'Add HolyArrs',
							text: 'Add this to ~/.codex/config.toml (Windows: %USERPROFILE%\\.codex\\config.toml), then restart Codex:',
							code: `[mcp_servers.holyarrs]\nurl = "${c.url}"\nhttp_headers = { "Authorization" = "${bearer(c)}" }`
						}
					]
	},
	{
		id: 'cursor',
		guide: 'code-editors/',
		name: 'Cursor',
		key: true,
		check: 'Cursor Settings, MCP: holyarrs has a green dot',
		steps: (c) => [
			{
				title: 'Add HolyArrs',
				text: 'Press the button, or put this in ~/.cursor/mcp.json:',
				code: json({ mcpServers: { holyarrs: c.signIn ? { url: c.url } : { url: c.url, headers: { Authorization: bearer(c) } } } }),
				link: { label: 'Add to Cursor', href: cursorLink(c) }
			},
			c.signIn ? signInStep('Cursor shows "Needs login": click it.') : { title: 'Check it', text: 'In Cursor Settings, MCP, holyarrs turns green.' }
		]
	},
	{
		id: 'vscode',
		guide: 'copilot/',
		name: 'VS Code',
		key: true,
		check: 'Command Palette: MCP: List Servers',
		steps: (c) => [
			{
				title: 'Add HolyArrs',
				text: 'Press the button, or run MCP: Add Server from the Command Palette, pick HTTP and paste this address:',
				code: c.url,
				link: { label: 'Add to VS Code', href: vscodeLink(c) }
			},
			c.signIn
				? signInStep('Start the server (MCP: List Servers, holyarrs, Start) and allow the sign-in.')
				: { title: 'Add the key', text: 'If you added it by hand, open its entry in mcp.json and add this header:', code: json({ headers: { Authorization: bearer(c) } }) }
		]
	},
	{
		id: 'gemini',
		guide: 'gemini/',
		name: 'Gemini',
		key: true,
		agent: true,
		check: '/mcp list',
		steps: (c) => [
			{
				title: 'Add HolyArrs to Gemini CLI',
				text: 'Run this in a terminal:',
				code: c.signIn
					? `gemini mcp add --scope user --transport http holyarrs ${c.url}`
					: `gemini mcp add --scope user --transport http --header "Authorization: ${bearer(c)}" holyarrs ${c.url}`
			},
			c.signIn ? signInStep('In Gemini type /mcp auth holyarrs.') : { title: 'Check it', text: 'In Gemini type /mcp list.' },
			{
				title: 'Antigravity',
				text: 'Using Antigravity instead? Add this to ~/.gemini/config/mcp_config.json:',
				code: json({ mcpServers: { holyarrs: c.signIn ? { serverUrl: c.url } : { serverUrl: c.url, headers: { Authorization: bearer(c) } } } })
			}
		]
	},
	{
		id: 'perplexity',
		guide: 'chat-apps/',
		name: 'Perplexity',
		cloud: true,
		steps: (c) => [
			{
				title: 'Add a connector',
				text: 'In Perplexity open Settings, Connectors, Add custom, then Remote. Name it HolyArrs, paste this address, and under Advanced pick OAuth and Streamable HTTP:',
				code: c.url
			},
			signInStep('Add it, then click its card.')
		]
	}
];

const fill = (t: string, c: Ctx) => t.replaceAll('{url}', c.url).replaceAll('{key}', c.key ?? KEY_HOLDER);

/** dataApp turns an entry of ai-apps.ts into a tab. */
export function dataApp(d: AppData): AIApp {
	return {
		id: d.id,
		name: d.name,
		guide: d.family + '/',
		cloud: d.cloud,
		key: d.key,
		signIn: d.signIn,
		untried: true,
		check: d.check || undefined,
		steps: (c) => {
			const signIn = c.signIn && d.signIn;
			// Cloud apps only sign in; local apps without a sign-in snippet take the key form.
			const code = d.cloud || signIn ? d.signInSnippet || (d.key && !d.cloud ? d.keySnippet : '') : d.keySnippet || d.signInSnippet;
			return [
				{ title: 'Add HolyArrs', text: `${d.where}:`, code: code ? fill(code, c) : c.url },
				{ title: signIn || d.cloud ? 'Sign in' : 'Set it up', text: d.steps }
			];
		}
	};
}

/** Claude Desktop on a home address, through the mcp-remote bridge. */
const claudeDesktop: AIApp = {
	id: 'claude-desktop',
	name: 'Claude Desktop (home)',
	guide: 'claude/',
	key: true,
	signIn: false,
	untried: true,
	steps: (c) => [
		{
			title: 'Add HolyArrs',
			text: 'Add this to claude_desktop_config.json (Settings, Developer, Edit config), then quit and reopen Claude Desktop. It needs Node.js.',
			code: claudeDesktopConfig(c.url, c.key)
		}
	]
};

/** Any other app that can add a remote MCP server. */
const anyApp: AIApp = {
	id: 'any-app',
	name: 'Any other app',
	guide: 'any-app/',
	key: true,
	steps: (c) => [
		{ title: 'Add a remote MCP server', text: 'In the app, add a remote (Streamable HTTP) MCP server named holyarrs at this address:', code: c.url },
		c.signIn
			? signInStep('When the app asks to sign in,')
			: { title: 'Add the key', text: 'Have the app send this header:', code: `Authorization: ${bearer(c)}` },
		{
			title: 'Config file',
			text: 'Apps with a settings file mostly take this form (some name the address serverUrl):',
			code: json({ mcpServers: { holyarrs: c.signIn ? { url: c.url } : { url: c.url, headers: { Authorization: bearer(c) } } } })
		},
		{
			title: 'Only runs local programs?',
			text: 'Use the mcp-remote bridge (needs Node.js) as the command:',
			code: `npx -y mcp-remote@latest ${c.url} --transport http-only${c.url.startsWith('http:') ? ' --allow-http' : ''} --header "Authorization: ${bearer(c)}"`
		}
	]
};

export type Family = { id: string; name: string; apps: AIApp[]; local?: boolean };

const main = (id: string) => AI_APPS.find((a) => a.id === id)!;
const data = (family: string, skip: string[] = []) => APP_DATA.filter((d) => d.family === family && !skip.includes(d.id)).map(dataApp);

const chat = data('chat-apps', ['perplexity']);

/** FAMILIES are the tabs in Settings > AI connections, one per website guide. */
export const FAMILIES: Family[] = [
	{ id: 'claude', name: 'Claude', apps: [main('claude-code'), main('claude'), claudeDesktop] },
	{ id: 'chatgpt-codex', name: 'ChatGPT & Codex', apps: [main('chatgpt'), main('codex')] },
	{ id: 'gemini', name: 'Gemini', apps: [main('gemini'), ...data('gemini')] },
	{ id: 'copilot', name: 'Copilot', apps: [{ ...main('vscode'), name: 'VS Code' }, ...data('copilot')] },
	{ id: 'chat-apps', name: 'Grok & more', apps: [chat[0], main('perplexity'), ...chat.slice(1)] },
	{ id: 'code-editors', name: 'Code editors', apps: [main('cursor'), ...data('code-editors')] },
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
