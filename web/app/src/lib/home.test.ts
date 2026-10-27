import { describe, expect, it } from 'vitest';
import { arrivedAt, libraryCards, n, arrivedTitle, askProgressLine, attention, fallbackRows, groupAttention, homeStrip, downloadLine, downloadSubtitle, healthFix, homeFresh, isActive, mixLine, soonLine, splitArrivals } from './home.ts';
import type { HealthCheck } from './types.ts';

const check = (key: string, state: HealthCheck['state'], kind: HealthCheck['kind'] = 'source'): HealthCheck => ({ id: 1, key, kind, subject: 'S', state, message: `${key} broke`, since: '', checked_at: '' });

describe('home', () => {
	it('writes the download meta line', () => {
		expect(downloadLine({ progress: 0.4, eta: 600, state: 'downloading', label: '' })).toBe('40% · 10m left');
		expect(downloadLine({ progress: 1.4, state: 'downloading', label: '' })).toBe('100%');
		expect(downloadLine({ progress: 0, state: 'queued', label: '' })).toBe('Waiting to start');
		expect(downloadSubtitle({ covers: 'S01E01-E08, 8 episodes', version: 'Main' })).toBe('S01E01-E08, 8 episodes');
		expect(downloadSubtitle({ year: 2023, version: 'Main' })).toBe('2023');
		expect(downloadSubtitle({ year: 2023, version: '4K' })).toBe('2023 · 4K');
		expect(isActive({ active: false, state: 'stalled' })).toBe(true);
		expect(isActive({ active: false, state: 'finished' })).toBe(false);
	});

	it('lists what needs attention, worst first, each with its fix', () => {
		const items = attention({
			failed: [{ id: 3, title: 'Barbie', title_id: 4, text: '' }],
			decisions: [{ slot_id: 7, title: 'Harbour Lights', reason: 'Ran out.', episode: { label: 'S02E01' } }],
			health: [check('w', 'warning', 'disk_space'), check('e', 'error', 'downloader'), check('ok', 'ok')]
		});
		expect(items.map((i) => i.word)).toEqual(['Problem', 'Failed', 'Your call', 'Warning']);
		expect(items[0].href).toBe('/settings/downloaders');
		expect(items[1].text).toBe('Barbie: The download failed.');
		expect(items[2]).toMatchObject({ text: 'Harbour Lights S02E01: Ran out.', href: '/downloads/decide' });
		expect(items[3].href).toBe('/settings/library-folders');
		expect(healthFix('update')).toBe('/settings/this-app');
	});

	it('puts the title and the reason on one line, in the glossary words', () => {
		const items = attention({
			failed: [{ id: 1, title: 'Gladiator', title_id: 2, text: 'The Downloader says error.' }, { id: 2, title: 'Alien', title_id: 3, text: '' }],
			decisions: [{ slot_id: 7, title: 'Harbour Lights', reason: 'Ran out.', episode: { label: 'S02E01' } }],
			health: [check('s', 'error', 'source'), check('b', 'warning', 'backups')]
		});
		expect(items.map((i) => `${i.title} · ${i.reason}`)).toEqual(['Source · not answering', 'Gladiator · download failed', 'Alien · download failed', 'Harbour Lights S02E01 · needs a decision', 'Backups · getting old']);
		// What the Downloader said stays one tap away; an empty reason adds nothing.
		expect(items[1].note).toBe('The Downloader says error.');
		expect(items[2].note).toBeUndefined();
		expect(items.map((i) => i.action)).toEqual(['Fix', 'Downloads', 'Downloads', 'Decide', 'Look']);
	});

	it('merges rows that share a cause and keeps the rest apart', () => {
		const items = attention({
			failed: [{ id: 1, title: 'Gladiator', title_id: 2, text: 'x' }, { id: 2, title: 'Everything Everywhere All at Once', title_id: 3, text: 'y' }],
			decisions: [],
			health: [check('s1', 'error', 'source'), check('s2', 'error', 'source'), check('d', 'error', 'downloader'), check('w', 'warning', 'source')]
		});
		const rows = groupAttention(items);
		expect(rows.map((r) => `${r.title} · ${r.reason}`)).toEqual(['2 Sources · not answering', 'Downloader · not answering', '2 downloads failed · Gladiator, Everything Everywhere All at Once', 'Source · answered with a problem']);
		expect(rows.map((r) => r.items.length)).toEqual([2, 1, 2, 1]);
		expect(rows[2]).toMatchObject({ tone: 'fail', word: 'Failed', href: '/downloads', action: 'Downloads' });
		expect(rows.reduce((n, r) => n + r.items.length, 0)).toBe(items.length);
		// A lone item keeps its own key, so its row does not jump when another joins it.
		expect(rows[1].key).toBe(items[2].key);
	});
});


