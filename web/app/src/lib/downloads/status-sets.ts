// The five sets of status chips Luis is comparing on the Downloads table
// (?set=1…5). Every row lands on one fine-grained stage first; each set then
// groups stages into its chips. Pure, so it can be tested.
//
// The words mean the same thing for a torrent and a Usenet download:
// Downloading is only bytes coming in; Importing is everything after the
// bytes arrive (Usenet verify, repair and unpack, waiting to import, and
// putting the files in the Library); Seeding is a finished torrent the
// Downloader still shares.
import type { DownloadRow } from '../downloaders.ts';

export type Stage = 'queued' | 'downloading' | 'importing' | 'seeding' | 'done' | 'paused' | 'stalled' | 'failed';

type RowBits = Pick<DownloadRow, 'phase' | 'state' | 'progress'>;

/** stageOf places one row on its fine-grained stage. */
export function stageOf(r: RowBits): Stage {
	switch (r.phase) {
		case 'waiting':
			return 'queued';
		case 'downloading':
			// A Usenet job at full progress is being verified, repaired or
			// unpacked by the Downloader: the bytes are in, so it is importing.
			return r.progress >= 1 ? 'importing' : 'downloading';
		case 'waiting_to_import':
		case 'importing':
			return 'importing';
		case 'paused':
			return 'paused';
		case 'stalled':
			return 'stalled';
		case 'failed':
		case 'vanished':
		case 'needs_a_decision':
		case 'import_refused':
			return 'failed';
		case 'done':
			return r.state === 'seeding' ? 'seeding' : 'done';
	}
	// removed (cleaned up) and anything new: finished as far as the table goes.
	return 'done';
}

export type SetId = 1 | 2 | 3 | 4 | 5;

export interface Sub {
	key: string;
	label: string;
	stages: Stage[];
}

export interface Chip {
	key: string;
	label: string;
	stages: Stage[];
	/** Show the seeding sub-count on this chip ("Done 12 · 4 seeding"). */
	seedCount?: boolean;
	/** Set 4: the small second row under this chip. */
	subs?: Sub[];
}

export interface StatusSet {
	id: SetId;
	/** The VariantBar label. */
	label: string;
	chips: Chip[];
	/** The word for a seeding row in the status column (sets 1, 2: Done plus a Seeding tag). */
	seedWord: 'Done' | 'Seeding';
	/** The word for a waiting row. */
	queuedWord: 'Waiting' | 'Queued';
}

export const ALL = 'all';

const problems: Stage[] = ['stalled', 'paused', 'failed'];

export const SETS: Record<SetId, StatusSet> = {
	1: {
		id: 1,
		label: '1 Path',
		seedWord: 'Done',
		queuedWord: 'Waiting',
		chips: [
			{ key: 'waiting', label: 'Waiting', stages: ['queued'] },
			{ key: 'downloading', label: 'Downloading', stages: ['downloading'] },
			{ key: 'importing', label: 'Importing', stages: ['importing'] },
			{ key: 'done', label: 'Done', stages: ['done', 'seeding'], seedCount: true },
			{ key: 'problems', label: 'Problems', stages: problems }
		]
	},
	2: {
		id: 2,
		label: '2 Moving',
		seedWord: 'Done',
		queuedWord: 'Waiting',
		chips: [
			{ key: 'downloading', label: 'Downloading', stages: ['downloading'] },
			{ key: 'importing', label: 'Importing', stages: ['importing'] },
			{ key: 'done', label: 'Done', stages: ['done', 'seeding'], seedCount: true },
			{ key: 'stopped', label: 'Stopped', stages: ['queued', 'paused', 'stalled', 'failed'] }
		]
	},
	3: {
		id: 3,
		label: '3 Active',
		seedWord: 'Seeding',
		queuedWord: 'Waiting',
		chips: [
			{ key: 'active', label: 'Active', stages: ['downloading', 'importing', 'seeding'] },
			{ key: 'waiting', label: 'Waiting', stages: ['queued'] },
			{ key: 'paused', label: 'Paused', stages: ['paused'] },
			{ key: 'done', label: 'Done', stages: ['done'] },
			{ key: 'problems', label: 'Problems', stages: ['stalled', 'failed'] }
		]
	},
	4: {
		id: 4,
		label: '4 Two-level',
		seedWord: 'Done',
		queuedWord: 'Waiting',
		chips: [
			{
				key: 'progress',
				label: 'In progress',
				stages: ['queued', 'downloading', 'importing'],
				subs: [
					{ key: 'waiting', label: 'Waiting', stages: ['queued'] },
					{ key: 'downloading', label: 'Downloading', stages: ['downloading'] },
					{ key: 'importing', label: 'Importing', stages: ['importing'] }
				]
			},
			{
				key: 'finished',
				label: 'Finished',
				stages: ['done', 'seeding'],
				seedCount: true,
				subs: [
					{ key: 'done', label: 'Done', stages: ['done'] },
					{ key: 'seeding', label: 'Seeding', stages: ['seeding'] }
				]
			},
			{
				key: 'needs',
				label: 'Needs you',
				stages: problems,
				subs: [
					{ key: 'paused', label: 'Paused', stages: ['paused'] },
					{ key: 'stalled', label: 'Stalled', stages: ['stalled'] },
					{ key: 'failed', label: 'Failed', stages: ['failed'] }
				]
			}
		]
	},
	5: {
		id: 5,
		label: '5 Downloader words',
		seedWord: 'Seeding',
		queuedWord: 'Queued',
		chips: [
			{ key: 'queued', label: 'Queued', stages: ['queued'] },
			// Like the Downloaders' own lists, a stalled download stays under
			// Downloading; its status reads Stalled.
			{ key: 'downloading', label: 'Downloading', stages: ['downloading', 'stalled'] },
			{ key: 'importing', label: 'Importing', stages: ['importing'] },
			{ key: 'seeding', label: 'Seeding', stages: ['seeding'] },
			{ key: 'done', label: 'Done', stages: ['done'] },
			{ key: 'paused', label: 'Paused', stages: ['paused'] },
			{ key: 'failed', label: 'Failed', stages: ['failed'] }
		]
	}
};

