// Picks the few words the Status cell shows after the status word when a download is at risk; the tooltip keeps the full line.
import { protocolOf } from './size.ts';
import { partialPack, stageOf } from './status-tabs.ts';
import type { DownloadRow } from '../core/types.ts';

export type Problem = { text: string; tone: 'fail' | 'wait' };

type Bits = Pick<DownloadRow, 'phase' | 'state' | 'progress' | 'text'> &
	Partial<Pick<DownloadRow, 'protocol' | 'downloader_kind' | 'tracker_problem' | 'availability' | 'seeds' | 'peers' | 'client_hint' | 'replaced' | 'pack'>>;

/** The message key that shows each problem's words in the picked language. */
export const PROBLEM_KEY: Record<string, string> = {
	'import blocked': 'downloads.problem.importBlocked',
	'missing articles': 'downloads.problem.missingArticles',
	'no full copy': 'downloads.problem.noFullCopy',
	'no peers': 'downloads.problem.noPeers',
	'repair needed': 'downloads.problem.repairNeeded',
	repairing: 'downloads.step.repairing',
	'tracker error': 'downloads.tracker.error',
	'tracker warning': 'downloads.tracker.warning',
	'no tracker reply': 'downloads.tracker.announce'
};

/** The row's visible reason, or null when nothing is wrong. */
export function problem(r: Bits): Problem | null {
	const stage = stageOf(r);
	if (stage === 'done' || stage === 'paused') return null;
	const failed = stage === 'failed';
	if (protocolOf(r) === 'usenet') {
		// SABnzbd and NZBGet fail or warning lines, already in plain words.
		const t = r.text ?? '';
		if (/article|missing|incomplete/i.test(t)) return { text: 'missing articles', tone: failed ? 'fail' : 'wait' };
		// A healthy job also says Verifying or Repairing; only a failed or failing line counts.
		if ((failed || /fail|error|need|unable|cannot/i.test(t)) && /repair|par2?\b|verif/i.test(t)) return { text: 'repair needed', tone: failed ? 'fail' : 'wait' };
		if (r.client_hint === 'repairing') return { text: 'repairing', tone: 'wait' };
	}
	if (r.phase === 'import_refused' || stage === 'decide') return { text: 'import blocked', tone: 'fail' };
	if (r.tracker_problem === 'error') return { text: 'tracker error', tone: 'fail' };
	if (r.tracker_problem === 'announce') return { text: 'no tracker reply', tone: 'wait' };
	// A season pack taken a few episodes at a time leaves the other files unwanted on purpose, so copies and peers say nothing.
	const getting = (stage === 'downloading' || stage === 'stalled') && !partialPack(r);
	if (getting && typeof r.availability === 'number' && r.availability < 1) return { text: 'no full copy', tone: stage === 'stalled' ? 'fail' : 'wait' };
	if (stage === 'stalled' && !partialPack(r) && protocolOf(r) === 'torrent' && !(r.seeds ?? 0) && !(r.peers ?? 0)) return { text: 'no peers', tone: 'wait' };
	if (r.tracker_problem === 'warning') return { text: 'tracker warning', tone: 'wait' };
	return null;
}