describe('home rows', () => {
	const now = Date.parse('2026-10-20T12:00:00Z');
	const t = (id: number, kind: string, ...days: number[]) => ({ id, kind, slots: days.map((d) => ({ file: { AddedAt: new Date(now - d * 86400000).toISOString() } })) }) as never;
	it('splits arrivals so no title is on both rows', () => {
		const { arrived, earlier } = splitArrivals([t(1, 'movie', 30), t(2, 'movie', 2), t(3, 'show', 5, 1), t(4, 'movie')], now);
		expect(arrived.map((x: { id: number }) => x.id)).toEqual([3, 2]);
		expect(earlier.map((x: { id: number }) => x.id)).toEqual([4, 1]);
		expect(arrivedAt(t(5, 'movie'))).toBeUndefined();
	});
	it('picks one factual freshness strip', () => {
		expect(homeFresh(t(1, 'show', 2, 40), [], now)?.kind).toBe('new_episode');
		expect(homeFresh(t(1, 'movie', 2), [], now)?.kind).toBe('recently_added');
		expect(homeFresh(t(1, 'show', 40), [{ title_id: 1, date: '2026-10-25', kind: 'episode', season: 3, episode: 1 }], now)).toEqual({ kind: 'season_coming', text: 'Season 3 Coming' });
		expect(homeFresh(t(1, 'movie', 40), [], now)).toBeNull();
		expect(homeFresh(undefined, [], now)).toBeNull();
	});
	it('writes true row titles and lines', () => {
		expect(mixLine([{ kind: 'movie' }, { kind: 'show' }, { kind: 'show' }])).toBe('1 movie and 2 shows');
		expect(mixLine([])).toBe('');
		expect(arrivedTitle(now - 3600000, now)).toBe('New today');
		expect(arrivedTitle(now - 3 * 86400000, now)).toBe('New this week');
		expect(arrivedTitle(now - 10 * 86400000, now)).toBe('Just arrived');
		expect(arrivedTitle(now - 40 * 86400000, now)).toBe('Recently added');
		expect(soonLine({ name: 'Dune', subtitle: 'Fri' })).toBe('Next up: Dune, Fri');
		expect(soonLine(undefined)).toBe('The next 30 days');
	});
	it('labels the server-filled Home cards', () => {
		expect(homeStrip({ kind: 'show', name: 'Andor', line: 'S2 · 3 new episodes' })).toEqual({ kind: 'new_episode', text: 'S2 · 3 new episodes' });
		expect(homeStrip({ kind: 'show', name: 'Andor', line: 'Season 2 added' })).toEqual({ kind: 'new_season', text: 'Season 2 added' });
		expect(homeStrip({ kind: 'artist', name: 'Adele', line: '3 new albums' })).toEqual({ kind: 'recently_added', text: 'Adele · 3 new albums' });
		expect(homeStrip({ kind: 'movie', name: 'Dune', line: 'New movie' })).toBeNull();
	});

	it('writes live ask progress', () => {
		expect(askProgressLine({ progress: 0.42, state: 'downloading', eta: 600 })).toBe('Downloading 42% · 10m left');
		expect(askProgressLine({ progress: 0, state: 'queued' })).toBe('Waiting to start');
		expect(askProgressLine(undefined)).toBe('');
	});

	it('keeps members off the owner rows while Home is unreachable', () => {
		expect(fallbackRows(false)).toEqual(['asks', 'soon', 'discover']);
		expect(fallbackRows(true)[0]).toBe('attention');
	});
});

describe('Your Library cards', () => {
	it('keeps sidebar order, names seasons and tracks, and splits the bar', () => {
		const cards = libraryCards({
			artist: { titles: 1, parts: 3, items: 40, on_disk: 30, missing: 10, downloading: 0, size: 900 },
			movie: { titles: 1200, parts: 0, items: 1200, on_disk: 1100, missing: 60, downloading: 40, size: 5e12 },
			show: { titles: 12, parts: 1, items: 1, on_disk: 1, missing: 0, downloading: 0, size: 2 }
		});
		expect(cards.map((c) => c.kind)).toEqual(['movie', 'show', 'artist']);
		expect(cards[0]).toMatchObject({ big: 1200, unit: 'movies', detail: '', href: '/library?kind=movie' });
		expect(cards[0].segments.map((s) => [s.key, s.count, s.share])).toEqual([['on_disk', 1100, 1100 / 1200], ['downloading', 40, 40 / 1200], ['missing', 60, 60 / 1200]]);
		expect(cards[0].segments[2].href).toBe('/library?kind=movie&show=missing');
		expect(cards[1].detail).toBe('1 season · 1 episode');
		expect(cards[2]).toMatchObject({ unit: 'artist', detail: '3 albums · 40 tracks' });
	});
	it('leaves out kinds that are off and copes with an empty kind', () => {
		expect(libraryCards(undefined)).toEqual([]);
		const [m] = libraryCards({ movie: { titles: 0, parts: 0, items: 0, on_disk: 0, missing: 0, downloading: 0, size: 0 } });
		expect(m.segments.every((s) => s.share === 0)).toBe(true);
		expect(n(12345)).toBe('12,345');
	});
});
