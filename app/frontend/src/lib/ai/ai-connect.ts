import type { ApiKey } from '../core/types.ts';
import { APP_DATA, type AppData, type AppStep } from './ai-apps.ts';

export type Part = { text: string; b?: boolean; code?: boolean };

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
	cloud?: boolean;
	signIn: boolean;
	needs?: string;
	note?: string;
	check?: string;
	tested?: string | null;
	clientIdHelper?: boolean;
	steps: (c: Ctx) => Step[];
};

export type Group = 'chat' | 'agents' | 'computer';

export type Row = {
	id: string;
	name: string;
	group: Group;
	where: string;
	guide: string;
	page: string | null;
	deeplink?: 'cursor' | 'vscode';
	app: AIApp;
};

export type Picked = { id: string; row?: Row; app: AIApp };

export const AI_SCOPE = 'mcp';

export const GUIDES_URL = 'https://holyarrs.coderluii.dev/docs/ai/';

export const CHECK_QUESTION = "What's downloading in HolyArrs?";

export const DEEPLINK_LABEL = { cursor: 'Add to Cursor', vscode: 'Add to VS Code' } as const;

export const GROUPS: [Group, string][] = [
	['chat', 'Chat apps'],
	['agents', 'Agents and editors'],
	['computer', 'On your computer']
];

const KEY_HOLDER = '<your AI key>';

const GLOBAL_BOLD = ['Settings › AI apps', 'Make client ID', 'Make a key', 'Allow'];
const CODE_WORDS =
	/(~\/[\w./-]+|%\w+%\\[\w.\\*]+|(?<![\w-])--[a-z][a-z-]+|\b[A-Z][A-Z0-9]*_[A-Z0-9_]+\b|\bclaude mcp login holyarrs\b|(?<![\w/])\/mcp\b|\b[\w-]+\.(?:json|toml)\b|\bholyarrs\.json\b)/g;
const reEsc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

// A "where" cell that starts with the app's one-click button gets that button after the rest.
const LEAD_DEEPLINK = /^Add to (Cursor|VS Code), or /;

const fill = (t: string, c: Ctx) => t.replaceAll('{url}', c.url).replaceAll('{key}', c.key ?? KEY_HOLDER);
const bearer = (c: Ctx) => `Bearer ${c.key ?? KEY_HOLDER}`;
const json = (v: unknown) => JSON.stringify(v, null, 2);
const plain = (text: string): Step => ({ text, parts: stepParts(text) });

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

export const ROWS: Row[] = [
	...tableRows,
	{ id: 'any-app', name: 'Any other app', group: 'computer', where: 'Its remote MCP server settings, with Streamable HTTP', guide: `${GUIDES_URL}#any-other-app`, page: null, app: anyApp }
];

export function isAIKey(k: ApiKey): boolean {
	return (k.scopes ?? []).includes(AI_SCOPE);
}

export function aiURL(origin: string, base = ''): string {
	return origin.replace(/\/+$/, '') + base.replace(/\/+$/, '') + '/api/v1/mcp';
}

// Apps take browser sign-in only on https or this computer.
export function canSignIn(url: string): boolean {
	try {
		const u = new URL(url);
		return u.protocol === 'https:' || ['localhost', '127.0.0.1', '[::1]'].includes(u.hostname);
	} catch {
		return false;
	}
}

// Private, Tailscale and loopback addresses, and plain http, cannot be reached from the internet.
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

export function guideURL(page?: string | null): string {
	return page ? `${GUIDES_URL}${page}/` : `${GUIDES_URL}#apps`;
}

export function agentPrompt(origin: string, o: { key?: string; home?: boolean } = {}): string {
	const read = `Read ${GUIDES_URL}ai.md and connect yourself to my HolyArrs at ${origin}.`;
	const rest = 'Only add the HolyArrs MCP server. Ask me before you change anything else.';
	if (o.key) return `${read} Use this AI key: ${o.key}. ${rest}`;
	return o.home ? `${read} ${rest} I'll give you an AI key when you ask.` : `${read} ${rest}`;
}

export function codeParts(text: string): Part[] {
	return text
		.split('`')
		.map((t, i) => (i % 2 ? { text: t, code: true } : { text: t }))
		.filter((p) => p.text);
}

// Bold labels are swapped for numbered markers first so the code pass cannot touch them.
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

export function deeplinkHref(kind: 'cursor' | 'vscode', url: string): string {
	if (kind === 'cursor') {
		const json = new TextEncoder().encode(JSON.stringify({ url }));
		return `cursor://anysphere.cursor-deeplink/mcp/install?name=holyarrs&config=${btoa(String.fromCharCode(...json))}`;
	}
	return `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name: 'holyarrs', type: 'http', url }))}`;
}

// Steps done inside HolyArrs are left out because the page has a button for them.
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

// An app without its own row opens under the row of its guide page, or alone.
export function pickFor(slug: string | null | undefined): Picked | undefined {
	if (!slug) return undefined;
	const own = ROWS.find((r) => r.id === slug) ?? (slug === 'any-other-app' ? ROWS.at(-1) : undefined);
	if (own) return { id: own.id, row: own, app: own.app };
	const d = APP_DATA.find((x) => x.slug === slug);
	const row = ROWS.find((r) => r.page && r.page === (d?.page ?? slug));
	if (d) return { id: d.slug, row, app: dataApp(d) };
	return row ? { id: row.id, row, app: row.app } : undefined;
}

export function appsFor(r: Row): AIApp[] {
	if (!r.page) return [r.app];
	return [r.app, ...APP_DATA.filter((d) => d.page === r.page && d.slug !== r.id).map(dataApp)];
}

// Without sign-in, a command in the cell takes the app's key version so it sends the key.
export function whereText(r: Row, url: string, signIn = true): string {
	if (!signIn && r.where.includes('<address>')) {
		const head = r.where.replaceAll('`', '').split(' ').slice(0, 2).join(' ');
		const code = r.app.steps({ url, signIn: false }).find((s) => s.code?.startsWith(head + ' '))?.code;
		if (code) return '`' + code + '`';
	}
	return r.where.replaceAll('<address>', url);
}

export function testedLine(a: AIApp): string {
	if (a.id === 'any-app') return '';
	if (a.tested) {
		const [y, m, d] = a.tested.split('-').map(Number);
		return `Tested with HolyArrs on ${d} ${MONTHS[m - 1]} ${y}`;
	}
	return `Not tested with HolyArrs yet. These steps follow ${a.name}'s own help pages.`;
}

function toStep(s: AppStep, c: Ctx, signIn: boolean): Step {
	return {
		text: fill(s.text, c),
		parts: stepParts(s.text, s.bold).map((p) => ({ ...p, text: fill(p.text, c) })),
		...(s.code ? { code: fill(s.code, c) } : {}),
		...(s.link ? { link: s.link } : {}),
		...(s.deeplink && signIn ? { deeplink: s.deeplink } : {})
	};
}
