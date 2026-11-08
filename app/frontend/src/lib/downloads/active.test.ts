import { summarise } from './active.svelte.ts';
import { split } from './downloaders.ts';
import { describe, expect, it } from 'vitest';

describe('summarise', () => {
	it('is empty when nothing downloads', () => {
		expect(summarise([{ phase: 'done', progress: 1 }])).toEqual({ count: 0, percent: 0 });
	});
	it('matches the Downloads page Now list, imports included', () => {
		const rows = [{ phase: 'importing', progress: 1 }, { phase: 'waiting_to_import', progress: 1 }, { phase: 'downloading', progress: 0 }, { phase: 'failed', progress: 0 }];
		expect(summarise(rows).count).toBe(split(rows as never).now.length);
	});
	it('counts every active row: waiting, downloading, stalled and paused', () => {
		expect(summarise([{ phase: 'downloading', progress: 0.2 }, { phase: 'downloading', progress: 0.5 }, { phase: 'paused', progress: 0.9 }, { phase: 'waiting', progress: 0 }, { phase: 'stalled', progress: 0.2 }, { phase: 'done', progress: 1 }])).toEqual({ count: 5, percent: 36 });
	});
});
