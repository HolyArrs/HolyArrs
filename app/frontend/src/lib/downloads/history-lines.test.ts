import { dayWord, episodeBits, eventText, fakeEpisodeName, plainWhy, resolutionOf, rowText } from './history-lines.ts';
import { groupDownloads } from './history.ts';
import { describe, expect, it } from 'vitest';
import type { EventRecord } from '../core/types.ts';

let id = 0;
const now = new Date(2026, 9, 28, 12);
const today = new Date(2026, 9, 28, 1).toISOString();
const ev = (Kind: string, extra: Partial<EventRecord> = {}): EventRecord => ({ ID: ++id, At: today, Kind, Summary: Kind, TitleID: 1, title_name: 'Dune', ...extra });
const ctx = { now, titles: new Map([[1, { kind: 'movie' }], [2, { kind: 'show' }]]), people: new Map([[5, 'Maya']]), me: 1 };

describe('line 1', () => {
	it('hides made-up episode names', () => {
		expect(fakeEpisodeName('Episode 8827')).toBe(true);
		expect(fakeEpisodeName('27/10/2026')).toBe(true);
		expect(fakeEpisodeName('2026-10-27')).toBe(true);
		expect(fakeEpisodeName('Pastry Week')).toBe(false);
		expect(episodeBits('S39E192 Episode 8827')).toEqual(['S39 E192']);
		expect(episodeBits('S10E06 Pastry Week')).toEqual(['S10 E06', 'Pastry Week']);
		expect(episodeBits('S3E2')).toEqual(['S03 E02']);
	});
	it('names the kind when there is no episode', () => {
		expect(eventText(ev('title_added'), ctx).bits.map((b) => b.text)).toEqual(['Movie']);
		expect(eventText(ev('gate_opened', { TitleID: 2, title_name: 'Home and Away', episode: 'S39E192 Episode 8827' }), ctx).bits.map((b) => b.text)).toEqual(['S39 E192']);
	});
});

describe('line 2', () => {
	it('says when an episode aired, with no word on what comes next', () => {
		const t = eventText(ev('gate_opened', { TitleID: 2, title_name: 'Home and Away', episode: 'S39E192', Summary: 'Home and Away S39E192 has aired: it is wanted now' }), ctx);
		expect(t.tag.word).toBe('Aired');
		expect(t.line).toBe('Aired today');
		expect(dayWord(new Date(2026, 9, 27, 9).toISOString(), now)).toBe('yesterday');
		expect(dayWord(new Date(2026, 9, 20, 9).toISOString(), now)).toBe('on Oct 20');
	});
	it('says who added a title', () => {
		expect(eventText(ev('title_added', { Actor: { kind: 'user', id: 5 }, Data: { why_added: 'by_hand' } }), ctx).line).toBe('Added to the Library by Maya');
		expect(eventText(ev('title_added', { Actor: { kind: 'user', id: 1 }, Data: { why_added: 'by_hand' } }), ctx).line).toBe('Added to the Library by you');
	});
	it('checks, problems and plain reasons', () => {
		expect(eventText(ev('folder_scanned', { Data: { versions: [{ exists: true }] } }), ctx).line).toBe('Checked its folder · nothing changed');
		const f = eventText(ev('download_failed', { Data: { reason: 'the Downloader says error' } }), ctx);
		expect(f.tag.word).toBe('Failed');
		expect(f.line).toBe('Downloader error');
		expect(eventText(ev('download_failed'), ctx).line).toBe('Download failed');
		expect(eventText(ev('import_failed', { Data: { reason: 'no video file found' } }), ctx).line).toBe('No video file found');
		expect(eventText(ev('import_rejected'), ctx).line).toBe('Nothing could be placed');
		expect(plainWhy('no Release passed the Pick rules')).toBe('nothing good enough was found');
		expect(resolutionOf('Se7en.1995.1080p.BluRay')).toBe('1080p');
		expect(resolutionOf('Dune.2160p.WEB')).toBe('4K');
	});
	it('never says what comes next or repeats the step', () => {
		for (const [k, d] of [['title_added', { search_on_add: true }], ['search_done', {}], ['download_vanished', {}], ['file_missing', {}], ['release_chosen', { release: 'Dune.1080p' }], ['download_sent', { release: 'Dune.1080p' }], ['download_finished', { release: 'Dune.1080p' }]] as const)
			expect(eventText(ev(k, { Data: { ...d } }), ctx).line, k).not.toMatch(/will look|brings it|sending it|bringing it|in the Library$|^Sent to download|^Downloaded/);
		expect(eventText(ev('download_sent', { Data: { release: 'Dune.2021.1080p.WEB' } }), ctx).line).toBe('1080p');
		expect(eventText(ev('needs_decision', { Data: { reason: 'nothing could be placed; look at the import plan' } }), ctx).line).toBe('Nothing could be placed');
	});
	it('never repeats the title', () => {
		for (const k of ['title_added', 'gate_opened', 'title_refreshed', 'folder_scanned', 'ask_approved', 'album_changed', 'search_done', 'download_sent', 'import_done', 'title_removed', 'needs_decision', 'renamed', 'something_new'])
			expect(eventText(ev(k, { Summary: `Dune: ${k} for Dune`, Data: { reason: 'Dune is held' } }), ctx).line, k).not.toMatch(/Dune/);
	});
	it('a download that came in says its picture size', () => {
		const [row] = groupDownloads([
			ev('import_done', { DownloadID: 3, Data: { paths: [{ from: 'x/Dune.2021.1080p.mkv', to: 'y' }] } }),
			ev('download_finished', { DownloadID: 3, Data: { release: 'Dune.2021.1080p.WEB' } }),
			ev('download_sent', { DownloadID: 3, Data: { release: 'Dune.2021.1080p.WEB' } })
		]);
		const t = rowText(row, ctx);
		expect(t.tag.word).toBe('Came in');
		expect(t.line).toBe('1080p');
	});
});

