// AI connections (Settings > AI connections): which keys are AI keys, the
// "Where to add HolyArrs" table and the setup for each app, the same as the
// website's /docs/ai/. On an https address (or this computer) apps sign in
// through the browser and nothing is pasted; on a plain http address most
// apps refuse browser sign-in, so the page makes a key for the app and fills
// it in. Cloud apps (Claude app, ChatGPT, Gemini) reach HolyArrs from the
// internet, so they need a public https address.
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

/** Where the guides live: the HolyArrs website, one main page and 8 app pages. */
export const GUIDES_URL = 'https://holyarrs.coderluii.dev/docs/ai/';

/** The question that shows the connection works; the same on the website. */
export const CHECK_QUESTION = "What's downloading in HolyArrs?";

/** guideURL: an app's own page, or the main page's table. */
export function guideURL(page?: string | null): string {
	return page ? `${GUIDES_URL}${page}/` : `${GUIDES_URL}#apps`;
}

/** agentPrompt is pasted into an agent; it holds a key only when one is given. */
export function agentPrompt(origin: string, o: { key?: string; home?: boolean } = {}): string {
	const read = `Read ${GUIDES_URL}ai.md and connect yourself to my HolyArrs at ${origin}.`;
	const rest = 'Only add the HolyArrs MCP server. Ask me before you change anything else.';
	if (o.key) return `${read} Use this AI key: ${o.key}. ${rest}`;
	return o.home ? `${read} ${rest} I'll give you an AI key when you ask.` : `${read} ${rest}`;
}

/** A piece of step text: plain, bold (a label in the app) or code. */
export type Part = { text: string; b?: boolean; code?: boolean };

/** codeParts splits text at `backticks` into plain and code parts. */
export function codeParts(text: string): Part[] {
	return text
		.split('`')
		.map((t, i) => (i % 2 ? { text: t, code: true } : { text: t }))
		.filter((p) => p.text);
}

// The website's step markup (dev/build-ai-docs.mjs): the step's labels and a
// few HolyArrs ones in bold, commands, paths and file names as code.
const GLOBAL_BOLD = ['Settings › AI connections', 'Make client ID', 'Make a key', 'Allow'];
const CODE_WORDS =
	/(~\/[\w./-]+|%\w+%\\[\w.\\*]+|(?<![\w-])--[a-z][a-z-]+|\b[A-Z][A-Z0-9]*_[A-Z0-9_]+\b|\bclaude mcp login holyarrs\b|(?<![\w/])\/mcp\b|\b[\w-]+\.(?:json|toml)\b|\bholyarrs\.json\b)/g;
const reEsc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/** stepParts marks a data step's labels as bold and its commands as code. */
export function stepParts(text: string, bold: string[] = []): Part[] {
	const labels = [...new Set([...bold, ...GLOBAL_BOLD])].sort((a, b) => b.length - a.length);
	const held: string[] = [];
	let s = text;
	for (const label of labels) {
		s = s.replace(new RegExp(`(?<![\\w\\u0001])${reEsc(label)}(?![\\w\\u0001])`, 'g'), () => `\u0001${held.push(label) - 1}\u0001`);
	}
	s = s.replace(CODE_WORDS, (m) => (m.endsWith('.') ? '`' + m.slice(0, -1) + '`.' : '`' + m + '`'));
	return codeParts(s).flatMap((p) =>
		p.code ? [p] : p.text.split(/\u0001(\d+)\u0001/).map((t, i) => (i % 2 ? { text: held[Number(t)], b: true } : { text: t })).filter((x) => x.text)
	);
}

/** deeplinkHref builds an app's own one-click install link from the AI address only. */
export function deeplinkHref(kind: 'cursor' | 'vscode', url: string): string {
	if (kind === 'cursor') {
		const json = new TextEncoder().encode(JSON.stringify({ url }));
		return `cursor://anysphere.cursor-deeplink/mcp/install?name=holyarrs&config=${btoa(String.fromCharCode(...json))}`;
	}
	return `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name: 'holyarrs', type: 'http', url }))}`;
}

