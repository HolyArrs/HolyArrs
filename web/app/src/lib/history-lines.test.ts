import { describe, expect, it } from 'vitest';
import { dayWord, episodeBits, eventText, fakeEpisodeName, plainWhy, resolutionOf, rowText } from './history-lines.ts';
import { groupDownloads } from './history.ts';
import type { EventRecord } from './types.ts';

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
	it('says when an episode aired and what comes next', () => {
		const t = eventText(ev('gate_opened', { TitleID: 2, title_name: 'Home and Away', episode: 'S39E192', Summary: 'Home and Away S39E192 has aired: it is wanted now' }), ctx);
		expect(t.tag.word).toBe('Aired');
		expect(t.line).toBe('Aired today · HolyArrs will look for it');
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
		expect(f.line).toBe('Download failed · the downloader reported an error');
		expect(plainWhy('no Release passed the Pick rules')).toBe('nothing good enough was found');
		expect(resolutionOf('Se7en.1995.1080p.BluRay')).toBe('1080p');
		expect(resolutionOf('Dune.2160p.WEB')).toBe('4K');
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
		expect(t.line).toBe('Downloaded · 1080p · in the Library');
	});
});

describe('reported problems', () => {
	it('says who reported, fixed or reopened it, with the kind and the words', () => {
		const made = eventText(ev('report_made', { Data: { kind: 'wont_play', note: 'stops at 20 minutes', made_by: 'kim' } }), ctx);
		expect(made.tag.word).toBe('Reported');
		expect(made.line).toBe("Problem reported by kim · Won't play · stops at 20 minutes");
		const fixed = eventText(ev('report_fixed', { Data: { kind: 'sound', reply: 'swapped the copy', fixed_by: 'luis' } }), ctx);
		expect(fixed.tag.word).toBe('Fixed');
		expect(fixed.line).toBe('Problem fixed by luis · Sound · swapped the copy');
		expect(eventText(ev('report_reopened', { Data: { kind: 'picture', reopened_by: 'luis' } }), ctx).line).toBe('Problem opened again by luis · Picture');
	});
});
