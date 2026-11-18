import { describe, expect, it, vi } from 'vitest';
import { render } from 'svelte/server';

vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/downloads') } }));
import DlSections from './DlSections.svelte';
import { applyPatch } from './dl-store.ts';
import type { DownloadRow } from '../core/types.ts';

const base = { importOf: new Map(), titles: new Map(), busy: 0, decisionsOf: () => [], onfail: () => {}, onremove: () => {}, ondone: () => {} };
const row = {
	id: 7, phase: 'done', state: 'finished', label: 'Finished', text: '', progress: 1, speed: 0, active: false, name: 'Film.2020.1080p', title: 'Film',
	downloader: 'qBittorrent', downloader_kind: 'qbittorrent', protocol: 'torrent', downloader_id: 2, hash: 'a'.repeat(40), live_key: '2:' + 'a'.repeat(40),
	size: 1000, updated_at: '2026-11-15T00:00:00Z', can_act: true, seeding: true
} as unknown as DownloadRow;

function statusCell(body: string): string {
	const i = body.indexOf('data-col="status" title="');
	expect(i).toBeGreaterThan(-1);
	return body.slice(i, i + 400);
}

describe('a finished torrent still seeding in the list', () => {
	it('reads Done · seeding in the row and its title text', () => {
		const cell = statusCell(render(DlSections, { props: { ...base, rows: [row] } }).body);
		expect(cell).toContain('title="Done · seeding"');
		expect(cell).toContain('>Done · seeding</span>');
	});
	it('still reads Done · seeding after a live patch of its numbers', () => {
		const out = applyPatch([row], { boot: '', revs: { '2': 1 } }, { kind: 'patch', client: 2, fromRev: 1, toRev: 2, upsert: [{ key: row.live_key!, progress: 1, up_speed: 5000, ratio: 1.2 }] });
		expect(out.kind).toBe('applied');
		const rows = (out as { rows: DownloadRow[] }).rows;
		const cell = statusCell(render(DlSections, { props: { ...base, rows } }).body);
		expect(cell).toContain('>Done · seeding</span>');
	});
	it('reads plain Done once it stops seeding', () => {
		const cell = statusCell(render(DlSections, { props: { ...base, rows: [{ ...row, seeding: false }] } }).body);
		expect(cell).toContain('>Done</span>');
	});
});