describe('reported problems', () => {
	it('the tag says the step; the line keeps the kind and the words', () => {
		const made = eventText(ev('report_made', { Data: { kind: 'wont_play', note: 'stops at 20 minutes', made_by: 'kim' } }), ctx);
		expect(made.tag.word).toBe('Reported');
		expect(made.line).toBe("Won't play · stops at 20 minutes");
		const fixed = eventText(ev('report_fixed', { Data: { kind: 'sound', reply: 'swapped the copy', fixed_by: 'luis' } }), ctx);
		expect(fixed.tag.word).toBe('Fixed');
		expect(fixed.line).toBe('Sound · swapped the copy');
		const again = eventText(ev('report_reopened', { Data: { kind: 'picture', reopened_by: 'luis' } }), ctx);
		expect([again.tag.word, again.line]).toEqual(['Reopened', 'Picture']);
	});
});

describe('asks', () => {
	it('who and what, never "Asked for by" or "Approved by"', () => {
		expect(eventText(ev('ask_made', { Actor: { kind: 'user', id: 5 }, Data: { albums: 'every album', asked_by: 'Maya' } }), ctx).line).toBe('Maya · Every album');
		expect(eventText(ev('ask_made', { Actor: { kind: 'user', id: 9 }, actor_name: 'leo', Data: {} }), ctx).line).toBe('leo');
		expect(eventText(ev('ask_approved', { Actor: { kind: 'user', id: 2 }, Data: { approved_by: 'home' } }), ctx).line).toBe('home');
		expect(eventText(ev('ask_approved', { Actor: { kind: 'system' }, Data: { auto: true, approved_by: 'HolyArrs' } }), ctx).line).toBe('Automatic');
		const no = eventText(ev('ask_turned_down', { Actor: { kind: 'user', id: 2 }, Data: { turned_down_by: 'home', note: 'already have it' } }), ctx);
		expect([no.tag.word, no.line]).toEqual(['Turned down', 'home · Already have it']);
	});
});

describe('system rows', () => {
	it('a sign-in keeps the username as it is', () => {
		expect(eventText(ev('signed_in', { TitleID: null, title_name: undefined, Summary: 'agent-check signed in', actor_name: 'agent-check', Data: { name: 'agent-check' } }), ctx).line).toBe('agent-check signed in');
	});
	it('a Library folder out of reach: the folder and the count, no question', () => {
		const t = eventText(ev('library_folder_unreachable', { TitleID: null, title_name: undefined, Summary: 'Library folder Music: 1347 of 1347 files are gone at once; is a drive or share not mounted? No file was marked missing', Data: { gone: 1347, known: 1347 } }), ctx);
		expect([t.tag.word, t.line]).toEqual(['Failed', 'Music folder · All 1,347 files gone']);
		expect(eventText(ev('library_folder_unreachable', { TitleID: null, title_name: undefined, Summary: 'Library folder TV: 1200 of 1347 files are gone at once', Data: { gone: 1200, known: 1347 } }), ctx).line).toBe('TV folder · 1,200 of 1,347 files gone');
	});
	it('no tails about what may come next', () => {
		expect(eventText(ev('turned_down_removed'), ctx).line).toBe('Unblocked a download');
		expect(eventText(ev('rename_planned'), ctx).line).toBe('Rename ready');
		expect(eventText(ev('season_changed', { Data: { season: 2, keep_up_to_date: true } }), ctx).line).toBe('Season 2 · Kept up to date');
		expect(eventText(ev('season_changed', { Data: { season: 2, keep_up_to_date: false } }), ctx).line).toBe('Season 2 · No longer kept up to date');
		expect(eventText(ev('download_removed'), ctx).line).toBe('Removed from downloader');
	});
	it('a failed download row says the reason alone', () => {
		const [row] = groupDownloads([
			ev('import_failed', { DownloadID: 4, Data: { reason: 'nothing could be placed' } }),
			ev('download_finished', { DownloadID: 4, Data: { release: 'Dune.2021.1080p.WEB' } }),
			ev('download_sent', { DownloadID: 4, Data: { release: 'Dune.2021.1080p.WEB' } })
		]);
		const t = rowText(row, ctx);
		expect([t.tag.word, t.line]).toEqual(['Failed', 'Nothing could be placed']);
	});
});