export const SET_IDS: SetId[] = [1, 2, 3, 4, 5];

/** chipsFor is a set's chips as shown: set 5 drops Seeding (and Done takes
 *  seeding rows) unless a torrent Downloader is set up or a row is seeding. */
export function chipsFor(set: SetId, rows: RowBits[], torrent: boolean): Chip[] {
	const s = SETS[set];
	if (set !== 5 || torrent || rows.some((r) => stageOf(r) === 'seeding')) return s.chips;
	return s.chips.filter((c) => c.key !== 'seeding').map((c) => (c.key === 'done' ? { ...c, stages: ['done', 'seeding'] } : c));
}

/** pick filters rows to one chip (and one sub-chip in set 4); unknown keys show all. */
export function pick<T extends RowBits>(rows: T[], chips: Chip[], show: string, sub = ''): T[] {
	const c = chips.find((x) => x.key === show);
	if (!c) return rows;
	const sc = c.subs?.find((x) => x.key === sub);
	const stages = new Set<Stage>(sc ? sc.stages : c.stages);
	return rows.filter((r) => stages.has(stageOf(r)));
}

export interface Counted {
	key: string;
	label: string;
	count: number;
	/** Rows of this chip that are seeding, when the chip shows that sub-count. */
	seeding?: number;
}

/** counts gives All plus every chip with its row count. */
export function counts(rows: RowBits[], chips: Chip[]): Counted[] {
	const stages = rows.map(stageOf);
	const out: Counted[] = [{ key: ALL, label: 'All', count: rows.length }];
	for (const c of chips) {
		const n = stages.filter((s) => c.stages.includes(s)).length;
		const k: Counted = { key: c.key, label: c.label, count: n };
		if (c.seedCount) k.seeding = stages.filter((s) => s === 'seeding').length;
		out.push(k);
	}
	return out;
}

/** subCounts gives a set-4 chip's sub-chips with their counts. */
export function subCounts(rows: RowBits[], chip: Chip | undefined): Counted[] {
	if (!chip?.subs) return [];
	const stages = rows.map(stageOf);
	return chip.subs.map((s) => ({ key: s.key, label: s.label, count: stages.filter((x) => s.stages.includes(x)).length }));
}

/** statusWord is the row's word in the status column, matching the chip
 *  words of the picked set. Problems keep their own word (Stalled, Vanished,
 *  Needs a decision) so the row says what went wrong. */
export function statusWord(r: Pick<DownloadRow, 'phase' | 'state' | 'progress' | 'label'>, set: SetId): string {
	const s = SETS[set];
	switch (stageOf(r)) {
		case 'queued':
			return s.queuedWord;
		case 'downloading':
			return 'Downloading';
		case 'importing':
			return 'Importing';
		case 'seeding':
			return s.seedWord;
		case 'paused':
			return 'Paused';
		case 'stalled':
			return 'Stalled';
		case 'done':
			return r.phase === 'removed' ? 'Removed' : 'Done';
	}
	switch (r.phase) {
		case 'vanished':
			return 'Vanished';
		case 'needs_a_decision':
			return 'Needs a decision';
		case 'import_refused':
			return 'Import refused';
	}
	return 'Failed';
}

/** seedTag says whether the row shows the small Seeding tag (the sets where
 *  seeding counts as Done). */
export function seedTag(r: RowBits, set: SetId): boolean {
	return stageOf(r) === 'seeding' && SETS[set].seedWord === 'Done';
}

/** setOf reads ?set= (1…5); anything else is set 1. */
export function setOf(v: string | number | null | undefined): SetId {
	const n = Number(v);
	return (SET_IDS as number[]).includes(n) ? (n as SetId) : 1;
}

/** showOf reads ?show= for a set: a chip key (or chip.sub in set 4), else All. */
export function showOf(v: string | null | undefined, chips: Chip[]): { show: string; sub: string } {
	const [a = '', b = ''] = (v ?? '').split('.');
	const c = chips.find((x) => x.key === a);
	if (!c) return { show: ALL, sub: '' };
	return { show: c.key, sub: c.subs?.some((s) => s.key === b) ? b : '' };
}
