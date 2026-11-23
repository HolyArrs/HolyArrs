// The file points answer and the lines that say why a file is short of good enough.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export interface FilePointsView {
	step: string;
	good_enough_at: string;
	step_reached: boolean;
	points: number;
	good_enough_points: number | null;
	good_enough: boolean;
	meets_version: boolean;
	traits: { trait: string; points: number }[];
	names: Record<string, string>;
}

/** Why the file is short of good enough, one line per reason. */
export function filePointsWhy(fp: FilePointsView, step: (s: string) => string): string[] {
	const out: string[] = [];
	if (!fp.step_reached) out.push(t('downloads.filePoints.stepBelow', { have: fp.step ? step(fp.step) : t('downloads.filePoints.unknownStep'), need: step(fp.good_enough_at) }));
	if (fp.good_enough_points != null && fp.points < fp.good_enough_points) out.push(t('downloads.filePoints.pointsBelow', { n: fp.good_enough_points - fp.points }));
	if (!fp.meets_version) out.push(t('downloads.filePoints.version'));
	return out;
}
