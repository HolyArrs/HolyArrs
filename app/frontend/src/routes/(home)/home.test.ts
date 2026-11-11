import { describe, expect, it } from 'vitest';
import { arrivedAt, libraryCards, n, arrivedTitle, arrivedHead, askProgressLine, attention, downloadMoved, downloadingNow, homeDownloadsURL, notSentNow, fallbackRows, groupAttention, homeFailures, homeStrip, downloadLine, downloadSubtitle, healthFix, homeFresh, isActive, mixLine, moreLink, shownState, soonLine, splitArrivals, uncheckedLine, freeSpace, partCount } from './home.ts';
import type { HealthCheck } from '#lib/core/types.ts';

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
		expect(items[2]).toMatchObject({ text: 'Harbour Lights S02E01: Ran out.', href: '/downloads?show=problems.decide&slot=7' });
		expect(items[1].href).toBe('/downloads?show=problems.failed&open=3');
		expect(items[3].href).toBe('/settings/library-folders');
		expect(healthFix('update')).toBe('/settings/this-app');
	});

	it('puts the title and the reason on one line, in the glossary words', () => {
		const items = attention({
			failed: [{ id: 1, title: 'Gladiator', title_id: 2, text: 'The Downloader says error.' }, { id: 2, title: 'Alien', title_id: 3, text: '' }],
			decisions: [{ slot_id: 7, title: 'Harbour Lights', reason: 'Ran out.', episode: { label: 'S02E01' } }],
			health: [check('s', 'error', 'source'), check('b', 'warning', 'backups')]
		});
		expect(items.map((i) => `${i.title} · ${i.reason}`)).toEqual(['Source · S not answering', 'Gladiator · download failed', 'Alien · download failed', 'Harbour Lights S02E01 · needs a decision', 'Backups · getting old']);
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
		expect(rows.map((r) => `${r.title} · ${r.reason}`)).toEqual(['2 Sources · S, S not answering', 'Downloader · S not answering', '2 downloads failed · Gladiator, Everything Everywhere All at Once', 'Source · S has a problem']);
		expect(rows.map((r) => r.items.length)).toEqual([2, 1, 2, 1]);
		expect(rows[2]).toMatchObject({ tone: 'fail', word: 'Failed', href: '/downloads?show=problems.failed', action: 'Downloads' });
		expect(rows.reduce((n, r) => n + r.items.length, 0)).toBe(items.length);
		// A lone item keeps its own key, so its row does not jump when another joins it.
		expect(rows[1].key).toBe(items[2].key);
	});

	it('counts failures with the Downloads rule: replaced ones are done, refused imports count', () => {
		const row = (id: number, phase: string, state: string, replaced?: boolean) => ({ id, phase, state, progress: 0, replaced });
		const failed = homeFailures([row(1, 'failed', 'failed', true), row(2, 'failed', 'failed'), row(3, 'import_refused', 'completed'), row(4, 'vanished', 'vanished'), row(5, 'vanished', 'vanished', true), row(6, 'downloading', 'downloading')]);
		expect(failed.map((d) => d.id)).toEqual([2, 3, 4]);
		const [, refused] = attention({ failed: [{ id: 2, title: 'Alien', title_id: 1, text: '', phase: 'failed' }, { id: 3, title: 'Barbie', title_id: 2, text: '', phase: 'import_refused' }], decisions: [], health: [] });
		expect(`${refused.title} · ${refused.reason}`).toBe('Barbie · import refused');
		expect(refused.text).toBe('Barbie: The import was refused.');
	});

	it('keeps the metadata outage while it lasts, and sends people without Settings to Status', () => {
		const items = attention({ failed: [], decisions: [], health: [check('e', 'error', 'downloader'), check('w', 'warning', 'trailers' as HealthCheck['kind'])], metadata: 'Search pauses.', admin: false });
		expect(items.map((i) => `${i.title} · ${i.reason}`)).toEqual(['Downloader · S not answering', "HolyArrs metadata · can't be reached", 'Trailers · lookups failing']);
		expect(items.map((i) => [i.href, i.action])).toEqual([['/health', 'Look'], ['/health', 'Look'], ['/health', 'Look']]);
		expect(items[1].note).toBe('Search pauses.');
		const own = attention({ failed: [], decisions: [], health: [check('e', 'error', 'downloader')], metadata: 'x' });
		expect(own.map((i) => i.href)).toEqual(['/settings/downloaders', '/settings/tmdb']);
		expect(attention({ failed: [], decisions: [], health: [] })).toEqual([]);
	});

	it('names the missing key kit apart from old backups', () => {
		const kit = { ...check('backups:key', 'warning', 'backups'), subject: 'Backups' };
		const items = attention({ failed: [], decisions: [], health: [check('backups', 'warning', 'backups'), kit] });
		expect(items.map((i) => `${i.title} · ${i.reason}`)).toEqual(['Backups · getting old', 'Master key · no key kit saved']);
		expect(items[1]).toMatchObject({ href: '/settings/backups', text: 'No key kit is saved: backups do not hold the master key.' });
		// Two different problems stay two rows.
		expect(groupAttention(items).map((r) => r.title)).toEqual(['Backups', 'Master key']);
	});

	it('keeps the key kit sentence while the flap guard still holds the warning', () => {
		const held = { ...check('backups:key', 'ok', 'backups'), confirmed: 'warning' as const };
		const [item] = attention({ failed: [], decisions: [], health: [held] });
		expect(item).toMatchObject({ title: 'Master key', text: 'No key kit is saved: backups do not hold the master key.' });
	});

	it('tells people who are not the owner that the owner saves the key kit', () => {
		const kit = { ...check('backups:key', 'warning', 'backups'), message: 'The owner should download the key kit on the Backups page: backups do not hold the master key' };
		const [item] = attention({ failed: [], decisions: [], health: [kit] });
		expect(item.text).toBe('The owner should save the key kit: backups do not hold the master key.');
	});

	it('names the /config error and waiting downloads, linked to where they are fixed', () => {
		const items = attention({ failed: [], decisions: [], health: [check('config_folder', 'error', 'config_folder'), check('waiting_downloads', 'warning', 'waiting_downloads')] });
		expect(items.map((i) => `${i.title} · ${i.reason}`)).toEqual(['Config folder · lost if the container is recreated', 'Downloads · waiting for their files']);
		expect(items.map((i) => [i.href, i.action])).toEqual([['/health', 'Look'], ['/downloads?show=waiting.paused', 'Downloads']]);
		expect(items[0].text).toBe('/config is not mapped to a folder, so HolyArrs data is lost when the container is recreated.');
		const member = attention({ failed: [], decisions: [], health: [check('config_folder', 'error', 'config_folder'), check('waiting_downloads', 'error', 'waiting_downloads')], admin: false });
		expect(member.map((i) => i.href)).toEqual(['/health', '/health']);
	});

	it('follows the flap guard when the server sends the confirmed state', () => {
		const flaky = { ...check('d', 'error', 'downloader'), confirmed: 'ok' as const };
		const held = { ...check('s', 'ok', 'source'), confirmed: 'warning' as const };
		expect(shownState(flaky)).toBe('ok');
		expect(shownState(held)).toBe('warning');
		expect(shownState(check('x', 'error'))).toBe('error');
		expect(shownState({ state: 'error', confirmed: '' })).toBe('error');
		const items = attention({ failed: [], decisions: [], health: [flaky, held] });
		expect(items.map((i) => `${i.word} ${i.title} · ${i.reason}`)).toEqual(['Warning Source · S has a problem']);
	});

	it('links the rows that do not fit to the page that holds them, counting rows', () => {
		const g = (cause: string, n = 1) => ({ items: Array.from({ length: n }, () => ({ cause }) as never) });
		expect(moreLink([])).toBeNull();
		expect(moreLink([g('health:source:warning', 2), g('health:backups:warning')])).toEqual({ href: '/settings/health', text: '2 more on Health' });
		expect(moreLink([g('health:source:warning'), g('metadata')], false)).toEqual({ href: '/health', text: '2 more on Status' });
		expect(moreLink([g('download_failed', 3), g('decision')])).toEqual({ href: '/downloads', text: '2 more in Downloads' });
		expect(moreLink([g('decision'), g('health:update:warning')])).toEqual({ href: '/downloads', text: '2 more in Downloads' });
	});

	it('says plainly what Home could not check', () => {
		expect(uncheckedLine(true, false)).toBe('Home could not check downloads right now.');
		expect(uncheckedLine(false, true)).toBe('Home could not check health right now.');
		expect(uncheckedLine(true, true)).toBe('Home could not check downloads or health right now.');
	});

	it('lists what is downloading now, furthest along first, without client-only or finished rows', () => {
		const row = (id: number, phase: string, state: string, progress: number, extra: object = {}) => ({ id, phase, state, progress, active: state !== 'completed' && state !== 'failed', ...extra });
		const rows = [
			row(1, 'downloading', 'downloading', 0.2),
			row(2, 'waiting', 'queued', 0),
			row(3, 'downloading', 'downloading', 0.8),
			row(4, 'done', 'seeding', 1, { active: true }),
			row(5, 'downloading', 'downloading', 0.5, { client_item: true, outside: true }),
			row(6, 'failed', 'failed', 0.3),
			row(7, 'done', 'completed', 1),
			row(8, 'stalled', 'stalled', 0.2)
		];
		expect(downloadingNow(rows).map((d) => d.id)).toEqual([3, 8, 1, 2]);
		expect(downloadingNow(rows, 2).map((d) => d.id)).toEqual([3, 8]);
		expect(downloadingNow([])).toEqual([]);
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
		// Downloading links to Downloads only for people who may open it.
		expect(cards[0].segments[1].href).toBeUndefined();
		expect(libraryCards({ movie: { titles: 1, parts: 0, items: 1, on_disk: 0, missing: 0, downloading: 1, size: 0 } }, '/downloads')[0].segments[1].href).toBe('/downloads');
	});
	it('leaves out kinds that are off and copes with an empty kind', () => {
		expect(libraryCards(undefined)).toEqual([]);
		const [m] = libraryCards({ movie: { titles: 0, parts: 0, items: 0, on_disk: 0, missing: 0, downloading: 0, size: 0 } });
		expect(m.segments.every((s) => s.share === 0)).toBe(true);
		expect(n(12345)).toBe('12,345');
	});
	it('re-reads Home on a new download or state, not on progress ticks', () => {
		const seen = new Map<number, string>();
		expect(downloadMoved(seen, { ID: 7, State: 'downloading', Progress: 0.1 })).toBe(true);
		expect(downloadMoved(seen, { ID: 7, State: 'downloading', Progress: 0.2 })).toBe(false);
		expect(downloadMoved(seen, { ID: 7, State: 'failed' })).toBe(true);
		expect(downloadMoved(seen, { id: 8, state: 'queued' })).toBe(true);
		expect(downloadMoved(seen, null)).toBe(true);
	});
	it('words stalled and paused asks, and only claims two weeks when the newest arrival is that new', () => {
		expect(askProgressLine({ state: 'stalled', progress: 0.42 })).toBe('Stalled at 42%');
		expect(askProgressLine({ state: 'paused', progress: 0.5 })).toBe('Paused at 50%');
		expect(homeStrip({ kind: 'show', name: 'S', line: 'Show' })).toBeNull();
		expect(homeStrip({ kind: 'artist', name: 'A', line: 'Music' })).toBeNull();
		const now = Date.parse('2026-11-03T12:00:00Z');
		expect(arrivedHead([{ at: '2026-11-01T12:00:00Z' }, { at: '2026-01-01T00:00:00Z' }], now)).toEqual({ title: 'New this week', subtitle: 'New in the last two weeks' });
		expect(arrivedHead([{ at: '2026-08-01T00:00:00Z' }], now)).toEqual({ title: 'Recently added', subtitle: undefined });
		expect(arrivedHead([{}], now)).toEqual({ title: 'Just arrived' });
	});
	it('names a failed download with no matched title by its own name', () => {
		const items = attention({ failed: [{ id: 1, title: '', name: 'Some.Release.2024', text: '' }, { id: 2, title: '', text: '' }], decisions: [], health: [] });
		expect(items.map((i) => i.title)).toEqual(['Some.Release.2024', 'A download']);
	});
	it('keeps outside torrents apart: no HolyArrs id, so they key by Downloader and hash', () => {
		const items = attention({
			failed: [
				{ id: 0, title: '', name: 'A.Torrent', text: '', client_item: true, downloader_id: 2, hash: 'aa' },
				{ id: 0, title: '', name: 'B.Torrent', text: '', client_item: true, downloader_id: 2, hash: 'bb' },
				{ id: 7, title: 'Dune', text: '' }
			],
			decisions: [],
			health: []
		});
		expect(new Set(items.map((i) => i.key)).size).toBe(3);
		expect(items.map((i) => i.href)).toEqual(['/downloads?show=problems.failed', '/downloads?show=problems.failed', '/downloads?show=problems.failed&open=7']);
		const group = groupAttention(items)[0];
		expect(new Set(group.items.map((i) => i.key)).size).toBe(3);
	});
});

