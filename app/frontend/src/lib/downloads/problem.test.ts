import { problem } from './problem.ts';
import { describe, expect, it } from 'vitest';

const row = (o: Record<string, unknown>) => ({ phase: 'downloading', state: 'downloading', progress: 0.4, text: '', ...o }) as Parameters<typeof problem>[0];

describe('problem', () => {
	it('is null for a healthy, paused or finished row', () => {
		expect(problem(row({ protocol: 'torrent', availability: 3 }))).toBeNull();
		expect(problem(row({ phase: 'paused', state: 'paused', tracker_problem: 'error' }))).toBeNull();
		expect(problem(row({ phase: 'done', state: 'done', progress: 1, availability: 0.2 }))).toBeNull();
	});
	it('reads Usenet fail and warning lines', () => {
		expect(problem(row({ protocol: 'usenet', phase: 'failed', state: 'failed', text: 'Aborted, cannot be completed: 12 articles missing' }))).toEqual({ text: 'missing articles', tone: 'fail' });
		expect(problem(row({ downloader_kind: 'nzbget', text: 'Repair needed (par2)' }))).toEqual({ text: 'repair needed', tone: 'wait' });
	});
	it('names tracker problems', () => {
		expect(problem(row({ protocol: 'torrent', tracker_problem: 'error' }))).toEqual({ text: 'tracker error', tone: 'fail' });
		expect(problem(row({ protocol: 'torrent', tracker_problem: 'announce' }))?.text).toBe('no tracker reply');
		expect(problem(row({ protocol: 'torrent', tracker_problem: 'warning' }))).toEqual({ text: 'tracker warning', tone: 'wait' });
	});
	it('says no full copy under one copy, red once stalled', () => {
		expect(problem(row({ protocol: 'torrent', availability: 0.6 }))).toEqual({ text: 'no full copy', tone: 'wait' });
		expect(problem(row({ protocol: 'torrent', phase: 'stalled', state: 'stalled', availability: 0.6 }))?.tone).toBe('fail');
	});
	it('says no peers for a stalled torrent nobody shares', () => {
		expect(problem(row({ protocol: 'torrent', phase: 'stalled', state: 'stalled', seeds: 0, peers: 0 }))).toEqual({ text: 'no peers', tone: 'wait' });
		expect(problem(row({ protocol: 'torrent', phase: 'stalled', state: 'stalled', seeds: 2 }))).toBeNull();
	});
	it('says import blocked when the files did not match', () => {
		expect(problem(row({ phase: 'import_refused', state: 'failed' }))).toEqual({ text: 'import blocked', tone: 'fail' });
	});
	it('says nothing about copies or peers for a season pack taken a few episodes at a time', () => {
		const pack = { from: 1, to: 2, taken: 2, total: 9 };
		expect(problem(row({ protocol: 'torrent', availability: 0.4, pack }))).toBeNull();
		expect(problem(row({ protocol: 'torrent', phase: 'stalled', state: 'stalled', seeds: 0, peers: 0, pack }))).toBeNull();
		expect(problem(row({ protocol: 'torrent', tracker_problem: 'error', pack }))?.text).toBe('tracker error');
	});
});
