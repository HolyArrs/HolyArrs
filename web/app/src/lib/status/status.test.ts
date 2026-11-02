import { describe, expect, it } from 'vitest';
import { realTime, buildRows, groupLine, groupRows, headline, jobTone, linkId, lookOf, needsAttention } from './status.ts';
import type { ScheduleRow } from '../schedules.ts';
import type { StageState } from '../types.ts';

const link = (kind: StageState['kind'], id: number, state: StageState['state'], name = `${kind} ${id}`): StageState => ({
	key: `link:${kind}:${id}`, kind, name, state, reason: state === 'failed' ? "Can't reach it" : '', since: '2026-11-02T09:00:00Z', stuck: false, restarts: 0
});
const job = (o: Partial<ScheduleRow>): ScheduleRow => ({
	kind: 'refresh', label: 'Refresh titles', about: '', every_s: 3600, min_s: 60, max_s: 86400, step_s: 60, enabled: true, state: 'waiting', part: '', ...o
});
const NOW = Date.parse('2026-11-02T10:00:00Z');

describe('status rows', () => {
	it('reads ids from link keys', () => {
		expect(linkId('link:source:3')).toBe(3);
		expect(linkId('stage:x')).toBeNull();
	});

	it('groups in the fixed order and leaves stages and parts out', () => {
		const rows = buildRows({
			states: [link('downloader', 1, 'running'), link('source', 2, 'failed'), { ...link('stage', 9, 'running'), key: 'stage:x' }],
			metadataUp: true,
			schedules: [job({ last_run: '2026-11-02T09:30:00Z', last_job: 'done', next_run: '2026-11-02T10:30:00Z' })],
			now: NOW
		});
		expect(groupRows(rows).map((g) => g.id)).toEqual(['sources', 'downloaders', 'metadata', 'jobs']);
		expect(rows.find((r) => r.group === 'sources')?.check).toEqual({ kind: 'source', id: 2 });
	});

	it('puts broken first, then warnings, never off', () => {
		const rows = buildRows({ states: [link('source', 1, 'retrying'), link('source', 2, 'off'), link('downloader', 3, 'failed')] });
		expect(needsAttention(rows).map((r) => r.key)).toEqual(['link:downloader:3', 'link:source:1']);
	});

	it('says job results plainly', () => {
		expect(jobTone(job({ last_job: 'failed', last_job_error: 'timed out' }), NOW)).toMatchObject({ word: 'Failed', reason: 'timed out' });
		expect(jobTone(job({}), NOW).word).toBe('Not run yet');
		expect(jobTone(job({ last_run: '2026-11-02T08:00:00Z', next_run: '2026-11-02T09:00:00Z' }), NOW).word).toBe('Late');
		expect(jobTone(job({ enabled: false }), NOW).tone).toBe('idle');
	});

	it('builds list and alert rows from their last error', () => {
		const rows = buildRows({
			lists: [{ id: 1, name: 'Top', enabled: true, last_error: 'not found', last_result: '' } as never],
			alerts: [{ id: 2, name: 'Discord', enabled: true, last_sent_at: '2026-11-02T09:00:00Z' } as never]
		});
		expect(rows.map((r) => [r.key, r.word])).toEqual([['list:1', 'Failed'], ['alert:2', 'Working']]);
	});

	it('writes the summary lines', () => {
		const rows = buildRows({ states: [link('source', 1, 'running'), link('source', 2, 'running'), link('source', 3, 'failed'), link('source', 4, 'off')] });
		expect(groupLine(groupRows(rows)[0])).toBe('2 of 4 working · 1 needs attention · 1 off');
		expect(headline(rows)).toBe('1 part needs attention');
		expect(headline([])).toBe('Everything is working');
	});

	it('drops empty server times', () => {
		expect(realTime('0001-01-01T00:00:00Z')).toBeUndefined();
		expect(realTime('2026-11-02T09:00:00Z')).toBe('2026-11-02T09:00:00Z');
	});

	it('defaults the look to b', () => {
		expect(lookOf(null)).toBe('f');
		expect(lookOf('a')).toBe('a');
		expect(lookOf('zz')).toBe('f');
		expect(lookOf('b')).toBe('b');
	});
});
