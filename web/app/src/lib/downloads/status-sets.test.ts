import { describe, expect, it } from 'vitest';
import { ALL, ALL_HINT, SETS, SET_IDS, chipsFor, counts, pick, seedTag, setOf, showOf, stageOf, statusWord, subCounts, type SetId } from './status-sets.ts';

type R = { id: number; phase: string; state: string; progress: number; label: string };
const r = (id: number, phase: string, state = phase, progress = 0): R => ({ id, phase, state, progress, label: phase });

// One row of every phase the app knows, plus a Usenet job being unpacked
// (downloading at full progress) and a seeding torrent.
const rows: R[] = [
	r(1, 'waiting', 'queued'),
	r(2, 'downloading', 'downloading', 0.4),
	r(3, 'downloading', 'downloading', 1),
	r(4, 'stalled'),
	r(5, 'paused'),
	r(6, 'waiting_to_import', 'finished', 1),
	r(7, 'importing', 'finished', 1),
	r(8, 'done', 'finished', 1),
	r(9, 'done', 'seeding', 1),
	r(10, 'failed'),
	r(11, 'removed', 'cleaned', 1),
	r(12, 'vanished'),
	r(13, 'needs_a_decision', 'finished', 1),
	r(14, 'import_refused', 'finished', 1)
];
const ids = (xs: R[]) => xs.map((x) => x.id);

describe('stageOf', () => {
	it('puts every phase on one stage, the same for a torrent and a Usenet download', () => {
		expect(rows.map((x) => stageOf(x))).toEqual([
			'queued',
			'downloading',
			'importing',
			'stalled',
			'paused',
			'importing',
			'importing',
			'done',
			'seeding',
			'failed',
			'done',
			'failed',
			'failed',
			'failed'
		]);
	});
});

describe('the five sets', () => {
	it('put All first, and the other chips split the rows with none left over or counted twice (sets 1, 2, 3, 5, 6, 7)', () => {
		for (const id of [1, 2, 3, 5, 6, 7] as SetId[]) {
			const chips = chipsFor(id, rows, true);
			const c = counts(rows, chips);
			expect(c[0]).toMatchObject({ key: ALL, label: 'All', count: rows.length });
			const seen = chips.flatMap((ch) => ids(pick(rows, chips, ch.key)));
			expect(seen.sort((a, b) => a - b)).toEqual(ids(rows));
			expect(c.slice(1).reduce((n, x) => n + x.count, 0)).toBe(rows.length);
		}
	});
	it('set 4: the top level splits the rows, and each chip\'s sub-chips split that chip', () => {
		const chips = chipsFor(4, rows, true);
		expect(counts(rows, chips).map((c) => `${c.label} ${c.count}`)).toEqual(['All 14', 'In progress 5', 'Finished 3', 'Needs you 6']);
		for (const ch of chips) {
			const inChip = pick(rows, chips, ch.key);
			const subs = subCounts(rows, ch);
			expect(subs.reduce((n, s) => n + s.count, 0)).toBe(inChip.length);
			const seen = (ch.subs ?? []).flatMap((s) => ids(pick(rows, chips, ch.key, s.key)));
			expect(seen.sort((a, b) => a - b)).toEqual(ids(inChip));
		}
		expect(subCounts(rows, chips[0]).map((s) => `${s.label} ${s.count}`)).toEqual(['Waiting 1', 'Downloading 1', 'Importing 3']);
		expect(subCounts(rows, chips[1]).map((s) => `${s.label} ${s.count}`)).toEqual(['Done 2', 'Seeding 1']);
		expect(subCounts(rows, chips[2]).map((s) => `${s.label} ${s.count}`)).toEqual(['Paused 1', 'Stalled 1', 'Failed 4']);
	});
	it('set 1 counts', () => {
		expect(counts(rows, chipsFor(1, rows, true)).map((c) => `${c.label} ${c.count}`)).toEqual(['All 14', 'Waiting 1', 'Downloading 1', 'Importing 3', 'Done 3', 'Problems 6']);
	});
	it('set 2 puts waiting under Stopped', () => {
		expect(ids(pick(rows, chipsFor(2, rows, true), 'stopped'))).toEqual([1, 4, 5, 10, 12, 13, 14]);
	});
	it('set 3: Active is downloading, importing and seeding; Done is finished and no longer seeding', () => {
		const chips = chipsFor(3, rows, true);
		expect(ids(pick(rows, chips, 'active'))).toEqual([2, 3, 6, 7, 9]);
		expect(ids(pick(rows, chips, 'done'))).toEqual([8, 11]);
	});
	it('the Done chip carries the seeding sub-count where seeding counts as Done', () => {
		expect(counts(rows, chipsFor(1, rows, true)).find((c) => c.key === 'done')).toMatchObject({ count: 3, seeding: 1 });
		expect(counts(rows, chipsFor(2, rows, true)).find((c) => c.key === 'done')).toMatchObject({ count: 3, seeding: 1 });
		expect(counts(rows, chipsFor(4, rows, true)).find((c) => c.key === 'finished')).toMatchObject({ count: 3, seeding: 1 });
		expect(counts(rows, chipsFor(3, rows, true)).find((c) => c.key === 'done')?.seeding).toBeUndefined();
	});
	it('set 5 shows Seeding only with a torrent Downloader or a seeding row; without it Done takes seeding rows', () => {
		const calm = rows.filter((x) => x.state !== 'seeding');
		expect(chipsFor(5, calm, false).map((c) => c.key)).not.toContain('seeding');
		expect(chipsFor(5, calm, true).map((c) => c.key)).toContain('seeding');
		expect(chipsFor(5, rows, false).map((c) => c.key)).toContain('seeding');
		expect(chipsFor(5, rows, true).map((c) => c.label)).toEqual(['Queued', 'Downloading', 'Importing', 'Seeding', 'Done', 'Paused', 'Failed']);
		const noSeed = chipsFor(5, calm, false);
		expect(noSeed.find((c) => c.key === 'done')?.stages).toEqual(['done', 'seeding']);
	});
	it('every set has a VariantBar label', () => {
		expect(SET_IDS.map((id) => SETS[id].label)).toEqual(['1 Path', '2 Moving', '3 Active', '4 Two-level', '5 Downloader words', '6 Six tabs', '7 Five tabs']);
	});
});