export const DEEPLINK_LABEL = { cursor: 'Add to Cursor', vscode: 'Add to VS Code' } as const;

export type Step = {
	text: string;
	parts: Part[];
	code?: string;
	link?: { text: string; href: string };
	deeplink?: 'cursor' | 'vscode';
};

export type Ctx = { url: string; key?: string; signIn: boolean };

export type AIApp = {
	id: string;
	name: string;
	/** Connects from the maker's servers: needs a public https address. */
	cloud?: boolean;
	/** Takes browser sign-in (false: always an AI key). */
	signIn: boolean;
	/** One line: what the app needs (plan, version). */
	needs?: string;
	note?: string;
	/** How to see that it worked. */
	check?: string;
	/** The day it was tested with HolyArrs (YYYY-MM-DD). */
	tested?: string | null;
	/** The app does not register itself: the page makes it a client ID. */
	clientIdHelper?: boolean;
	steps: (c: Ctx) => Step[];
};

const fill = (t: string, c: Ctx) => t.replaceAll('{url}', c.url).replaceAll('{key}', c.key ?? KEY_HOLDER);

function toStep(s: AppStep, c: Ctx, signIn: boolean): Step {
	return {
		text: fill(s.text, c),
		parts: stepParts(s.text, s.bold).map((p) => ({ ...p, text: fill(p.text, c) })),
		...(s.code ? { code: fill(s.code, c) } : {}),
		...(s.link ? { link: s.link } : {}),
		...(s.deeplink && signIn ? { deeplink: s.deeplink } : {})
	};
}

/** dataApp turns an entry of ai-apps.ts into an app: sign-in steps where the
 * address allows it, otherwise the key steps. Steps done in HolyArrs are left
 * out; this page has their button (Make a key, the client ID form). */
export function dataApp(d: AppData): AIApp {
	return {
		id: d.slug,
		name: d.name,
		cloud: d.cloud,
		signIn: d.signin.length > 0,
		needs: d.needs || undefined,
		note: d.note || undefined,
		check: d.check || undefined,
		tested: d.tested,
		clientIdHelper: d.clientIdHelper,
		steps: (c) => {
			const signIn = c.signIn && d.signin.length > 0;
			const list = signIn ? d.signin : d.key.length ? d.key : d.signin;
			return list.filter((s) => !s.text.startsWith('In HolyArrs')).map((s) => toStep(s, c, signIn));
		}
	};
}

const bearer = (c: Ctx) => `Bearer ${c.key ?? KEY_HOLDER}`;
const json = (v: unknown) => JSON.stringify(v, null, 2);
const plain = (text: string): Step => ({ text, parts: stepParts(text) });

/** Any other app that can add a remote MCP server. */
const anyApp: AIApp = {
	id: 'any-app',
	name: 'Any other app',
	signIn: true,
	steps: (c) => [
		{ ...plain('Add a remote MCP server named holyarrs, type Streamable HTTP, with this address.'), code: c.url },
		c.signIn ? plain('When the app asks, sign in. HolyArrs opens in your browser. Press Allow.') : { ...plain('Have the app send this header.'), code: `Authorization: ${bearer(c)}` },
		{ ...plain('Apps with a settings file mostly take this form.'), code: json({ mcpServers: { holyarrs: c.signIn ? { url: c.url } : { url: c.url, headers: { Authorization: bearer(c) } } } }) },
		{
			...plain('App runs only local programs? Use this bridge (needs Node.js).'),
			code: `npx -y mcp-remote@latest ${c.url} --transport http-only${c.url.startsWith('http:') ? ' --allow-http' : ''} --header "Authorization: ${bearer(c)}"`
		}
	]
};

export type Group = 'chat' | 'agents' | 'computer';

/** The table's groups, in the website's order. */
export const GROUPS: [Group, string][] = [
	['chat', 'Chat apps'],
	['agents', 'Agents and editors'],
	['computer', 'On your computer']
];

export type Row = {
	/** The app's slug; ?app= picks it. */
	id: string;
	name: string;
	group: Group;
	/** Where the setting is; `code` in backticks, <address> is the AI address. */
	where: string;
	/** Its guide on the website. */
	guide: string;
	page: string | null;
	deeplink?: 'cursor' | 'vscode';
	app: AIApp;
};

