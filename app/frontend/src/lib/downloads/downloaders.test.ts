import { decisionName, groupDecisions, manyWords, phaseTone, removeBody, removeFilesWords, removeStart, rowName, split, whyWords } from './downloaders.ts';
import { samePathPair, seedingBody, seedingFields, deadRuleOn, downloadTone, emptyForm, formBody, formFrom, healthTone, pendingAttempts, percent, timeLeft, type DownloaderAttempt, type DownloaderView } from './downloaders.ts';
import { describe, expect, it } from 'vitest';
import { type Decision, type DownloadRow } from '../core/types.ts';

const saved: DownloaderView = {
	id: 2,
	name: 'Lab SABnzbd',
	kind: 'sabnzbd',
	protocol: 'usenet',
	url: 'http://10.0.0.5:8080',
	username: '',
	secret: '••••••••',
	category: 'movies',
	priority: 2,
	path_map: [{ theirs: '/data/downloads', ours: '/downloads' }],
	allow_local_network: true,
	writable: true,
	writable_locked: false,
	health: { state: 'working', label: 'Working', text: '' },
	active_downloads: 1
};

describe('downloaders', () => {
	it('colours health and download states', () => {
		expect(healthTone('working')).toBe('ok');
		expect(healthTone('paused')).toBe('idle');
		expect(healthTone('resting')).toBe('wait');
		expect(healthTone('sign_in_refused')).toBe('fail');
		expect(healthTone('not_answering')).toBe('fail');
		expect(downloadTone('downloading')).toBe('wait');
		expect(downloadTone('stalled')).toBe('fail');
		expect(downloadTone('vanished')).toBe('fail');
		expect(downloadTone('finished')).toBe('ok');
	});

	it('says progress and time left', () => {
		expect(percent(0.426)).toBe('42%');
		expect(percent(2)).toBe('100%');
		expect(timeLeft(0)).toBe('');
		expect(timeLeft(30)).toBe('under a minute left');
		expect(timeLeft(600)).toBe('10 min left');
		expect(timeLeft(7200)).toBe('2 h left');
		expect(timeLeft(5400)).toBe('1 h 30 min left');
	});

	it('starts a new Downloader on the local network', () => {
		expect(emptyForm().allow_local_network).toBe(true);
		expect(emptyForm().writable).toBe(true);
		expect(emptyForm('sabnzbd').category).toBe('movies');
		expect(emptyForm('qbittorrent').category).toBe('holyarrs');
	});

	it('preserves optional Downloader sign-in names and labels', () => {
		for (const [kind, name] of [['transmission', 'Transmission'], ['nzbget', 'NZBGet'], ['deluge', 'Deluge'], ['rtorrent', 'rTorrent'], ['aria2', 'aria2']] as const) {
			const form = emptyForm(kind);
			expect(form.name).toBe(name);
			expect(form.category).toBe('holyarrs');
			form.username = 'task-user';
			form.secret = '••••••••';
			const body = formBody(form);
			expect(body.username).toBe(kind === 'deluge' || kind === 'aria2' ? '' : 'task-user');
			expect(body.secret).toBe('••••••••');
		}
	});

	it('shows only uncertain sends that still need review', () => {
		const base = { downloader_id: 4, release_id: 9, slot_id: 12, created_at: '2026-10-15T20:00:00Z', updated_at: '2026-10-15T20:00:00Z' };
		const attempts: DownloaderAttempt[] = [
			{ ...base, id: 1, state: 'resolved' },
			{ ...base, id: 2, state: 'pending' },
			{ ...base, id: 3, state: 'abandoned' }
		];
		expect(pendingAttempts(attempts).map((attempt) => attempt.id)).toEqual([2]);
		expect(pendingAttempts(undefined)).toEqual([]);
	});

	it('round trips a saved Downloader and keeps the masked key', () => {
		const body = formBody(formFrom(saved));
		expect(body).toEqual({
			name: 'Lab SABnzbd',
			kind: 'sabnzbd',
			url: 'http://10.0.0.5:8080',
			username: '',
			secret: '••••••••',
			category: 'movies',
			category_movies: '',
			category_shows: '',
			category_music: '',
			priority: 2,
			path_map: [{ theirs: '/data/downloads', ours: '/downloads' }],
			allow_local_network: true,
			writable: true,
			tags: []
		});
	});

	it('drops empty path rows and the sign-in name for SABnzbd', () => {
		const f = formFrom(saved);
		f.path_map.push({ theirs: ' ', ours: '' });
		f.username = 'someone';
		const body = formBody(f);
		expect(body.path_map).toHaveLength(1);
		expect(body.username).toBe('');
	});

	it('sends qBittorrent with a user name and password, or a key with no name', () => {
		const f = emptyForm('qbittorrent');
		f.username = ' admin ';
		f.secret = 'your-typed-word';
		expect(formBody(f)).toMatchObject({ username: 'admin', secret: 'your-typed-word' });
		f.username = '';
		f.secret = 'your-typed-key';
		expect(formBody(f)).toMatchObject({ username: '', secret: 'your-typed-key' });
	});
});

