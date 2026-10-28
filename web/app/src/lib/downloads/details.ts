// What the Download details views show, worked out once so every look says the
// same thing: what it is, where it is, what happens next. Empty facts are left
// out. Pure, so it can be tested.
import { bytes, duration, percent } from '../format.ts';
import { manyWords, type DownloadRow } from '../downloaders.ts';
import type { IconName } from '../Icon.svelte';
import type { ImportRecord } from '../types.ts';
import { failReason, placed } from './imports.ts';
import { stageOf, type Stage } from './status-tabs.ts';

export interface Fact {
	key: string;
	label: string;
	value: string;
}

export type StepState = 'done' | 'now' | 'next' | 'fail';
export interface Step {
	key: 'download' | 'import' | 'library';
	label: string;
	state: StepState;
}

export interface DetailFacts {
	stage: Stage;
	/** What it is: episode or track, Version, size. */
	what: Fact[];
	/** Where it is: progress, speed, time left, ratio, Downloader. */
	where: Fact[];
	/** What happens next: the import and where it went. */
	next: Fact[];
	/** One plain sentence about what happens next. */
	line: string;
	/** Download → Import → Library, each done, now, next or failed. */
	steps: Step[];
	/** Whether a progress bar means anything (not failed, removed or gone). */
	moving: boolean;
}

/** The small symbol each stage gets in the Status column; the word is its name. */
export const stageIcon: Record<Stage | 'removed', IconName> = {
	queued: 'clock',
	paused: 'pause',
	downloading: 'download',
	stalled: 'alert',
	importing: 'folder',
	seeding: 'upload',
	done: 'check',
	decide: 'info',
	failed: 'x',
	removed: 'trash'
};

/** iconOf is the Status column symbol for a row. */
export function iconOf(r: Pick<DownloadRow, 'phase' | 'state' | 'progress'>): IconName {
	return r.phase === 'removed' ? stageIcon.removed : stageIcon[stageOf(r)];
}

const fact = (key: string, label: string, value: string | number | undefined | null): Fact | null =>
	value === undefined || value === null || value === '' ? null : { key, label, value: String(value) };
const keep = (xs: (Fact | null)[]): Fact[] => xs.filter((x): x is Fact => x !== null);

function nextLine(r: DownloadRow, stage: Stage, im: ImportRecord | undefined): string {
	const put = placed(im);
	const why = failReason(im);
	const where = put?.where ? ` in ${put.where}` : '';
	switch (stage) {
		case 'queued':
			return 'Starts when the Downloader has room, then HolyArrs imports it.';
		case 'paused':
			return `Paused in ${r.downloader || 'the Downloader'}. It goes on when it is resumed there.`;
		case 'downloading':
			return 'When it finishes, HolyArrs imports it into the Library.';
		case 'stalled':
			return 'Nothing is coming in. Wait, or mark it failed to try the next Release.';
		case 'importing':
			return r.phase === 'waiting_to_import' ? 'Next in line to be imported into the Library.' : 'HolyArrs is putting the files into the Library.';
		case 'seeding':
			return `In the Library${where}. The Downloader still shares it.`;
		case 'decide':
			return 'Waiting for your decision.';
		case 'failed':
			if (why) return `The import failed: ${why}.`;
			if (r.phase === 'vanished') return 'It was removed outside HolyArrs.';
			if (r.phase === 'import_refused') return 'The files did not match, so nothing was imported.';
			return 'Its Release is turned down; HolyArrs looks for the next one.';
		case 'done':
			return r.phase === 'removed' ? 'Removed from the Downloader.' : `In the Library${where}.`;
	}
}

function stepsOf(stage: Stage, im: ImportRecord | undefined): Step[] {
	const s = (download: StepState, imp: StepState, library: StepState): Step[] => [
		{ key: 'download', label: 'Download', state: download },
		{ key: 'import', label: 'Import', state: imp },
		{ key: 'library', label: 'Library', state: library }
	];
	switch (stage) {
		case 'queued':
		case 'paused':
		case 'downloading':
			return s('now', 'next', 'next');
		case 'stalled':
			return s('fail', 'next', 'next');
		case 'importing':
			return s('done', 'now', 'next');
		case 'decide':
			return s('done', 'fail', 'next');
		case 'failed':
			return im?.State === 'failed' ? s('done', 'fail', 'next') : s('fail', 'next', 'next');
		case 'seeding':
		case 'done':
			return s('done', 'done', 'done');
	}
}

