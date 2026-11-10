// Usenet parts of the Downloads page: which rows can be retried, the words for a job's steps after its bytes are in, and the Servers and Repair tabs.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { DownloadRow } from '../core/types.ts';

/** One step of a finished job, as /usenet answers it. */
export type JobStage = { stage: string; lines: string[] };

/** One news server's counters. */
export type ServerStat = { name: string; total: number; month: number; week: number; day: number; articles_tried: number; articles_found: number };

/** The Servers and Repair tabs of one Usenet job. */
export type UsenetView = { stages: JobStage[]; job_servers: { name: string; amount: string }[]; servers?: { total: number; month: number; week: number; day: number; servers: ServerStat[] } };

type RetryBits = Pick<DownloadRow, 'phase' | 'protocol' | 'client_item' | 'hash' | 'downloader_id' | 'outside'>;

/** A failed Usenet job the Downloader still holds and HolyArrs did not send (its own failed downloads were searched again). NZBGet takes Try again too. */
export function retryable(r: RetryBits & { downloader_kind?: string }): boolean {
	return (controllable(r.downloader_kind) || r.downloader_kind === 'nzbget') && !!r.client_item && !!r.outside && !!r.hash && !!r.downloader_id && r.protocol === 'usenet' && r.phase === 'failed';
}

/** Whether Pause and Resume mean anything: a Usenet job in the finished list cannot be paused. NZBGet takes pause and resume but not the other row controls. */
export function pausable(r: { protocol?: string; phase?: string; downloader_kind?: string }): boolean {
	return (controllable(r.downloader_kind) || r.downloader_kind === 'nzbget') && !(r.protocol === 'usenet' && (r.phase === 'done' || r.phase === 'failed'));
}

/** Whether the Downloader kind takes pause, resume, retry and the other row controls; Transmission, Deluge and NZBGet do not yet. An unknown kind is allowed and the server answers. */
export function controllable(kind?: string): boolean {
	return !kind || kind === 'qbittorrent' || kind === 'sabnzbd';
}

/** The words for a Usenet job's step after the download, from client_hint. */
export const STEP_WORD: Record<string, string> = {
	get verifying() { return t('downloads.step.verifying'); },
	get repairing() { return t('downloads.step.repairing'); },
	get unpacking() { return t('downloads.step.unpacking'); },
	get finishing() { return t('downloads.step.moving'); },
	get script() { return t('downloads.step.script'); }
};

// The Repair tab's stages in order, with our headings.
const REPAIR: [string, string][] = [['download', 'download'], ['repair', 'repair'], ['join', 'join'], ['unpack', 'unpack'], ['rename', 'rename'], ['move', 'move'], ['script', 'script']];

/** The post-download stages that have lines. */
export function repairSteps(v: UsenetView | undefined): { title: string; lines: string[] }[] {
	if (!v) return [];
	const out: { title: string; lines: string[] }[] = [];
	for (const [key, title] of REPAIR) {
		const lines = v.stages.filter((s) => s.stage === key).flatMap((s) => s.lines);
		if (lines.length) out.push({ title: t(`downloads.repair.${title}`), lines });
	}
	return out;
}

/** Joins this job's per-server amounts with each server's totals. */
export function serverRows(v: UsenetView | undefined): { name: string; job: string; total: number; found: string }[] {
	if (!v) return [];
	const stats = new Map((v.servers?.servers ?? []).map((s) => [s.name, s]));
	const rows = v.job_servers.map((j) => {
		const s = stats.get(j.name);
		stats.delete(j.name);
		return { name: j.name, job: j.amount, total: s?.total ?? 0, found: s ? foundWord(s) : '' };
	});
	// Servers that sent nothing for this job still show their totals.
	for (const s of stats.values()) rows.push({ name: s.name, job: '', total: s.total, found: foundWord(s) });
	return rows;
}

function foundWord(s: ServerStat): string {
	if (!s.articles_tried) return '';
	return `${Math.round((s.articles_found / s.articles_tried) * 100)}%`;
}
