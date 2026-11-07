import { describe, expect, it } from 'vitest';
import { barHeight, emptyForm, formBody, formFrom, ms, sourceTone, uses } from './api.ts';
import type { SourceView } from './types.ts';

const saved: SourceView = {
	id: 3,
	name: 'Lab',
	protocol: 'torrent',
	format: 'torznab',
	url: 'http://lab/api',
	api_key: '••••••••',
	use_for: { feed: true, auto_search: false, hand_search: true },
	kinds: ['movie'],
	categories: { movie: [2000] },
	min_seeders: 2,
	limits: { searches_per_window: 100, window_seconds: 7200, timeout_ms: 15000 },
	state: 'enabled',
	health: { state: 'working', text: 'Working.' },
	priority: 10,
	allow_local_network: true,
	proxy: 'socks5://me@10.0.0.2:1080',
	proxy_password: '••••••••',
	proxy_allow_local_network: true,
	summary: { last_success: null, last_failure: null, calls_24h: 0, failures_24h: 0, avg_ms_24h: 0 }
};

describe('sources', () => {
	it('colours health', () => {
		expect(sourceTone('working')).toBe('ok');
		expect(sourceTone('disabled')).toBe('idle');
		expect(sourceTone('auth_failed')).toBe('fail');
		expect(sourceTone('unreachable')).toBe('fail');
	});

	it('says times', () => {
		expect(ms(0)).toBe('—');
		expect(ms(240)).toBe('240 ms');
		expect(ms(1500)).toBe('1.5 s');
	});

	it('says uses', () => {
		expect(uses(saved)).toBe('newest list · search by hand');
		expect(uses({ use_for: { feed: false, auto_search: false, hand_search: false } })).toBe('not used');
	});

	it('round-trips a saved Source through the form', () => {
		const b = formBody(formFrom(saved));
		expect(b.api_key).toBe('••••••••');
		expect(b.limits).toEqual({ searches_per_window: 100, window_seconds: 7200, gap_ms: 0, timeout_ms: 15000 });
		expect(b.use_for).toEqual({ feed: true, auto_search: false, hand_search: true });
		expect(b.priority).toBe(10);
		expect(b.proxy).toBe('socks5://me@10.0.0.2:1080');
	});

	it('keeps an rss Source to the newest list', () => {
		const f = { ...emptyForm(), name: ' Feed ', format: 'rss' as const, url: 'https://x/rss?passkey=1' };
		const b = formBody(f);
		expect(b.name).toBe('Feed');
		expect(b.use_for).toEqual({ feed: true, auto_search: false, hand_search: false });
	});

	it('drops seeders for usenet', () => {
		expect(formBody({ ...emptyForm(), protocol: 'usenet', min_seeders: 5 }).min_seeders).toBe(0);
	});

	it('scales bars', () => {
		expect(barHeight(0, 10)).toBe(0);
		expect(barHeight(10, 10)).toBe(100);
		expect(barHeight(1, 1000)).toBe(4);
		expect(barHeight(3, 0)).toBe(0);
	});
});

describe('source health by reason', async () => {
	const { healthTone, healthWord, pausedUntil } = await import('./health.ts');
	it('tells waits apart', () => {
		expect(healthWord({ state: 'unreachable', reason: 'slowed' })).toBe('Slowed down');
		expect(healthTone({ state: 'unreachable', reason: 'slowed' })).toBe('wait');
		expect(healthWord({ state: 'unreachable', reason: 'blocked' })).toBe('Blocked');
		expect(healthWord({ state: 'auth_failed', reason: 'key' })).toBe('Key refused');
		expect(healthTone({ state: 'unreachable', reason: 'down' })).toBe('fail');
	});
	it('falls back to the state', () => {
		expect(healthWord({ state: 'working' })).toBe('Working');
		expect(healthTone({ state: 'disabled' })).toBe('idle');
		expect(healthWord({ state: 'unreachable' })).toBe('Unreachable');
	});
	it('says paused until a time', () => {
		expect(pausedUntil({})).toBe('');
		expect(pausedUntil({ until: 'nope' })).toBe('');
		expect(pausedUntil({ until: '2026-11-02T14:05:00Z' })).toMatch(/^Paused until /);
	});
});
