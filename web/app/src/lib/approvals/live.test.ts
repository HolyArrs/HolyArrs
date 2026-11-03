import { describe, expect, it } from 'vitest';
import { liveByTitle, liveFacts, partialFacts, realAlbum } from './live.ts';

const dl = (title_id: number, phase: string, progress = 0) => ({ title_id, phase, progress });
const se = (title_id: number, state = 'running', kind = 'search_slot') => ({ title_id, state, kind }) as Parameters<typeof liveByTitle>[1][number];

describe('liveByTitle', () => {
	it('maps phases to downloading, importing and failed', () => {
		const m = liveByTitle([dl(1, 'downloading', 0.4), dl(2, 'importing', 1), dl(3, 'failed'), dl(4, 'done', 1), dl(5, 'waiting')], []);
		expect(liveFacts(m.get(1))).toEqual({ phase: 'downloading', progress: 0.4 });
		expect(liveFacts(m.get(2))).toEqual({ phase: 'importing', progress: 1 });
		expect(liveFacts(m.get(3))).toEqual({ phase: 'failed' });
		expect(m.has(4)).toBe(false);
		expect(liveFacts(m.get(5)).phase).toBe('downloading');
	});
	it('running searches count, finished ones and feed checks do not', () => {
		const m = liveByTitle([], [se(1), se(2, 'done'), se(3, 'queued'), se(4, 'running', 'feed_check')]);
		expect(liveFacts(m.get(1))).toEqual({ phase: 'searching' });
		expect(m.has(2)).toBe(false);
		expect(m.has(3)).toBe(true);
		expect(m.has(4)).toBe(false);
	});
	it('a copy still moving beats a failed one, and a search beats a failed copy', () => {
		const m = liveByTitle([dl(1, 'failed'), dl(1, 'downloading', 0.2), dl(2, 'failed')], [se(2)]);
		expect(liveFacts(m.get(1)).phase).toBe('downloading');
		expect(liveFacts(m.get(2)).phase).toBe('searching');
	});
	it('a failed copy passes on the server reason', () => {
		const m = liveByTitle([{ ...dl(1, 'failed'), text: 'No progress for 6 hours.' }, { ...dl(2, 'failed'), text: ' ' }], []);
		expect(liveFacts(m.get(1))).toEqual({ phase: 'failed', reason: 'No progress for 6 hours.' });
		expect(liveFacts(m.get(2))).toEqual({ phase: 'failed' });
	});
	it('nothing known gives no facts', () => {
		expect(liveFacts(undefined)).toEqual({});
	});
});

describe('real parts in "more available" (#135)', () => {
	it('counts only real albums the ask did not cover', () => {
		const parts = [
			{ key: 'a', complete: true, real: true },
			{ key: 'b', complete: false, real: true },
			{ key: 'single', complete: false, real: false },
			{ key: 'remix', complete: false, real: false }
		];
		expect(partialFacts('albums', parts, ['a']).more).toBe(1);
	});
	it('realAlbum drops singles, secondary types and bootleg titles', () => {
		expect(realAlbum({ type: 'Album', secondary_types: [], name: 'Views' })).toBe(true);
		expect(realAlbum({ type: 'Single', secondary_types: [], name: 'Hotline Bling' })).toBe(false);
		expect(realAlbum({ type: 'Album', secondary_types: ['Remix'], name: 'X' })).toBe(false);
		expect(realAlbum({ type: 'Album', secondary_types: [], name: 'Unreleased Bootleg Vol. 2' })).toBe(false);
	});
});

describe('a download waiting on a person', () => {
	it('shows Needs a decision instead of Processing', () => {
		for (const phase of ['needs_a_decision', 'import_refused']) {
			const t = liveByTitle([dl(7, phase, 1)], []).get(7);
			expect(liveFacts(t)).toEqual({ phase: 'stuck', progress: 1 });
		}
	});
	it('a copy still downloading wins over a stuck one', () => {
		expect(liveByTitle([dl(7, 'needs_a_decision', 1), dl(7, 'downloading', 0.4)], []).get(7)?.download?.phase).toBe('downloading');
	});
});