describe('statusWord and seedTag', () => {
	it('use the chip words of the picked set', () => {
		expect(statusWord(rows[0], 1)).toBe('Waiting');
		expect(statusWord(rows[0], 5)).toBe('Queued');
		expect(statusWord(rows[2], 1)).toBe('Importing');
		expect(statusWord(rows[5], 2)).toBe('Importing');
		expect(statusWord(rows[4], 1)).toBe('Paused');
		expect(statusWord(rows[10], 1)).toBe('Removed');
		expect(statusWord(rows[11], 1)).toBe('Vanished');
		expect(statusWord(rows[12], 4)).toBe('Needs a decision');
	});
	it('a seeding row reads Done with a Seeding tag in sets 1, 2 and 4, and Seeding in 3 and 5', () => {
		const s = rows[8];
		expect([1, 2, 4].map((id) => statusWord(s, id as SetId))).toEqual(['Done', 'Done', 'Done']);
		expect([1, 2, 4].map((id) => seedTag(s, id as SetId))).toEqual([true, true, true]);
		expect([3, 5].map((id) => statusWord(s, id as SetId))).toEqual(['Seeding', 'Seeding']);
		expect([3, 5].map((id) => seedTag(s, id as SetId))).toEqual([false, false]);
		expect(seedTag(rows[7], 1)).toBe(false);
	});
});

describe('setOf and showOf', () => {
	it('read the address safely', () => {
		expect(setOf('3')).toBe(3);
		expect(setOf('now')).toBe(6);
		expect(setOf(null)).toBe(6);
		expect(setOf(9)).toBe(6);
		expect(setOf('7')).toBe(7);
		expect(showOf('done', SETS[1].chips)).toEqual({ show: 'done', sub: '' });
		expect(showOf('nope', SETS[1].chips)).toEqual({ show: ALL, sub: '' });
		expect(showOf('progress.importing', SETS[4].chips)).toEqual({ show: 'progress', sub: 'importing' });
		expect(showOf('progress.nope', SETS[4].chips)).toEqual({ show: 'progress', sub: '' });
		expect(showOf(null, SETS[2].chips)).toEqual({ show: ALL, sub: '' });
	});
});

describe('sets 6 and 7', () => {
	it('set 6: Waiting is queued and paused, Downloading keeps stalled ones, Problems is what failed', () => {
		const chips = chipsFor(6, rows, true);
		expect(counts(rows, chips).map((c) => `${c.label} ${c.count}`)).toEqual(['All 14', 'Waiting 2', 'Downloading 2', 'Importing 3', 'Done 3', 'Problems 4']);
		expect(ids(pick(rows, chips, 'waiting'))).toEqual([1, 5]);
		expect(ids(pick(rows, chips, 'downloading'))).toEqual([2, 4]);
		expect(ids(pick(rows, chips, 'problems'))).toEqual([10, 12, 13, 14]);
		expect(counts(rows, chips).find((c) => c.key === 'done')).toMatchObject({ count: 3, seeding: 1 });
		expect(statusWord(rows[3], 6)).toBe('Stalled');
		expect(seedTag(rows[8], 6)).toBe(true);
	});
	it('set 7: Working is downloading and importing; the status says which', () => {
		const chips = chipsFor(7, rows, true);
		expect(counts(rows, chips).map((c) => `${c.label} ${c.count}`)).toEqual(['All 14', 'Waiting 2', 'Working 5', 'Done 3', 'Problems 4']);
		expect(ids(pick(rows, chips, 'working'))).toEqual([2, 3, 4, 6, 7]);
		expect(['Downloading', 'Importing', 'Stalled']).toContain(statusWord(rows[1], 7));
		expect(statusWord(rows[2], 7)).toBe('Importing');
	});
	it('every chip and sub-chip in every set has a short hint', () => {
		expect(ALL_HINT.length).toBeGreaterThan(0);
		for (const id of SET_IDS) {
			for (const c of counts(rows, chipsFor(id, rows, true)).concat(SETS[id].chips.flatMap((c) => subCounts(rows, c)))) {
				expect(c.hint, `${id} ${c.key}`).toMatch(/^[A-Z].{8,90}\.$/);
			}
		}
	});
});
