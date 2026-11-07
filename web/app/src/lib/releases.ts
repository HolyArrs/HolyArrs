// Words for Releases, their verdicts and searches, shared by the Movie
// page, the Held and Turned down lists and Home.
import type { Reason, ReleaseView, SearchView } from './types.ts';

const outcomeWords: Record<string, string> = {
	chosen: 'Chosen',
	held: 'Held',
	passed_over: 'Passed over',
	refused: 'Turned away by the rules',
	turned_down: 'Turned down'
};

/** verdict names what happened to a Release, in the words of the screens. */
export function verdict(r: Pick<ReleaseView, 'state' | 'outcome'>): string {
	if (r.state === 'turned_down') return outcomeWords.turned_down;
	if (r.state === 'chosen') return outcomeWords.chosen;
	if (r.state === 'held') return outcomeWords.held;
	// A Choose a release search only judges: its best one was not sent.
	if (r.outcome === 'chosen') return 'Would be chosen';
	if (r.outcome === 'held') return 'Would be held';
	return outcomeWords[r.outcome] ?? outcomeWords[r.state] ?? 'Judged';
}

/** verdictTone colours the verdict chip. */
export function verdictTone(r: Pick<ReleaseView, 'state' | 'outcome'>): 'red' | 'ok' | 'wait' | 'fail' | 'idle' {
	if (r.state === 'chosen') return 'red';
	if (r.state === 'turned_down' || r.outcome === 'refused') return 'fail';
	if (r.outcome === 'chosen') return 'ok';
	if (r.outcome === 'held' || r.state === 'held') return 'wait';
	return 'idle';
}

/** keyReasons are the lines that decided the verdict: why it failed, lost or waits. */
export function keyReasons(reasons: Reason[]): Reason[] {
	return reasons.filter((x) => x.effect === 'fail' || x.effect === 'rank' || x.effect === 'hold' || x.effect === 'warn');
}

/** facts is the short line under a Release name. */
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

/** chooseWarnings says, before choosing, what the person should know. show
 * says the Release is for a show's episodes. */
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

/** listedName names what a listed Release is for: "Copper Valley S01E02",
 * or the movie's name. */
export function listedName(r: Pick<ReleaseView, 'title_name' | 'episode' | 'turned_down_for' | 'state' | 'album'>): string {
	if (r.album) return [r.title_name, r.album].filter(Boolean).join(' – ');
	const ep = r.state === 'turned_down' && r.turned_down_for ? r.turned_down_for : r.episode;
	return [r.title_name, ep].filter(Boolean).join(' ');
}

export interface Pager {
	from: number;
	to: number;
	prev: number | null;
	next: number | null;
}

/** pager works out the "1 to 25 of 60" line and the offsets of the pages around it. */
export function pager(total: number, limit: number, offset: number): Pager {
	if (total <= 0) return { from: 0, to: 0, prev: null, next: null };
	const from = Math.min(offset, total - 1) + 1;
	const to = Math.min(offset + limit, total);
	return { from, to, prev: offset > 0 ? Math.max(0, offset - limit) : null, next: to < total ? offset + limit : null };
}

/** searchLine says what a search is doing or found, for Home. */
export function searchLine(s: SearchView): string {
	if (s.kind === 'feed_check') return s.state === 'done' ? 'Feeds checked' : 'Checking the feeds of every Source';
	if (s.kind === 'search_missing')
		return s.state === 'done' ? 'Searches queued for everything missing' : 'Searching for every movie, episode and album still looking';
	const name = s.title_name
		? s.album
			? `${s.title_name} – ${s.album}`
			: `${s.title_name}${s.episode ? ` ${s.episode}` : ''}`
		: s.kind === 'search_season'
			? 'a season'
			: s.kind === 'search_album'
				? 'an album'
				: 'a movie';
	const what = `${name}${s.title_name && s.version && s.version !== 'Main' ? ` (${s.version})` : ''}`;
	if (s.outcome === 'episodes' && s.state === 'done') {
		return s.found ? `${what}: found ${s.found} ${s.found === 1 ? 'Release' : 'Releases'}, searching the rest episode by episode` : `${what}: searching episode by episode`;
	}
	if (s.outcome === 'album' && s.state === 'done') return `${what}: searching the whole album`;
	if (s.state === 'queued') return `Waiting to search for ${what}`;
	if (s.state === 'running') return `Searching for ${what}${s.note ? `: ${s.note}` : ''}`;
	if (s.state === 'failed') return `Search for ${what} failed${s.error ? `: ${s.error}` : ''}`;
	const n = `${s.found} ${s.found === 1 ? 'Release' : 'Releases'}`;
	const refused = s.refused ? `, ${s.refused} turned away` : '';
	switch (s.outcome) {
		case 'chosen':
			return `${what}: found ${n}${refused}, chose ${s.picked || 'one'}`;
		case 'held':
			return `${what}: found ${n}${refused}, holding ${s.picked || 'one'}`;
		case 'shown':
			return `${what}: found ${n}${refused}, ready to choose from`;
		default:
			return `${what}: found ${n}${refused}, none passed the Pick rules`;
	}
}

/** feedMinutesOk says whether HolyArrs takes this feed check interval: 0 (off) or 10 to 120. */
export function feedMinutesOk(n: number): boolean {
	return Number.isInteger(n) && (n === 0 || (n >= 10 && n <= 120));
}
