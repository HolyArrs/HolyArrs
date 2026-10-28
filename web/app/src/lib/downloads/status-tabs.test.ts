import { describe, expect, it } from 'vitest';
import { stageOf, statusWord } from './status-tabs.ts';

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

describe('stageOf', () => {
	it('puts every phase on one stage, the same for a torrent and a Usenet download', () => {
		expect(rows.map((x) => stageOf(x))).toEqual(['queued', 'downloading', 'importing', 'stalled', 'paused', 'importing', 'importing', 'done', 'seeding', 'failed', 'done', 'failed', 'decide', 'failed']);
	});
});

describe('statusWord', () => {
	it('follows the sub-tab words', () => {
		expect(rows.map((x) => statusWord(x))).toEqual(['Queued', 'Downloading', 'Importing', 'Stalled', 'Paused', 'Importing', 'Importing', 'Done', 'Seeding', 'Failed', 'Removed', 'Failed', 'Needs a decision', 'Failed']);
	});
});
