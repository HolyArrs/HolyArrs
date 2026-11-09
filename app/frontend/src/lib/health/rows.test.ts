import { describe, expect, it } from 'vitest';
import { realTime, buildRows, groupRows, headline, jobTone, linkId, everyKind, needsAttention } from './rows.ts';
import { nextRun, type ScheduleRow } from './schedules.ts';
import type { StageState } from './types.ts';

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
		expect(jobTone(job({ enabled: false }), NOW)).toMatchObject({ tone: 'idle', word: 'Turned off' });
		expect(jobTone(job({ state: 'part_off' }), NOW).word).toBe('Part is off');
		expect(jobTone(job({ state: 'safe_mode' }), NOW).word).toBe('Safe mode');
	});

	it('builds list and alert rows from their last error', () => {
		const rows = buildRows({
			lists: [{ id: 1, name: 'Top', enabled: true, last_error: 'not found', last_result: '' } as never],
			alerts: [{ id: 2, name: 'Discord', enabled: true, last_sent_at: '2026-11-02T09:00:00Z' } as never, { id: 3, name: 'Mail', enabled: true } as never]
		});
		expect(rows.map((r) => [r.key, r.word, r.tone])).toEqual([['list:1', 'Broken', 'fail'], ['alert:2', 'Working', 'ok'], ['alert:3', 'Not sent yet', 'idle']]);
		expect(buildRows({ metadataUp: false })[0].word).toBe('Broken');
	});

	it('names health checks once and keeps updates out of Storage', () => {
		const chk = (kind: string, subject: string) => ({ key: kind, kind, subject, state: 'error', message: 'x' }) as never;
		const rows = buildRows({ checks: [chk('backups', 'Backups'), chk('update', 'HolyArrs 1.2.3'), chk('disk_space', 'Movies')] });
		expect(rows.map((r) => [r.name, r.group])).toEqual([['Backups', 'library'], ['HolyArrs 1.2.3', 'app'], ['Free space Movies', 'library']]);
		expect(groupRows(rows).map((g) => g.label)).toEqual(['Storage', 'Updates']);
	});

	it('lists free space on the config disk, so a member sent here from Home sees it', () => {
		const rows = buildRows({ checks: [{ key: 'config_disk', kind: 'config_disk', subject: 'Config folder', state: 'error', message: 'Only 100 MB are free' } as never] });
		expect(rows.map((r) => [r.name, r.group, r.tone, r.reason])).toEqual([['Config folder space', 'library', 'fail', 'Only 100 MB are free']]);
	});

	it('shows the /config mapping error and waiting downloads, and counts them', () => {
		const rows = buildRows({
			checks: [
				{ key: 'config_folder', kind: 'config_folder', subject: 'Config folder', state: 'error', message: '/config is not mapped' } as never,
				{ key: 'waiting_downloads', kind: 'waiting_downloads', subject: 'Downloads', state: 'warning', message: '2 downloads wait' } as never
			]
		});
		expect(rows.map((r) => [r.name, r.group, r.tone, r.reason])).toEqual([
			['Config folder mapping', 'library', 'fail', 'Make a backup first and download it. Then map /config to a folder in your container settings, start HolyArrs and restore the backup.'],
			['Downloads waiting for files', 'library', 'wait', '2 downloads wait']
		]);
		expect(rows[0].fix).toBe('/settings/backups');
		expect(rows[1].fix).toBe('/downloads?show=waiting.paused');
		expect(headline(rows)).toBe('2 checks need attention');
	});

	it('does not count a starting part as a problem', () => {
		expect(needsAttention(buildRows({ states: [link('source', 1, 'starting')] }))).toEqual([]);
	});

	it('writes the summary lines', () => {
		const rows = buildRows({ states: [link('source', 1, 'running'), link('source', 2, 'running'), link('source', 3, 'failed'), link('source', 4, 'off')] });
		expect(headline(rows)).toBe('1 check needs attention');
		expect(headline(buildRows({ states: [link('source', 1, 'running')] }))).toBe('Everything is working');
		expect(headline([])).toBe('No sources or downloaders added yet');
		expect(headline([], false)).toBe('No problems found');
	});

	it('says how late an overdue job is', () => {
		expect(nextRun('2026-11-02T09:35:00Z', NOW)).toBe('due 25 min ago');
		expect(nextRun('2026-11-02T09:59:40Z', NOW)).toBe('now');
		expect(nextRun('2026-11-02T10:30:00Z', NOW)).toBe('in 30 min');
	});

	it('drops empty server times', () => {
		expect(realTime('0001-01-01T00:00:00Z')).toBeUndefined();
		expect(realTime('2026-11-02T09:00:00Z')).toBe('2026-11-02T09:00:00Z');
	});

	it('keeps every kind of part, empty ones included', () => {
		const ks = everyKind([{ id: 'downloaders', label: 'Downloaders', rows: [] }]);
		expect(ks.map((k) => k.label)).toEqual(['Sources', 'Downloaders', 'Media servers', 'Subtitle sites', 'Outside lists', 'Alerts', 'Metadata server']);
		expect(ks.map((k) => k.id)).toEqual(['sources', 'downloaders', 'media', 'subtitles', 'lists', 'alerts', 'metadata']);
		expect(ks.every((k) => Array.isArray(k.rows))).toBe(true);
	});
});
