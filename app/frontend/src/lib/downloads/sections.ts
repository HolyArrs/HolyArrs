// The Downloads page in two sections, Queue and History, with the header strip's totals, the speed graph and the facts of a row's details panel. Pure, so it can be tested.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { ImportRecord } from './types.ts';
import { stageOf, statusWord } from './status-tabs.ts';
import { failReason, statusDetail } from './imports.ts';
import { size } from './size.ts';
import { quality } from './columns.ts';
import type { DownloadRow } from '../core/types.ts';
import { dateTime } from '../core/format.ts';

type Bits = Pick<DownloadRow, 'phase' | 'state' | 'progress'>;

export type BarTone = 'go' | 'queued' | 'held' | 'bad';

export interface Totals {
	/** Bytes per second across every download. */
	speed: number;
	/** Bytes per second sent out across every download. */
	up: number;
	/** Seconds until the whole Queue is in at today's speed (0 means unknown). */
	eta: number;
	/** Bytes the Queue still has to fetch. */
	left: number;
}

export interface Fact {
	label: string;
	value: string;
}

/** Done, seeding, failed and removed downloads; everything else is in the Queue. */
export function inHistory(r: Bits): boolean {
	const s = stageOf(r);
	return s === 'done' || s === 'seeding' || s === 'failed';
}

/** Colours a Queue bar: green taking in data or importing, blue queued, amber paused or stalled, red a problem. */
export function barTone(r: Bits): BarTone {
	switch (stageOf(r)) {
		case 'queued':
			return 'queued';
		case 'paused':
		case 'stalled':
			return 'held';
		case 'decide':
		case 'failed':
			return 'bad';
	}
	return 'go';
}

/** Bytes a download still has to fetch. */
const left = (r: Pick<DownloadRow, 'size' | 'progress'>) => Math.max(0, (r.size || 0) * (1 - Math.max(0, Math.min(1, r.progress || 0))));

/** A row taking in data right now. A queued, paused, stalled or finished row has no speed of its own, whatever the Downloader last said. */
export function moving(r: Pick<DownloadRow, 'phase' | 'state' | 'progress' | 'speed' | 'active'>): boolean {
	return r.active && r.speed > 0 && stageOf(r) === 'downloading';
}

/** Adds up the Queue for the header strip. */
export function totals(queue: Pick<DownloadRow, 'phase' | 'state' | 'size' | 'progress' | 'speed' | 'up_speed' | 'active'>[]): Totals {
	const speed = queue.reduce((n, r) => n + (moving(r) ? r.speed : 0), 0);
	const rest = queue.reduce((n, r) => n + left(r), 0);
	const up = queue.reduce((n, r) => n + (moving(r) ? (r.up_speed ?? 0) : 0), 0);
	return { speed, up, left: rest, eta: speed > 0 && rest > 0 ? Math.round(rest / speed) : 0 };
}

/** What finished today and this month, from the rows on the page. */
export function historyTotals(rows: Pick<DownloadRow, 'size' | 'finished_at' | 'updated_at' | 'phase' | 'state' | 'progress'>[], now = new Date()): { today: number; month: number } {
	let today = 0;
	let month = 0;
	for (const r of rows) {
		if (stageOf(r) !== 'done' && stageOf(r) !== 'seeding') continue;
		const at = new Date(r.finished_at || r.updated_at);
		if (Number.isNaN(at.getTime()) || at.getFullYear() !== now.getFullYear() || at.getMonth() !== now.getMonth()) continue;
		month += r.size || 0;
		if (at.getDate() === now.getDate()) today += r.size || 0;
	}
	return { today, month };
}

/** The Downloads search: title, episode, Release name, Downloader and the quality read from the name. */
export function matches(r: Pick<DownloadRow, 'name' | 'title' | 'covers' | 'album' | 'downloader' | 'downloader_kind'>, q: string): boolean {
	const s = q.trim().toLowerCase();
	if (!s) return true;
	return [r.title, r.name, r.covers, r.album, r.downloader, r.downloader_kind, quality(r.name)].some((v) => v?.toLowerCase().includes(s));
}

/** A History row's text and whether it reads as a failure. */
export function historyStatus(r: DownloadRow, im: ImportRecord | undefined): { text: string; bad: boolean } {
	const s = stageOf(r);
	// A client item has no import, so its word is the client's own state.
	if (r.client_item) {
		const why = s === 'failed' ? r.text?.trim() : '';
		return { text: why ? t('downloads.fact.colon', { word: statusWord(r), why }) : statusWord(r), bad: s === 'failed' };
	}
	if (s === 'failed') {
		const why = failReason(im) ?? statusDetail(r, im);
		return { text: why ? t('downloads.fact.failedWhy', { why }) : t('downloads.fact.failed'), bad: true };
	}
	if (r.phase === 'removed') return { text: t('downloads.fact.removed'), bad: false };
	return { text: s === 'seeding' ? t('downloads.fact.doneSeeding') : t('downloads.fact.done'), bad: false };
}

/** The details panel as label and value pairs, only those with a value. */
export function facts(r: DownloadRow, im: ImportRecord | undefined): Fact[] {
	const more = statusDetail(r, im);
	const target = im?.Plan?.find((p) => p.target_path)?.target_path;
	const source = im?.Plan?.[0]?.source_file;
	const out: [string, string | undefined][] = [
		[t('downloads.fact.name'), r.name],
		[t('downloads.fact.status'), more ? `${statusWord(r)} · ${more}` : statusWord(r)],
		[t('downloads.fact.size'), r.size ? size(r.size) : ''],
		[t('downloads.fact.downloader'), r.downloader],
		[t('downloads.fact.path'), target],
		[t('downloads.fact.source'), source],
		[t('downloads.fact.added'), dateTime(r.started_at)],
		[t('downloads.fact.completed'), dateTime(r.finished_at)],
		[t('downloads.fact.version'), r.version && r.version !== 'Main' ? r.version : '']
	];
	return out.filter((x): x is [string, string] => !!x[1]).map(([label, value]) => ({ label, value }));
}

