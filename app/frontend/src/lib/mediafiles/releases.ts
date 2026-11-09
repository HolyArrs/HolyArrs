// Plain-words labels, warnings and paging for the Releases list and search status lines.
import type { Reason, ReleaseView } from '../core/types.ts';

export interface Pager {
	from: number;
	to: number;
	prev: number | null;
	next: number | null;
}

export interface ReleaseScope {
	// Heading shown for what is being searched.
	label: string;
	// Search text sent to the sources.
	query: string;
	// Set when the search is for one slot only.
	slotId?: number;
	slotState?: string;
}

const outcomeWords: Record<string, string> = {
	chosen: 'Chosen',
	held: 'Held',
	passed_over: 'Passed over',
	refused: 'Turned away by the rules',
	turned_down: 'Turned down'
};

// A real state beats the judged outcome; an outcome alone is only what "would" happen.
export function verdict(r: Pick<ReleaseView, 'state' | 'outcome'>): string {
	if (r.state === 'turned_down') return outcomeWords.turned_down;
	if (r.state === 'chosen') return outcomeWords.chosen;
	if (r.state === 'held') return outcomeWords.held;
	if (r.outcome === 'chosen') return 'Would be chosen';
	if (r.outcome === 'held') return 'Would be held';
	return outcomeWords[r.outcome] ?? outcomeWords[r.state] ?? 'Judged';
}

export function verdictTone(r: Pick<ReleaseView, 'state' | 'outcome'>): 'red' | 'ok' | 'wait' | 'fail' | 'idle' {
	if (r.state === 'chosen') return 'red';
	if (r.state === 'turned_down' || r.outcome === 'refused') return 'fail';
	if (r.outcome === 'chosen') return 'ok';
	if (r.outcome === 'held' || r.state === 'held') return 'wait';
	return 'idle';
}

// Only reasons that changed the result are worth showing up front.
export function keyReasons(reasons: Reason[]): Reason[] {
	return reasons.filter((x) => x.effect === 'fail' || x.effect === 'rank' || x.effect === 'hold' || x.effect === 'warn');
}

export function facts(r: ReleaseView, size: (n: number) => string): string {
	return [
		r.source_name,
		size(r.size),
		r.seeders != null ? `${r.seeders} seeders` : '',
		r.protocol,
		r.passed ? `${r.points} points` : '',
		r.passed && r.rank ? `ranked ${r.rank}` : ''
	]
		.filter(Boolean)
		.join(' · ');
}

// Tells the user what choosing this release overrides, worded for a movie, show or album.
export function chooseWarnings(r: ReleaseView, slotState?: string, show = false, music = false): string[] {
	const out: string[] = [];
	if (r.needs_force) {
		const fails = r.reasons.filter((x) => x.effect === 'fail').map((x) => x.text);
		out.push(`It did not pass: ${fails.length ? fails.join('; ') : 'it was refused'}. Choosing it overrides that.`);
	}
	if (music) {
		if (r.gate_skipped || slotState === 'not_out_yet') out.push('This album is not out yet; choosing it skips that check.');
		const n = r.slot_ids?.length ?? 0;
		if (n > 1) out.push(`One download fills ${n} tracks${r.album ? ` of ${r.album}` : ''}.`);
		return out;
	}
	if (show) {
		if (r.gate_skipped || slotState === 'not_out_yet') out.push('This episode has not aired yet; choosing it skips that check.');
		if (slotState === 'in_library') out.push('This episode already has a good enough copy; the new one will replace it.');
		if ((r.slot_ids?.length ?? 0) > 1 && r.episode) out.push(`One download fills ${r.episode}.`);
		return out;
	}
	if (r.gate_skipped || slotState === 'not_out_yet') out.push('The Release gate is skipped: this movie does not count as out yet.');
	if (slotState === 'in_library') out.push('This Version already has a good enough copy; the new one will replace it.');
	return out;
}

export function listedName(r: Pick<ReleaseView, 'title_name' | 'episode' | 'turned_down_for' | 'state' | 'album'>): string {
	if (r.album) return [r.title_name, r.album].filter(Boolean).join(' – ');
	const ep = r.state === 'turned_down' && r.turned_down_for ? r.turned_down_for : r.episode;
	return [r.title_name, ep].filter(Boolean).join(' ');
}

// Shows "from to total" bounds, clamping an offset that ran past the end.
export function pager(total: number, limit: number, offset: number): Pager {
	if (total <= 0) return { from: 0, to: 0, prev: null, next: null };
	const from = Math.min(offset, total - 1) + 1;
	const to = Math.min(offset + limit, total);
	return { from, to, prev: offset > 0 ? Math.max(0, offset - limit) : null, next: to < total ? offset + limit : null };
}

// 0 turns feed checks off; otherwise 10 to 120 minutes.
export function feedMinutesOk(n: number): boolean {
	return Number.isInteger(n) && (n === 0 || (n >= 10 && n <= 120));
}
