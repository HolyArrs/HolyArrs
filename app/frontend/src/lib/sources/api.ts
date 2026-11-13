import type { SourceHealth, SourceView } from './types.ts';
import type { Tone } from '../core/format.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export interface SourceForm {
	name: string;
	format: 'torznab' | 'newznab' | 'rss' | 'site';
	// site is the built-in site a site Source reads.
	site: string;
	protocol: 'torrent' | 'usenet';
	url: string;
	api_key: string;
	feed: boolean;
	auto_search: boolean;
	hand_search: boolean;
	kinds: string[];
	min_seeders: number;
	priority: number;
	searches_per_window: number;
	window_hours: number;
	gap_ms: number;
	timeout_s: number;
	allow_local_network: boolean;
	proxy: string;
	proxy_password: string;
	proxy_allow_local_network: boolean;
	tags: string[];
}

// Getters, so each line follows the language picked when it is read.
export const healthWords: Record<SourceHealth['state'], string> = {
	get working() { return t('sources.health.working'); },
	get unreachable() { return t('sources.health.unreachable'); },
	get auth_failed() { return t('sources.health.authFailed'); },
	get disabled() { return t('sources.health.disabled'); }
};

export const opWords: Record<string, string> = {
	get caps() { return t('sources.op.caps'); },
	get search() { return t('sources.op.search'); },
	get feed() { return t('sources.op.feed'); },
	get test() { return t('sources.op.test'); }
};

export function sourceTone(state: string): Tone {
	if (state === 'working') return 'ok';
	if (state === 'disabled') return 'idle';
	return 'fail';
}

export function ms(n: number | null | undefined): string {
	if (!n) return '—';
	if (n < 1000) return `${Math.round(n)} ms`;
	return `${(n / 1000).toFixed(1)} s`;
}

export function uses(s: Pick<SourceView, 'use_for'>): string {
	const out: string[] = [];
	if (s.use_for?.feed) out.push(t('sources.uses.feed'));
	if (s.use_for?.auto_search) out.push(t('sources.uses.auto'));
	if (s.use_for?.hand_search) out.push(t('sources.uses.hand'));
	return out.length ? out.join(' · ') : t('sources.uses.none');
}

export function emptyForm(): SourceForm {
	return {
		name: '',
		format: 'torznab',
		site: '',
		protocol: 'torrent',
		url: '',
		api_key: '',
		feed: true,
		auto_search: true,
		hand_search: true,
		kinds: ['movie', 'tv', 'music'],
		min_seeders: 1,
		priority: 25,
		searches_per_window: 0,
		window_hours: 1,
		gap_ms: 0,
		timeout_s: 30,
		allow_local_network: false,
		proxy: '',
		proxy_password: '',
		proxy_allow_local_network: false,
		tags: []
	};
}

export function formFrom(s: SourceView): SourceForm {
	const f = emptyForm();
	return {
		...f,
		name: s.name,
		format: s.format === 'bridge' ? 'torznab' : s.format,
		site: s.site ?? '',
		protocol: s.protocol,
		url: s.url,
		api_key: s.api_key,
		feed: s.use_for?.feed ?? false,
		auto_search: s.use_for?.auto_search ?? false,
		hand_search: s.use_for?.hand_search ?? false,
		kinds: s.kinds ?? [],
		min_seeders: s.min_seeders ?? 0,
		priority: s.priority,
		searches_per_window: s.limits?.searches_per_window ?? 0,
		window_hours: s.limits?.window_seconds ? s.limits.window_seconds / 3600 : 1,
		gap_ms: s.limits?.gap_ms ?? 0,
		timeout_s: s.limits?.timeout_ms ? s.limits.timeout_ms / 1000 : 30,
		allow_local_network: s.allow_local_network,
		proxy: s.proxy,
		proxy_password: s.proxy_password,
		proxy_allow_local_network: s.proxy_allow_local_network,
		tags: [...(s.tags ?? [])]
	};
}

// The built-in sites a Source can read directly, with their polite limits.
// Names are the sites' own names, so they are not translated.
export const builtInSites = [
	{ id: 'knaben', name: 'Knaben', url: 'https://api.knaben.org', key: false, kinds: ['movie', 'tv', 'music'], gap_ms: 2000, per_hour: 100, timeout_s: 20 },
	{ id: 'torrentscsv', name: 'TorrentsCSV', url: 'https://torrents-csv.com', key: false, kinds: ['movie', 'tv', 'music'], gap_ms: 2000, per_hour: 100, timeout_s: 20 },
	{ id: 'subsplease', name: 'SubsPlease', url: 'https://subsplease.org', key: false, kinds: ['tv'], gap_ms: 2000, per_hour: 100, timeout_s: 20 },
	{ id: 'iptorrents', name: 'IPTorrents', url: 'https://iptorrents.com', key: true, kinds: ['movie', 'tv', 'music'], gap_ms: 3000, per_hour: 60, timeout_s: 30 }
] as const;

// pickSite fills a new form with a built-in site's address and limits.
export function pickSite(f: SourceForm, id: string): void {
	const s = builtInSites.find((x) => x.id === id);
	if (!s) return;
	const before = builtInSites.find((x) => x.id === f.site);
	f.site = s.id;
	f.protocol = 'torrent';
	f.url = s.url;
	if (!f.name.trim() || f.name === before?.name) f.name = s.name;
	f.kinds = [...s.kinds];
	f.gap_ms = s.gap_ms;
	f.searches_per_window = s.per_hour;
	f.window_hours = 1;
	f.timeout_s = s.timeout_s;
}

// An rss Source only feeds.
export function formBody(f: SourceForm): Record<string, unknown> {
	const rss = f.format === 'rss';
	return {
		name: f.name.trim(),
		format: f.format,
		...(f.format === 'site' ? { site: f.site } : {}),
		protocol: f.format === 'site' ? 'torrent' : f.protocol,
		url: f.url.trim(),
		api_key: f.api_key,
		use_for: { feed: f.feed, auto_search: rss ? false : f.auto_search, hand_search: rss ? false : f.hand_search },
		kinds: f.kinds,
		min_seeders: f.protocol === 'torrent' ? Number(f.min_seeders) || 0 : 0,
		priority: Number(f.priority) || 25,
		limits: {
			searches_per_window: Number(f.searches_per_window) || 0,
			window_seconds: Math.round((Number(f.window_hours) || 0) * 3600),
			gap_ms: Number(f.gap_ms) || 0,
			timeout_ms: Math.round((Number(f.timeout_s) || 0) * 1000)
		},
		allow_local_network: f.allow_local_network,
		proxy: f.proxy.trim(),
		proxy_password: f.proxy_password,
		proxy_allow_local_network: f.proxy_allow_local_network,
		tags: f.tags
	};
}

export function barHeight(n: number, most: number): number {
	if (!most || n <= 0) return 0;
	return Math.max(4, Math.round((n / most) * 100));
}
