import type { ImportRecord } from './types.ts';
import { filesGone, statusDetail } from './imports.ts';
import { stepLine } from './status-tabs.ts';
import { whyWords } from './downloaders.ts';
import { describe, expect, it } from 'vitest';

const im = (Result: ImportRecord['Result']): ImportRecord => ({ ID: 1, DownloadID: 8, State: 'failed', CreatedAt: '2026-11-11T10:00:00Z', Result });

describe('files another app took', () => {
	it('knows an import that found the files gone, new or from an older version', () => {
		expect(filesGone(im([{ source_file: 'a', outcome: 'gone' }, { source_file: 'b.nfo', outcome: 'skipped' }]))).toBe(true);
		expect(filesGone(im([{ source_file: 'a', outcome: 'failed', error: 'lstat /mnt/t/a.mkv: no such file or directory' }]))).toBe(true);
		expect(filesGone(im([{ source_file: 'a', outcome: 'failed', error: 'disk full' }]))).toBe(false);
		expect(filesGone(im([{ source_file: 'a', outcome: 'skipped' }]))).toBe(false);
		expect(filesGone(undefined)).toBe(false);
	});
	it('says so on the row, done or waiting for a decision', () => {
		const gone = im([{ source_file: 'a', outcome: 'gone' }]);
		const words = 'another app took the files before HolyArrs could import them';
		expect(statusDetail({ phase: 'done', state: 'finished', progress: 1 }, gone)).toBe(words);
		expect(statusDetail({ phase: 'needs_a_decision', state: 'finished', progress: 1 }, im([{ source_file: 'a', outcome: 'failed', error: 'lstat /x: no such file or directory' }]))).toBe(words);
		expect(stepLine({}, words)).toBe('Taken by another app');
	});
	it('names it on the Needs a decision card', () => {
		expect(whyWords({ why: 'taken', has_copy: false })).toContain('Another app took the files');
	});
});