describe('the Downloads screen', () => {
	const row = (phase: string): DownloadRow =>
		({ id: 1, state: 'x', label: '', text: '', progress: 0, speed: 0, active: false, phase, name: 'n', title: 't', version: 'HD', downloader: 'Q', size: 1, updated_at: '', can_act: false });
	it('colours places', () => {
		expect(phaseTone('done')).toBe('ok');
		expect(phaseTone('vanished')).toBe('fail');
		expect(phaseTone('needs_a_decision')).toBe('fail');
		expect(phaseTone('removed')).toBe('idle');
		expect(phaseTone('importing')).toBe('wait');
	});
	it('splits moving rows from finished ones', () => {
		const s = split(['downloading', 'done', 'waiting_to_import', 'failed', 'stalled'].map(row));
		expect(s.now.map((r) => r.phase)).toEqual(['downloading', 'waiting_to_import', 'stalled']);
		expect(s.past.map((r) => r.phase)).toEqual(['done', 'failed']);
	});
	it('starts the Remove dialog from the settings and sends search again only with Turned down', () => {
		expect(removeStart(null)).toEqual({ delete_files: true, turn_down: false, search_again: true });
		expect(removeStart({ remove_delete_files: false, remove_turn_down: true, remove_search_again: false })).toEqual({ delete_files: false, turn_down: true, search_again: false });
		expect(removeBody({ delete_files: true, turn_down: false, search_again: true })).toEqual({ delete_files: true, turn_down: false, search_again: false });
		expect(removeBody({ delete_files: false, turn_down: true, search_again: true })).toEqual({ delete_files: false, turn_down: true, search_again: true });
	});
	it('says why a movie needs a decision', () => {
		expect(whyWords({ why: 'ran_out', has_copy: false })).toContain('ran out of Releases');
		expect(whyWords({ why: 'conflict', has_copy: false })).toContain('import');
		expect(whyWords({ why: 'not_replaced', has_copy: false })).toContain('removed outside HolyArrs');
	});
});