describe('downloads that are being imported', () => {
	it('stay in the Downloading list although the server no longer calls them active', () => {
		const rows = [
			{ id: 1, phase: 'importing', state: 'finished', progress: 1, active: false, client_item: false },
			{ id: 2, phase: 'waiting_to_import', state: 'finished', progress: 1, active: false, client_item: false },
			{ id: 3, phase: 'done', state: 'done', progress: 1, active: false, client_item: false },
			{ id: 4, phase: 'downloading', state: 'downloading', progress: 0.4, active: true, client_item: false }
		];
		expect(downloadingNow(rows).map((d) => d.id).sort()).toEqual([1, 2, 4]);
	});
});

describe('torrents only the Downloader lists', () => {
	const row = (id: number, extra: object = {}) => ({ id, phase: 'downloading', state: 'downloading', progress: 0.5, active: true, client_item: true, live_key: `2:${id}`, ...extra });
	const rows = [
		row(1, { outside: true }),
		row(2, { outside: true, title_id: 40 }),
		row(3),
		row(4, { outside: true, phase: 'waiting', state: 'queued', progress: 0 }),
		row(5, { outside: true, phase: 'done', state: 'seeding', progress: 1 })
	];
	it('count when linked to a Library title or when HolyArrs sent them before its newest 100', () => {
		expect(downloadingNow(rows).map((d) => d.live_key).sort()).toEqual(['2:2', '2:3']);
	});
	it('that HolyArrs did not send are counted apart, only while they are fetching', () => {
		expect(notSentNow(rows).map((d) => d.live_key).sort()).toEqual(['2:1', '2:4']);
		expect(notSentNow([])).toEqual([]);
	});
	it('are all read, not only the first 200 Downloader rows', () => {
		const q = new URLSearchParams(homeDownloadsURL.split('?')[1]);
		expect(homeDownloadsURL.startsWith('/downloads/view?')).toBe(true);
		expect(q.get('limit')).toBe('100');
		expect(Number(q.get('outside_limit'))).toBe(2000);
	});
});

