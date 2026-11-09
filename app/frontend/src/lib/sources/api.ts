import type { SourceHealth, SourceView } from './types.ts';
import type { Tone } from '../core/format.ts';

export interface SourceForm {
	name: string;
	format: 'torznab' | 'newznab' | 'rss';
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

export const healthWords: Record<SourceHealth['state'], string> = {
	working: 'Working',
	unreachable: 'Unreachable',
	auth_failed: 'Key turned down',
	disabled: 'Paused'
};

export const opWords: Record<string, string> = { caps: 'Read what it can do', search: 'Search', feed: 'Newest list', test: 'Test' };

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
	if (s.use_for?.feed) out.push('newest list');
	if (s.use_for?.auto_search) out.push('searches on its own');
	if (s.use_for?.hand_search) out.push('search by hand');
	return out.length ? out.join(' · ') : 'not used';
}

export function emptyForm(): SourceForm {
	return {
		name: '',
		format: 'torznab',
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

// An rss Source only feeds.
export function formBody(f: SourceForm): Record<string, unknown> {
	const rss = f.format === 'rss';
	return {
		name: f.name.trim(),
		format: f.format,
		protocol: f.protocol,
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
