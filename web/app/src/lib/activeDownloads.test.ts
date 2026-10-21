import { describe, expect, it } from 'vitest';
import { summarise } from './activeDownloads.svelte.ts';

describe('summarise', () => {
	it('is empty when nothing downloads', () => {
		expect(summarise([{ phase: 'importing', progress: 1 }])).toEqual({ count: 0, percent: 0 });
	});
	it('counts every active row: waiting, downloading, stalled and paused', () => {
		expect(summarise([{ phase: 'downloading', progress: 0.2 }, { phase: 'downloading', progress: 0.5 }, { phase: 'paused', progress: 0.9 }, { phase: 'waiting', progress: 0 }, { phase: 'stalled', progress: 0.2 }, { phase: 'done', progress: 1 }])).toEqual({ count: 5, percent: 36 });
	});
});