describe('shows on the Downloads screen and Needs a decision', () => {
	const ep = (n: number, state = 'needs_a_decision') => ({ slot_id: n, season: 1, episode: n, label: `S01E0${n}`, state });
	const dec = (slot: number, extra: Partial<Decision> = {}): Decision => ({
		slot_id: slot,
		title_id: 1,
		title: 'Copper Valley',
		version: 'Main',
		versions: 1,
		why: 'conflict',
		reason: '',
		has_copy: false,
		since: '2026-10-13T10:00:00Z',
		kind: 'show',
		episode: ep(slot),
		...extra
	});
	it('names movies with the year and shows without', () => {
		expect(rowName({ title: 'Dune', year: 2024, name: 'x', kind: 'movie' })).toBe('Dune (2024)');
		expect(rowName({ title: 'Copper Valley', year: 2025, name: 'x', kind: 'show' })).toBe('Copper Valley');
		expect(rowName({ title: '', name: 'Some.Release', kind: 'show' })).toBe('Some.Release');
	});
	it('names a stuck episode', () => {
		expect(decisionName(dec(2, { episode: { ...ep(2), name: 'Flood' } }))).toBe('Copper Valley S01E02 Flood');
		expect(decisionName({ title: 'Dune', year: 2024 })).toBe('Dune (2024)');
	});
	it('puts the stuck episodes of one pack on one card', () => {
		const groups = groupDecisions([
			dec(3, { download_id: 9, pack_slot_ids: [1, 3] }),
			dec(5),
			dec(1, { download_id: 9, pack_slot_ids: [1, 3] }),
			{ ...dec(7), kind: 'movie', episode: undefined, title: 'Dune' }
		]);
		expect(groups.map((g) => g.key)).toEqual(['pack-9', 'slot-5', 'slot-7']);
		expect(groups[0].pack).toBe(true);
		expect(groups[0].items.map((d) => d.slot_id)).toEqual([1, 3]);
		expect(groups[1].pack).toBe(false);
	});
	it('keeps a pack with one stuck episode as a single card', () => {
		const groups = groupDecisions([dec(2, { download_id: 4, pack_slot_ids: [2] })]);
		expect(groups).toHaveLength(1);
		expect(groups[0].pack).toBe(false);
	});
});

describe('albums on the Downloads screen and Needs a decision', () => {
	const tr = (n: number, state = 'needs_a_decision') => ({ slot_id: n, season: 1, episode: n, label: `Track 0${n}`, state, track: true });
	const dec = (slot: number, extra: Partial<Decision> = {}): Decision => ({
		slot_id: slot,
		title_id: 1,
		title: 'Lab Band',
		album: 'First Light',
		version: 'Main',
		versions: 1,
		why: 'conflict',
		reason: '',
		has_copy: false,
		since: '2026-10-13T10:00:00Z',
		kind: 'artist',
		episode: tr(slot),
		...extra
	});
	it('names an album row Artist – Album (Year)', () => {
		expect(rowName({ title: 'Lab Band', album: 'First Light', year: 2020, name: 'x', kind: 'artist' })).toBe('Lab Band – First Light (2020)');
		expect(rowName({ title: 'Lab Band', name: 'x', kind: 'artist' })).toBe('Lab Band');
	});
	it('has plain words for tracks', () => {
		expect(manyWords('artist')).toEqual({ one: 'track', many: 'tracks', pack: 'album' });
		expect(manyWords('show')?.many).toBe('episodes');
		expect(manyWords('movie')).toBeNull();
	});
	it('names a stuck track with its album', () => {
		expect(decisionName(dec(2, { episode: { ...tr(2), name: 'Glass River' } }))).toBe('Lab Band – First Light Track 02 Glass River');
	});
	it('puts the stuck tracks of one album download on one card', () => {
		const groups = groupDecisions([dec(3, { download_id: 9, pack_slot_ids: [1, 3] }), dec(1, { download_id: 9, pack_slot_ids: [1, 3] })]);
		expect(groups).toHaveLength(1);
		expect(groups[0].pack).toBe(true);
		expect(groups[0].items.map((d) => d.slot_id)).toEqual([1, 3]);
	});
});

describe('removeFilesWords', () => {
	it('names the folder of one row, or its files in the client', () => {
		expect(removeFilesWords([{ path: '/downloads/Film.2020', downloader: 'qBittorrent' }])).toBe('Deletes /downloads/Film.2020 in qBittorrent.');
		expect(removeFilesWords([{ downloader: 'SABnzbd' }])).toBe('Deletes its files in SABnzbd.');
		expect(removeFilesWords([{ downloader: '' }])).toBe('Deletes its files in the Downloader.');
	});
	it('lists up to three folders for many rows', () => {
		const rows = ['/a', '/b', '/c', '/d'].map((path) => ({ path, downloader: 'qBittorrent' }));
		expect(removeFilesWords(rows)).toBe('Deletes /a, /b, /c and the files of 1 more in qBittorrent.');
		expect(removeFilesWords([{ downloader: 'A' }, { downloader: 'B' }])).toBe('Deletes their files in A, B.');
		expect(removeFilesWords([])).toBe('');
	});
});