describe('System grid numbers', () => {
	it('counts parts that are on and working; only disabled is off', () => {
		expect(partCount([{ state: 'ok' }, { state: 'disabled' }, { state: 'unreachable' }])).toEqual({ total: 3, on: 2, working: 1, off: 1, failing: 1 });
		expect(partCount(null)).toEqual({ total: 0, on: 0, working: 0, off: 0, failing: 0 });
	});
	it('counts a failing Source as on but not working, the way the server reports it (paused, with a failing check)', () => {
		expect(partCount([{ state: 'enabled' }, { state: 'paused' }, { state: 'paused' }, { state: 'disabled' }], 2)).toEqual({ total: 4, on: 3, working: 1, off: 1, failing: 2 });
		// A failing check on a part whose state still says enabled also takes it out of working.
		expect(partCount([{ state: 'enabled' }, { state: 'enabled' }], 1)).toMatchObject({ on: 2, working: 1, failing: 1 });
	});
	it('matches failing checks to parts by name', () => {
		// A lingering check for a part that is now off changes nothing.
		expect(partCount([{ name: 'A', state: 'enabled' }, { name: 'B', state: 'disabled' }], ['B'])).toMatchObject({ on: 1, working: 1, failing: 0 });
		// A paused part with its check is counted once.
		expect(partCount([{ name: 'A', state: 'enabled' }, { name: 'B', state: 'paused' }], ['B'])).toMatchObject({ on: 2, working: 1, failing: 1 });
		// A check on a part whose state still says enabled takes it out of working.
		expect(partCount([{ name: 'A', state: 'enabled' }, { name: 'B', state: 'enabled' }], ['B'])).toMatchObject({ working: 1, failing: 1 });
	});
	it('leaves out a removed Downloader kept for its history', () => {
		expect(partCount([{ name: 'SABnzbd', state: 'ok' }, { name: 'qBittorrent (removed 3)', state: 'disabled' }])).toEqual({ total: 1, on: 1, working: 1, off: 0, failing: 0 });
	});
	it('counts folders on one disk once, even when their readings differ a little; several disks give the fullest', () => {
		expect(freeSpace([{ free_space: 100e9 }, { free_space: 100e9 + 4096 }, { free_space: 100e9 + 900e3 }])).toEqual({ free: 100e9, many: false });
		expect(freeSpace([{ free_space: 100e9 }, { free_space: 40e9 }])).toEqual({ free: 40e9, many: true });
		// The showcase's own readings: five folders on one disk, one read 1.9 GB apart.
		expect(freeSpace([78929354752, 78929354752, 78929289216, 78929231872, 77035409408].map((free_space) => ({ free_space })))).toEqual({ free: 77035409408, many: false });
		expect(freeSpace([])).toBeNull();
	});
	it('reads 0 as unknown, or as a full disk when the disk check is failing', () => {
		expect(freeSpace([{ free_space: 0 }, { free_space: 0 }])).toBeNull();
		expect(freeSpace([{ free_space: 0 }, { free_space: 7e12 }])).toEqual({ free: 7e12, many: false });
		expect(freeSpace([{ free_space: 0 }, { free_space: 7e12 }], true)).toEqual({ free: 0, many: true });
	});
});
