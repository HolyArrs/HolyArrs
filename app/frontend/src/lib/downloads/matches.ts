// The Confirm match list: torrents in a Downloader that HolyArrs did not send and could not match to a title for sure. Pure helpers.

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
	return n === 1 ? '1 torrent needs a match' : `${n} torrents need a match`;
}

/** A choice's small line: kind and year. */
export function candidateLine(c: Pick<MatchCandidate, 'kind' | 'year'>): string {
	const k = c.kind === 'show' ? 'TV show' : 'Movie';
	return c.year ? `${k} · ${c.year}` : k;
}