describe('after seeding', () => {
	it('starts off and sends no goals', () => {
		expect(formBody(emptyForm('qbittorrent')).after_seeding).toEqual({ remove: false, ratio: null, minutes: null, delete_files: false });
	});
	it('turns hours into minutes and keeps goals for qBittorrent and rTorrent only', () => {
		const f = { ...emptyForm('rtorrent'), seed_remove: true, seed_ratio: 2 as const, seed_hours: 1.5, seed_delete_files: true };
		expect(seedingBody(f)).toEqual({ remove: true, ratio: 2, minutes: 90, delete_files: true });
		expect(seedingBody({ ...f, kind: 'transmission' })).toEqual({ remove: true, ratio: null, minutes: null, delete_files: true });
	});
	it('never deletes files when remove is off, and Usenet sends nothing', () => {
		expect(seedingBody({ ...emptyForm(), seed_delete_files: true }).delete_files).toBe(false);
		expect(formBody(emptyForm('sabnzbd'))).not.toHaveProperty('after_seeding');
	});
});

describe('after seeding fields', () => {
	it('shows the group on torrent Downloaders only, goals on qBittorrent and rTorrent', () => {
		expect(seedingFields('qbittorrent')).toBe('goals');
		expect(seedingFields('rtorrent')).toBe('goals');
		expect(seedingFields('transmission')).toBe('remove');
		expect(seedingFields('aria2')).toBe('remove');
		expect(seedingFields('sabnzbd')).toBe('none');
		expect(seedingFields('nzbget')).toBe('none');
	});
});

describe('dead torrent rule', () => {
	it('is on when either check has a number, off for zeros or missing', () => {
		expect(deadRuleOn({ dead_metadata_minutes: 30, crawl_kbps: 0 })).toBe(true);
		expect(deadRuleOn({ crawl_kbps: 10 })).toBe(true);
		expect(deadRuleOn({ dead_metadata_minutes: 0, crawl_kbps: 0 })).toBe(false);
		expect(deadRuleOn({})).toBe(false);
	});
});

describe('category after import and own items only', () => {
	it('sends the category after import for qBittorrent only, and own_only for torrent clients', () => {
		const q = { ...emptyForm('qbittorrent'), category_after_import: ' holyarrs-done ', own_only: true };
		expect(formBody(q)).toMatchObject({ category_after_import: 'holyarrs-done', own_only: true });
		const tr = { ...emptyForm('transmission'), category_after_import: 'x', own_only: true };
		expect(formBody(tr)).not.toHaveProperty('category_after_import');
		expect(formBody(tr)).toMatchObject({ own_only: true });
		expect(formBody({ ...emptyForm('sabnzbd'), own_only: true })).not.toHaveProperty('own_only');
		expect(emptyForm()).toMatchObject({ category_after_import: '', own_only: false });
	});
});

describe('same path pair', () => {
	it('spots a pair with the same folder on both sides', () => {
		expect(samePathPair({ theirs: '/mnt/Torrents', ours: '/mnt/Torrents' })).toBe(true);
		expect(samePathPair({ theirs: ' /mnt/Torrents/ ', ours: '/mnt/Torrents' })).toBe(true);
		expect(samePathPair({ theirs: 'D:\\Torrents\\', ours: 'd:/torrents' })).toBe(true);
		expect(samePathPair({ theirs: '/mnt/torrents', ours: '/mnt/Torrents' })).toBe(false);
		expect(samePathPair({ theirs: '/data/downloads', ours: '/downloads' })).toBe(false);
		expect(samePathPair({ theirs: '', ours: '' })).toBe(false);
	});
});
