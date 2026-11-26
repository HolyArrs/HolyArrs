// Home cards, title page sections and pages that running plugins add. A plugin sends plain data;
// HolyArrs draws it, so no plugin markup or script ever runs in the app's own page.
import { api } from '#lib/core/api.ts';

export interface CardRow {
	text: string;
	sub?: string;
	title_id?: number;
	link?: string;
}
export interface PluginCard {
	plugin: string;
	title: string;
	rows: CardRow[];
}
export interface PluginPageEntry {
	plugin: string;
	id: string;
	title: string;
}

// A row's link goes only to a page inside HolyArrs: one leading slash, no scheme, no host, no climbing.
export function cardLink(link: string | undefined): string | undefined {
	if (!link || !/^\/[a-z0-9][a-z0-9/_\-?=&.]*$/.test(link) || link.includes('..') || link.startsWith('//') || link.startsWith('/api/') || link === '/api') return undefined;
	return link;
}

export async function homeCards(): Promise<PluginCard[]> {
	const r = await api.get<{ widgets?: PluginCard[] }>('/plugin-widgets').catch(() => null);
	return r?.widgets ?? [];
}

export async function titleSections(titleId: number): Promise<PluginCard[]> {
	if (!titleId) return [];
	const r = await api.get<{ sections?: PluginCard[] }>(`/titles/${titleId}/plugin-sections`).catch(() => null);
	return r?.sections ?? [];
}

export async function pluginPages(): Promise<PluginPageEntry[]> {
	const r = await api.get<{ pages?: PluginPageEntry[] }>('/plugin-pages').catch(() => null);
	return r?.pages ?? [];
}

// One message a sandboxed plugin page may send: an API call by method and path.
export interface BridgeCall {
	id: string | number;
	method: 'GET' | 'POST' | 'PUT' | 'DELETE';
	path: string;
	body?: unknown;
}

// bridgeCall reads a postMessage from the page's frame. Only the frame's own window counts, and only a
// call shaped like BridgeCall to an /api/v1/ path; the server then checks the plugin's scopes and the
// person's own permission.
export function bridgeCall(e: { source: unknown; origin: string; data: unknown }, frame: unknown): BridgeCall | null {
	if (!frame || e.source !== frame || e.origin !== 'null') return null;
	const d = e.data as Partial<BridgeCall> | null;
	if (!d || typeof d !== 'object' || (typeof d.id !== 'string' && typeof d.id !== 'number')) return null;
	if (!['GET', 'POST', 'PUT', 'DELETE'].includes(String(d.method))) return null;
	if (typeof d.path !== 'string' || !d.path.startsWith('/api/v1/') || d.path.includes('..') || d.path.includes('//')) return null;
	// Streams and the player are the app's own: a page never asks for a link or the player's route.
	if (/^\/api\/v1\/(plugin-stream\/|plugin-hooks\/stream|plugin-pages\/)/.test(d.path)) return null;
	return { id: d.id, method: d.method as BridgeCall['method'], path: d.path, body: d.body };
}

export interface SidebarPluginPage {
	plugin: string;
	name: string;
	id: string;
	title: string;
}

/** The plugin pages for the sidebar, each with its plugin's shown name (owner only). */
export async function sidebarPluginPages(): Promise<SidebarPluginPage[]> {
	const [pages, ov] = await Promise.all([pluginPages(), api.get<{ plugins?: { id: string; name?: string }[] }>('/plugins').catch(() => null)]);
	const names = new Map((ov?.plugins ?? []).map((p) => [p.id, p.name || p.id]));
	return pages.map((p) => ({ plugin: p.plugin, name: names.get(p.plugin) ?? p.plugin, id: p.id, title: p.title }));
}

// One message a page may send to the app's player: play a file by id, pause, resume, seek or close.
// The page never gets a stream address; the app shows the video itself and sends back events.
export interface PlayerCall {
	id: string | number;
	op: 'play' | 'pause' | 'resume' | 'seek' | 'close';
	file_id?: number;
	time?: number;
}

/** playerCall reads a player message from the page's frame (same rules as bridgeCall: only the frame's own window, origin "null"). */
export function playerCall(e: { source: unknown; origin: string; data: unknown }, frame: unknown): PlayerCall | null {
	if (!frame || e.source !== frame || e.origin !== 'null') return null;
	const d = e.data as { id?: unknown; player?: unknown; file_id?: unknown; time?: unknown } | null;
	if (!d || typeof d !== 'object' || (typeof d.id !== 'string' && typeof d.id !== 'number')) return null;
	if (!['play', 'pause', 'resume', 'seek', 'close'].includes(String(d.player))) return null;
	const op = d.player as PlayerCall['op'];
	if (op === 'play') {
		if (typeof d.file_id !== 'number' || !Number.isSafeInteger(d.file_id) || d.file_id <= 0) return null;
		return { id: d.id, op, file_id: d.file_id };
	}
	if (op === 'seek') {
		if (typeof d.time !== 'number' || !Number.isFinite(d.time) || d.time < 0) return null;
		return { id: d.id, op, time: d.time };
	}
	return { id: d.id, op };
}
