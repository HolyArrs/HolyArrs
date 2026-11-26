import { describe, expect, it } from 'vitest';
import { chooseWarnings, facts, feedMinutesOk, keyReasons, languagesOf, listedName, pager, sortReleases, sourcesOf, verdict, verdictTone } from './releases.ts';
import type { ReleaseView } from '../core/types.ts';
import type { SearchView } from '../core/types.ts';

function rel(over: Partial<ReleaseView> = {}): ReleaseView {
	return {
		id: 1, name: 'Dune.2024.1080p.WEB-DL', size: 4_000_000_000, seeders: 50, protocol: 'torrent', state: 'judged', found_by: 'hand_search',
		source_id: 1, source_name: 'Lab', outcome: 'chosen', passed: true, rank: 1, points: 15, good_enough: true, needs_force: false,
		gate_skipped: false, reasons: [], created_at: '', ...over
	};
}

describe('verdicts', () => {
	it('names what happened', () => {
		expect(verdict(rel())).toBe('Would be chosen');
		expect(verdict(rel({ state: 'chosen' }))).toBe('Chosen');
		expect(verdict(rel({ outcome: 'held' }))).toBe('Would be held');
		expect(verdict(rel({ outcome: 'passed_over' }))).toBe('Passed over');
		expect(verdict(rel({ outcome: 'refused', state: 'refused' }))).toBe('Turned away by the rules');
		expect(verdict(rel({ outcome: 'refused', state: 'turned_down' }))).toBe('Turned down');
		expect(verdict(rel({ outcome: 'held', state: 'held' }))).toBe('Held');
		expect(verdict(rel({ outcome: '', state: 'seen' }))).toBe('Judged');
	});
	it('colours them', () => {
		expect(verdictTone(rel({ state: 'chosen' }))).toBe('red');
		expect(verdictTone(rel())).toBe('ok');
		expect(verdictTone(rel({ outcome: 'refused' }))).toBe('fail');
		expect(verdictTone(rel({ outcome: 'held' }))).toBe('wait');
		expect(verdictTone(rel({ outcome: 'passed_over' }))).toBe('idle');
	});
	it('keeps the reasons that decided it', () => {
		const rs = [
			{ code: 'step.allowed', effect: 'pass', text: 'ok' },
			{ code: 'step.not_allowed', effect: 'fail', text: 'cam is not allowed' },
			{ code: 'hard.gate_skipped', effect: 'warn', text: 'gate skipped' },
			{ code: 'points.trait', effect: 'add', text: 'HEVC (+10 points)' }
		];
		expect(keyReasons(rs).map((r) => r.code)).toEqual(['step.not_allowed', 'hard.gate_skipped']);
	});
	it('writes the facts line', () => {
		expect(facts(rel(), () => '3.7 GB')).toBe('Lab · 3.7 GB · 50 seeders · torrent · 15 points · ranked 1');
		expect(facts(rel({ passed: false, seeders: undefined }), () => '1 GB')).toBe('Lab · 1 GB · torrent');
	});
});

describe('choosing', () => {
	it('warns about force, the gate and a replaced copy', () => {
		expect(chooseWarnings(rel())).toEqual([]);
		const refused = rel({ needs_force: true, reasons: [{ code: 'step.not_allowed', effect: 'fail', text: 'cam is not allowed' }] });
		expect(chooseWarnings(refused)[0]).toContain('cam is not allowed');
		expect(chooseWarnings(rel({ gate_skipped: true }))[0]).toContain('Release gate is skipped');
		expect(chooseWarnings(rel(), 'not_out_yet')[0]).toContain('Release gate is skipped');
		expect(chooseWarnings(rel(), 'in_library')[0]).toContain('replace');
	});
	it('speaks of episodes for a show', () => {
		expect(chooseWarnings(rel(), 'not_out_yet', true)[0]).toBe('This episode has not aired yet; choosing it skips that check.');
		expect(chooseWarnings(rel(), 'in_library', true)[0]).toContain('This episode already has');
		expect(chooseWarnings(rel({ slot_ids: [1, 2, 3], episode: 'S01E01, S01E02, S01E03' }), 'wanted', true)).toEqual(['One download fills S01E01, S01E02, S01E03.']);
	});
	it('names what a listed Release is for', () => {
		expect(listedName({ title_name: 'Dune', state: 'held' })).toBe('Dune');
		expect(listedName({ title_name: 'Copper Valley', episode: 'S01E02', state: 'held' })).toBe('Copper Valley S01E02');
		expect(listedName({ title_name: 'Copper Valley', episode: 'S01 (10 episodes)', turned_down_for: 'S01E04', state: 'turned_down' })).toBe('Copper Valley S01E04');
	});
});