// A "where" that starts with the app's own one-click button ("Add to Cursor,
// or ...") gets that button after the rest, as the VS Code row does, and only
// where the button works (browser sign-in).
const LEAD_DEEPLINK = /^Add to (Cursor|VS Code), or /;

const tableRows: Row[] = GROUPS.flatMap(([g]) =>
	APP_DATA.filter((d) => d.table?.group === g)
		.sort((x, y) => x.table!.order - y.table!.order)
		.map((d) => {
			const lead = d.table!.where.match(LEAD_DEEPLINK);
			const deeplink = d.table!.deeplink ?? (lead ? (lead[1] === 'Cursor' ? 'cursor' : 'vscode') : undefined);
			return {
				id: d.slug,
				name: d.table!.label ?? d.name,
				group: g,
				where: lead ? d.table!.where.slice(lead[0].length) : d.table!.where,
				guide: guideURL(d.page),
				page: d.page,
				...(deeplink ? { deeplink } : {}),
				app: dataApp(d)
			};
		})
);

/** ROWS are the "Where to add HolyArrs" table: the website's rows, then Any other app. */
export const ROWS: Row[] = [
	...tableRows,
	{ id: 'any-app', name: 'Any other app', group: 'computer', where: 'Its remote MCP server settings, with Streamable HTTP', guide: `${GUIDES_URL}#any-other-app`, page: null, app: anyApp }
];

/** What a ?app= link opens: the app's own steps, and the table row that covers it, if any. */
export type Picked = { id: string; row?: Row; app: AIApp };

/** pickFor reads a ?app= link: a row's own slug, or any app in the data. An app
 * without its own row opens its own steps under the row of its guide page
 * (lm-studio under Open WebUI, LM Studio, Ollama; codex-ide under Codex), or
 * on its own. Unknown: nothing. */
export function pickFor(slug: string | null | undefined): Picked | undefined {
	if (!slug) return undefined;
	const own = ROWS.find((r) => r.id === slug) ?? (slug === 'any-other-app' ? ROWS.at(-1) : undefined);
	if (own) return { id: own.id, row: own, app: own.app };
	const d = APP_DATA.find((x) => x.slug === slug);
	const row = ROWS.find((r) => r.page && r.page === (d?.page ?? slug));
	if (d) return { id: d.slug, row, app: dataApp(d) };
	return row ? { id: row.id, row, app: row.app } : undefined;
}

/** appsFor lists the apps one row's guide covers (Codex: CLI, IDE extension,
 * ChatGPT desktop), the row's own app first, so each can be picked. */
export function appsFor(r: Row): AIApp[] {
	if (!r.page) return [r.app];
	return [r.app, ...APP_DATA.filter((d) => d.page === r.page && d.slug !== r.id).map(dataApp)];
}

/** rowFor is the table row a ?app= link marks, if any. */
export function rowFor(slug: string | null | undefined): Row | undefined {
	return pickFor(slug)?.row;
}

/** whereText fills the AI address into a row's "where" cell. On a plain-http
 * address a command in it takes the app's key version, so it sends the key. */
export function whereText(r: Row, url: string, signIn = true): string {
	if (!signIn && r.where.includes('<address>')) {
		const head = r.where.replaceAll('`', '').split(' ').slice(0, 2).join(' ');
		const code = r.app.steps({ url, signIn: false }).find((s) => s.code?.startsWith(head + ' '))?.code;
		if (code) return '`' + code + '`';
	}
	return r.where.replaceAll('<address>', url);
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/** testedLine says whether the steps were tried with HolyArrs, as the website does. */
export function testedLine(a: AIApp): string {
	if (a.id === 'any-app') return '';
	if (a.tested) {
		const [y, m, d] = a.tested.split('-').map(Number);
		return `Tested with HolyArrs on ${d} ${MONTHS[m - 1]} ${y}`;
	}
	return `Not tested with HolyArrs yet. These steps follow ${a.name}'s own help pages.`;
}
