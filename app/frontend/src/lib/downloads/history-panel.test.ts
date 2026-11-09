import { panelView, pathBits, shortPath, whenText, type PanelContext } from './history-panel.ts';
import { groupDownloads, type HistoryRow } from './history.ts';
import { describe, expect, it } from 'vitest';
import type { EventRecord } from '../core/types.ts';

let id = 0;
const now = new Date(2026, 10, 5, 22, 36);
const at = (d: number, h: number, m = 0) => new Date(2026, 10, d, h, m).toISOString();
const ev = (Kind: string, extra: Partial<EventRecord> = {}): EventRecord => ({ ID: ++id, At: at(5, 22), Kind, Summary: Kind, TitleID: null, ...extra });
const one = (e: EventRecord): HistoryRow => ({ key: `e${e.ID}`, head: e, events: [e], steps: [] });
const ctx: PanelContext = { now, titles: new Map([[1, { kind: 'show', artwork: ['p.jpg'] }], [2, { kind: 'movie', artwork: [] }], [3, { kind: 'artist', artwork: ['a.jpg'] }]]), people: new Map([[2, 'home'], [3, 'Maya'], [5, 'agent-check']]) };
const facts = (v: ReturnType<typeof panelView>) => v.facts.map((f) => [f.label, f.value]);
const root = 'D:\\media';

describe('when', () => {
	it('says the day and clock, relative only on the same day', () => {
		expect(whenText(at(5, 22, 0), now)).toEqual({ text: 'Today, 10:00 PM', ago: '36 min ago' });
		expect(whenText(at(5, 22, 36), now).ago).toBe('just now');
		expect(whenText(at(5, 17, 51), now).ago).toBe('4 h ago');
		expect(whenText(at(4, 16, 50), now)).toEqual({ text: 'Yesterday, 4:50 PM', ago: '' });
		expect(whenText(new Date(2026, 9, 21, 15, 5).toISOString(), now).text).toBe('Oct 21, 3:05 PM');
		expect(whenText(new Date(2025, 9, 21, 15, 5).toISOString(), now).text).toBe('Oct 21, 2025, 3:05 PM');
	});
});

describe('paths', () => {
	it('shows a path from the Library folder down', () => {
		expect(shortPath(`${root}\\movies\\Heat (1995)\\Heat (1995) 1080p WEB.mkv`, 'Heat')).toBe('movies\\Heat (1995)\\Heat (1995) 1080p WEB.mkv');
		expect(shortPath(`${root}\\movies\\_loose\\Heat.1995.1080p.WEB.mkv`, 'Heat')).toBe('_loose\\Heat.1995.1080p.WEB.mkv');
		expect(shortPath(`${root}\\music`)).toBe('music');
		expect(shortPath('Ted Lasso (2020)/Season 4/x.mkv', 'Ted Lasso')).toBe('Ted Lasso (2020)/Season 4/x.mkv');
		expect(pathBits('movies\\Heat (1995)\\Heat.mkv')).toEqual(['movies\\', 'Heat (1995)\\', 'Heat.mkv']);
		expect(pathBits('a/bbbbbbbbbb/cccccccccc/d.mkv', 20)).toEqual(['…/cccccccccc/', 'd.mkv']);
		expect(pathBits('a/' + 'x'.repeat(30) + '.mkv', 20)).toEqual(['…/' + 'x'.repeat(30) + '.mkv']);
	});
});