describe('pager', () => {
	it('works out pages', () => {
		expect(pager(0, 25, 0)).toEqual({ from: 0, to: 0, prev: null, next: null });
		expect(pager(60, 25, 0)).toEqual({ from: 1, to: 25, prev: null, next: 25 });
		expect(pager(60, 25, 25)).toEqual({ from: 26, to: 50, prev: 0, next: 50 });
		expect(pager(60, 25, 50)).toEqual({ from: 51, to: 60, prev: 25, next: null });
		expect(pager(60, 25, 10)).toEqual({ from: 11, to: 35, prev: 0, next: 35 });
	});
});

describe('search lines', () => {
	const s = (over: Partial<SearchView>): SearchView => ({
		job_id: 1, kind: 'search_slot', state: 'done', by_hand: false, only_show: false, title_name: 'Dune', version: 'Main', progress: 0, found: 3, refused: 1, ...over
	});
	it('checks the feed interval', () => {
		expect([0, 10, 15, 120].every(feedMinutesOk)).toBe(true);
		expect([5, 121, -1, 12.5].some(feedMinutesOk)).toBe(false);
	});
});

describe('albums', () => {
	it('names an album as Artist – Album', () => {
		expect(listedName({ title_name: 'Velvet Cartography', album: 'Low Tide Atlas', state: 'held' })).toBe('Velvet Cartography – Low Tide Atlas');
		const s: SearchView = {
			job_id: 1, kind: 'search_album', state: 'done', by_hand: true, only_show: false, title_name: 'Velvet Cartography', album: 'Low Tide Atlas',
			version: 'Main', progress: 1, found: 2, refused: 1, outcome: 'chosen', picked: 'VC - LTA [FLAC]'
		};
	});
	it('warns before choosing an album Release', () => {
		const w = chooseWarnings(rel({ gate_skipped: true, slot_ids: [1, 2, 3], album: 'Low Tide Atlas' }), undefined, true, true);
		expect(w).toEqual(['This album is not out yet; choosing it skips that check.', 'One download fills 3 tracks of Low Tide Atlas.']);
	});
});

describe('Choose a release filters and points sort (#539)', () => {
	const list = [
		rel({ id: 1, source_name: 'Usenet One', points: 10, rank: 1, read: { languages: ['fr'] } as ReleaseView['read'] }),
		rel({ id: 2, source_name: 'Torrent Hub', points: 40, rank: 2, read: { languages: ['fr', 'en'] } as ReleaseView['read'] }),
		rel({ id: 3, source_name: 'Torrent Hub', points: 25, passed: false, outcome: 'refused', rank: 0 }),
		rel({ id: 4, source_name: '', points: -5, rank: 3 })
	];
	it('sorts by points, refused ones included, highest first', () => {
		expect(sortReleases(list, 'points').map((r) => r.id)).toEqual([2, 3, 1, 4]);
	});
	it('keeps the judge order for best match', () => {
		expect(sortReleases(list, 'best').map((r) => r.id)).toEqual([1, 2, 4, 3]);
	});
	it('lists each source once, A to Z, skipping blanks', () => {
		expect(sourcesOf(list)).toEqual(['Torrent Hub', 'Usenet One']);
	});
	it('lists read languages, most common first', () => {
		expect(languagesOf(list)).toEqual(['fr', 'en']);
		expect(languagesOf([rel()])).toEqual([]);
	});
});
