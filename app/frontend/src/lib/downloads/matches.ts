// The Confirm match list: downloads in a Downloader that HolyArrs did not send and could not match to a title for sure. Pure helpers.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export interface MatchCandidate {
	id: string;
	kind: 'movie' | 'show';
	name: string;
	year?: number;
	poster?: string;
	score: number;
}

export interface Match {
	id: number;
	downloader_id: number;
	hash: string;
	name: string;
	state: string;
	title_id?: number;
	candidates: MatchCandidate[];
}

export interface Matches {
	matches: Match[];
	counts: Record<string, number>;
	pending: number;
}

/** The Downloads banner line; '' when nothing waits. */
export function bannerWords(n: number): string {
	if (n <= 0) return '';
	return n === 1 ? t('downloads.match.one') : t('downloads.match.many', { n });
}

/** A choice's small line: kind and year. */
export function candidateLine(c: Pick<MatchCandidate, 'kind' | 'year'>): string {
	const k = c.kind === 'show' ? t('downloads.kind.show') : t('downloads.kind.movie');
	return c.year ? `${k} · ${c.year}` : k;
}