/** detailFacts gathers what a details view shows for one row. */
export function detailFacts(r: DownloadRow, im: ImportRecord | undefined): DetailFacts {
	const stage = stageOf(r);
	const w = manyWords(r.kind);
	const put = placed(im);
	const why = failReason(im);
	const going = r.active && r.phase === 'downloading';
	const moving = r.id > 0 && r.phase !== 'failed' && r.phase !== 'removed' && r.phase !== 'vanished';
	const many = r.episodes && r.episodes.length > 1 ? `${r.episodes.length} ${w?.many ?? 'episodes'}` : '';
	const what = keep([
		w && r.covers ? fact('covers', w.one === 'track' ? 'Track' : 'Episode', r.covers) : null,
		many ? fact('count', 'Covers', many) : null,
		r.version && r.version !== 'Main' ? fact('version', 'Version', r.version) : null,
		fact('size', 'Size', r.size ? bytes(r.size) : '')
	]);
	const where = keep([
		moving ? fact('progress', 'Progress', `${Math.floor(percent(r.progress))}%`) : null,
		going && r.speed ? fact('speed', 'Speed', `${bytes(r.speed)}/s`) : null,
		going && r.eta ? fact('eta', 'Time left', duration(r.eta)) : null,
		stage === 'seeding' && typeof r.ratio === 'number' ? fact('ratio', 'Ratio', r.ratio.toFixed(2)) : null,
		fact('downloader', 'Downloader', r.downloader)
	]);
	const next = keep([
		put ? fact('import', 'Import', put.how) : why ? fact('import', 'Import', `Failed: ${why}`) : null,
		put?.where ? fact('folder', 'Folder', put.where) : null
	]);
	return { stage, what, where, next, line: nextLine(r, stage, im), steps: stepsOf(stage, im), moving };
}

/** files lists what an import planned or placed: the file name and the Library folder it goes to. */
export function filesOf(im: ImportRecord | undefined): { name: string; to: string; outcome: string }[] {
	if (!im) return [];
	const res = new Map((im.Result ?? []).map((x) => [x.source_file, x.outcome]));
	const base = (p: string) => p.split(/[\\/]+/).filter(Boolean).pop() ?? p;
	const dir = (p: string | undefined) => {
		const parts = (p ?? '').split(/[\\/]+/).filter(Boolean);
		return parts.length > 1 ? parts[parts.length - 2] : '';
	};
	const plan = im.Plan ?? [];
	if (plan.length) return plan.map((p) => ({ name: base(p.source_file), to: dir(p.target_path), outcome: res.get(p.source_file) ?? '' }));
	return (im.Result ?? []).map((x) => ({ name: base(x.source_file), to: '', outcome: x.outcome }));
}

/** activity is the short timeline of a download: when it started, finished, and was imported. */
export function activityOf(r: Pick<DownloadRow, 'started_at' | 'finished_at' | 'updated_at'>, im: ImportRecord | undefined): { key: string; label: string; at: string }[] {
	const out: { key: string; label: string; at: string }[] = [];
	if (r.started_at) out.push({ key: 'started', label: 'Sent to the Downloader', at: r.started_at });
	if (r.finished_at) out.push({ key: 'finished', label: 'Finished downloading', at: r.finished_at });
	if (im) out.push({ key: 'import', label: im.State === 'failed' ? 'Import failed' : im.State === 'done' || im.State === 'partly_done' ? 'Imported' : 'Import started', at: im.CreatedAt });
	if (!out.length && r.updated_at) out.push({ key: 'updated', label: 'Last change', at: r.updated_at });
	return out.sort((a, b) => a.at.localeCompare(b.at));
}