describe('panelView', () => {
	it('an episode out now: Added, the episode, Aired, When alone', () => {
		const v = panelView(one(ev('gate_opened', { TitleID: 1, title_name: "Grey's Anatomy", episode: 'S23E04 Love on the Brain', Actor: { kind: 'system' } })), ctx);
		expect([v.verb.word, v.title, v.meta, v.line]).toEqual(['Added', "Grey's Anatomy", 'S23 E04 · Love on the Brain', 'Aired']);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM']]);
		expect(v.art).toEqual({ src: 'p.jpg', name: "Grey's Anatomy", square: false });
	});
	it('a Library folder out of reach: the folder as title, the count, the folder', () => {
		const v = panelView(one(ev('library_folder_unreachable', { Summary: 'Library folder Music: 1347 of 1347 files are gone at once; is a drive or share not mounted? No file was marked missing', Actor: { kind: 'job', id: 1 }, Data: { gone: 1347, known: 1347, path: `${root}\\music`, reason: 'most files gone at once' } })), ctx);
		expect([v.verb.word, v.title, v.meta, v.line, v.art]).toEqual(['Failed', 'Music folder', '', 'All 1,347 files gone', null]);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['Folder', 'music']]);
		expect(v.facts[1]).toMatchObject({ mono: true, full: `${root}\\music` });
	});
	it('a sign-in: the person as title, no By', () => {
		const v = panelView(one(ev('signed_in', { Summary: 'agent-check signed in', Actor: { kind: 'user', id: 5 }, actor_name: 'agent-check', Data: { name: 'agent-check' } })), ctx);
		expect([v.verb.word, v.title, v.line]).toEqual(['System', 'agent-check', 'Signed in']);
		expect(facts(v).map((f) => f[0])).toEqual(['When']);
		expect(v.actions).toEqual([]);
	});
	it('a download: the steps instead of a line, the download facts', () => {
		const [row] = groupDownloads([
			ev('download_sent', { TitleID: 1, title_name: 'Slow Horses', episode: 'S03E04 Uninvited Guests', DownloadID: 14, Actor: { kind: 'system' }, Data: { downloader: 'Stand-in', release: 'Slow.Horses.S03E04.1080p.WEB-DL', release_id: 14, save_path: `${root}\\downloads`, size_bytes: 8.1 * 1024 ** 3, source_name: 'Lab' } })
		]);
		const v = panelView(row, ctx);
		expect([v.verb.word, v.line]).toEqual(['Sent', '']);
		expect(v.steps.map((s) => s.state)).toEqual(['done', 'waiting', 'waiting']);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['Quality', '1080p'], ['Size', '8.1 GB'], ['Source', 'Lab'], ['Downloader', 'Stand-in'], ['Release', 'Slow.Horses.S03E04.1080p.WEB-DL']]);
		expect(v.find).toBe(true);
		expect(v.actions.map((a) => a.label)).toEqual(['Title page', 'Downloads']);
	});
	it('an import by hand: no meta for a movie, By, Quality and File', () => {
		const v = panelView(one(ev('import_done', { TitleID: 2, title_name: 'Heat', Actor: { kind: 'user', id: 2 }, actor_name: 'home', Data: { from: `${root}\\movies\\_loose\\Heat.1995.1080p.WEB.mkv`, paths: [{ from: `${root}\\movies\\_loose\\Heat.1995.1080p.WEB.mkv`, to: `${root}\\movies\\Heat (1995)\\Heat (1995) 1080p WEB.mkv` }] } })), ctx);
		expect([v.verb.word, v.meta, v.line]).toEqual(['Imported', '', '']);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['By', 'home'], ['Quality', '1080p'], ['File', 'movies\\Heat (1995)\\Heat (1995) 1080p WEB.mkv']]);
	});
	it('a failed import: the reason once, the release and where from', () => {
		const v = panelView(one(ev('import_failed', { TitleID: 2, title_name: 'Heat', Data: { paths: [{ from: `${root}\\movies\\_loose\\Heat.1995.1080p.WEB.mkv`, to: '' }], release: 'Heat.1995.1080p.WEB' } })), ctx);
		expect([v.verb.word, v.line]).toEqual(['Failed', 'Nothing could be placed']);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['Release', 'Heat.1995.1080p.WEB'], ['From', '_loose\\Heat.1995.1080p.WEB.mkv']]);
	});
	it('a decision: where it stopped, the reason without the import plan, an Import plan button', () => {
		const v = panelView(one(ev('needs_decision', { TitleID: 2, title_name: 'Arrival', SlotID: 9, Actor: { kind: 'system' }, Data: { reason: 'nothing could be placed; look at the import plan' } })), ctx);
		expect([v.verb.word, v.line]).toEqual(['Needs you', 'Nothing could be placed']);
		expect(v.steps.map((s) => [s.label, s.state])).toEqual([['Sent', 'done'], ['Downloaded', 'done'], ['Needs you', 'needs']]);
		expect(facts(v).map((f) => f[0])).toEqual(['When']);
		expect(v.actions.map((a) => a.label)).toEqual(['Import plan', 'Title page']);
	});
	it('a deletion: whether the files stayed, who did it', () => {
		const v = panelView(one(ev('title_removed', { TitleID: 2, title_name: 'Heat', Actor: { kind: 'user', id: 2 }, actor_name: 'home', Data: { delete_files: false } })), ctx);
		expect([v.verb.word, v.line]).toEqual(['Deleted', 'Files kept']);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['By', 'home']]);
	});
	it('a rename: the new name and the old one', () => {
		const v = panelView(one(ev('renamed', { TitleID: 2, title_name: 'Blade Runner', Actor: { kind: 'job', id: 1 }, Data: { from: 'Blade Runner (1982)/a 720p.mkv', to: 'Blade Runner (1982)/a.mkv' } })), ctx);
		expect([v.verb.word, v.line]).toEqual(['Renamed', '']);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['File', 'Blade Runner (1982)/a.mkv'], ['From', 'Blade Runner (1982)/a 720p.mkv']]);
	});
	it('an ask for a title not in the Library: no art, Artist, what was asked, By', () => {
		const v = panelView(one(ev('ask_made', { Summary: 'Maya asked for Tame Impala, every album', Actor: { kind: 'user', id: 3 }, actor_name: 'Maya', Data: { albums: 'every album', artist: 'mbid:x', ask_id: 11, asked_by: 'Maya' } })), ctx);
		expect([v.verb.word, v.title, v.meta, v.line, v.art]).toEqual(['Asked', 'Tame Impala', 'Artist', 'Every album', null]);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['By', 'Maya']]);
		expect(v.actions.map((a) => a.label)).toEqual(['Requests']);
	});
	it('an approval: By for a person, Automatic and no By for an automatic one', () => {
		const by = panelView(one(ev('ask_approved', { TitleID: 1, title_name: 'Ted Lasso', Actor: { kind: 'user', id: 2 }, actor_name: 'home', Data: { approved_by: 'home', auto: false, note: '' } })), ctx);
		expect([by.line, facts(by)]).toEqual(['', [['When', 'Today, 10:00 PM'], ['By', 'home']]]);
		const auto = panelView(one(ev('ask_approved', { TitleID: 1, title_name: 'Ted Lasso', Actor: { kind: 'system' }, Data: { auto: true } })), ctx);
		expect([auto.line, facts(auto).map((f) => f[0])]).toEqual(['Automatic', ['When']]);
	});
	it('a report: the kind and the words, By, Problems', () => {
		const v = panelView(one(ev('report_made', { TitleID: 3, title_name: 'Arctic Monkeys', Actor: { kind: 'user', id: 5 }, actor_name: 'agent-check', Data: { kind: 'sound', made_by: 'agent-check', note: 'This track skips for a second near the end.' } })), ctx);
		expect([v.verb.word, v.meta, v.line, v.long]).toEqual(['Reported', 'Artist', 'Sound · “This track skips for a second near the end.”', true]);
		expect(v.art?.square).toBe(true);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['By', 'agent-check']]);
		expect(v.actions.map((a) => a.label)).toEqual(['Title page', 'Problems']);
	});
	it('a missing file: File missing, the file', () => {
		const v = panelView(one(ev('file_missing', { TitleID: 1, title_name: "Grey's Anatomy", episode: 'S23E04 Love on the Brain', Actor: { kind: 'job', id: 1 }, Data: { path: "Grey's Anatomy/Season 23/x.mkv" } })), ctx);
		expect([v.verb.word, v.line]).toEqual(['Failed', 'x · Missing']);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['File', "Grey's Anatomy/Season 23/x.mkv"]]);
	});
	it('a fixed report says Fixed first, By is who fixed it; a reopened one says Opened again', () => {
		const v = panelView(one(ev('report_fixed', { TitleID: 2, title_name: 'The Matrix', Actor: { kind: 'user', id: 2 }, actor_name: 'home', Data: { kind: 'subtitles', reply: 'Swapped in a subtitle file that stays in sync.', made_by: 'agent-check', fixed_by: 'home' } })), ctx);
		expect([v.verb.word, v.line]).toEqual(['Reported', 'Fixed · Subtitles · “Swapped in a subtitle file that stays in sync.”']);
		expect(facts(v)).toEqual([['When', 'Today, 10:00 PM'], ['By', 'home']]);
		const r = panelView(one(ev('report_reopened', { TitleID: 2, title_name: 'The Matrix', Actor: { kind: 'user', id: 5 }, actor_name: 'agent-check', Data: { kind: 'subtitles', reopened_by: 'agent-check' } })), ctx);
		expect(r.line).toBe('Opened again · Subtitles');
	});
	it('a health problem: its subject as title, the rest as the line, no Why', () => {
		const v = panelView(one(ev('health_broken', { Summary: 'Downloader NZBGet (homelab test) does not answer: context deadline exceeded', Actor: { kind: 'system' }, Data: { kind: 'downloader', subject: 'NZBGet (homelab test)', state: 'error', message: 'Downloader NZBGet (homelab test) does not answer: context deadline exceeded' } })), ctx);
		expect([v.verb.word, v.title, v.line]).toEqual(['Failed', 'Downloader NZBGet (homelab test)', 'Does not answer: context deadline exceeded']);
		expect(facts(v).map((f) => f[0])).toEqual(['When']);
		const t = panelView(one(ev('health_broken', { Summary: 'Trailer lookups failing: update the reader', Actor: { kind: 'system' }, Data: { kind: 'trailers', subject: 'Trailer reader', state: 'error', message: 'Trailer lookups failing: update the reader' } })), ctx);
		expect([t.title, t.line, facts(t).map((f) => f[0])]).toEqual(['Trailer reader', 'Trailer lookups failing: update the reader', ['When']]);
		const yt = panelView(one(ev('health_broken', { Summary: 'YouTube asks to confirm', Actor: { kind: 'system' }, Data: { subject: 'Trailer reader, last good lookup 5 Nov 08:21 UTC', message: 'YouTube asks to confirm' } })), ctx);
		expect([yt.title, yt.line]).toEqual(['Trailer reader', 'YouTube asks to confirm']);
		const bare = panelView(one(ev('health_broken', { Summary: 'Something broke', Actor: { kind: 'system' }, Data: { message: 'Something broke' } })), ctx);
		expect([bare.title, bare.line, facts(bare).map((f) => f[0])]).toEqual(['Something broke', '', ['When']]);
	});
	it('set-aside files: a set-aside file says so, a clean-up names its subject', () => {
		const held = panelView(one(ev('file_held', { TitleID: 2, title_name: 'Heat', Actor: { kind: 'user', id: 2 }, actor_name: 'home', Summary: 'Moved Heat.mkv to the holding folder' })), ctx);
		expect([held.verb.word, held.line]).toEqual(['Deleted', 'File set aside']);
		const clean = panelView(one(ev('holding_cleaned', { Actor: { kind: 'job', id: 1 }, Summary: 'Emptied 2026-10-01 from the holding folder of Movies', Data: { day: '2026-10-01' } })), ctx);
		expect([clean.verb.word, clean.title, clean.line]).toEqual(['Deleted', 'Set-aside files', '']);
	});
	it('leaves the default Version out of the meta', () => {
		const v = panelView(one(ev('report_made', { TitleID: 2, title_name: 'Interstellar', version: 'Main', Actor: { kind: 'user', id: 5 }, actor_name: 'agent-check', Data: { kind: 'sound' } })), ctx);
		expect(v.meta).toBe('');
		const k = panelView(one(ev('report_made', { TitleID: 2, title_name: 'Interstellar', version: '4K', Actor: { kind: 'user', id: 5 }, actor_name: 'agent-check', Data: { kind: 'sound' } })), ctx);
		expect(k.meta).toBe('4K');
	});
	it('never names HolyArrs as a title or a person', () => {
		const v = panelView(one(ev('title_added', { TitleID: 2, title_name: 'Heat', Actor: { kind: 'system' }, Data: { why_added: 'by_hand' } })), ctx);
		expect(v.facts.some((f) => f.label === 'By')).toBe(false);
		const s = panelView(one(ev('health_broken', { Summary: 'YouTube asks to confirm', Actor: { kind: 'system' }, Data: { message: 'YouTube asks to confirm' } })), ctx);
		expect(s.title).not.toBe('HolyArrs');
	});
});
