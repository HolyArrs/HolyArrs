// Leftover files of a title: videos in its folder that fill no listed episode.

export type Leftover = {
	file_id: number;
	relative_path: string;
	size: number;
	resolution?: number;
	reason: string;
	dropped_season?: number;
	guess?: { season: number; episodes: number[] };
	dismissed: boolean;
};

// One listed episode as the match sheet sees it.
export type MatchEpisode = { id: number; season: number; episode: number; name?: string; hasFile: boolean };

export type ShiftPair = { file_id: number; episode_id: number; from: [number, number]; to: [number, number] };

export type ShiftGuess = { from: number; to: number; shift: number; pairs: ShiftPair[] };

// The message key for a reason; an unknown key falls back to the general one.
const REASONS = new Set([
	'no_episode_numbers',
	'whole_season',
	'episode_not_listed',
	'bigger_file_same_episode',
	'episode_has_file',
	'movie_more_than_one',
	'movie_copy_choice',
	'movie_has_file',
	'replaced_by_hand',
	'unmatched'
]);

export function reasonKey(l: Pick<Leftover, 'reason' | 'dropped_season'>): string {
	if (l.reason === 'episode_not_listed' && l.dropped_season != null && l.dropped_season >= 0) return 'discover.leftovers.reason.season_not_listed';
	return `discover.leftovers.reason.${REASONS.has(l.reason) ? l.reason : 'episode_not_listed'}`;
}

// shiftGuess finds the one season move the file names suggest: the
// leftovers of an unlisted season land, in order, on a run of empty episodes
// of another season (2x12 -> 1x24 gives Season 2 -> Season 1, +12). It is
// only offered when exactly one target fits every file; a reorder inside
// one season gives no guess.
export function shiftGuess(leftovers: Leftover[], episodes: MatchEpisode[]): ShiftGuess | null {
	const bySeason = new Map<number, { file_id: number; e: number }[]>();
	const listed = new Set(episodes.map((e) => `${e.season}x${e.episode}`));
	for (const l of leftovers) {
		if (l.dismissed || !l.guess || l.guess.episodes.length !== 1) continue;
		// A file unmatched by hand (an undone shift) counts while its episode is still off the list.
		const off = l.reason === 'episode_not_listed' || (l.reason === 'unmatched' && !listed.has(`${l.guess.season}x${l.guess.episodes[0]}`));
		if (!off) continue;
		const list = bySeason.get(l.guess.season) ?? [];
		list.push({ file_id: l.file_id, e: l.guess.episodes[0] });
		bySeason.set(l.guess.season, list);
	}
	let best: ShiftGuess | null = null;
	let fits = 0;
	for (const [from, files] of bySeason) {
		if (files.length < 2) continue;
		files.sort((a, b) => a.e - b.e);
		const seasons = new Set(episodes.map((e) => e.season));
		for (const to of seasons) {
			if (to === from) continue;
			const empty = new Map(episodes.filter((e) => e.season === to && !e.hasFile).map((e) => [e.episode, e]));
			if (!empty.size) continue;
			const shift = Math.min(...empty.keys()) - files[0].e;
			const pairs: ShiftPair[] = [];
			for (const f of files) {
				const ep = empty.get(f.e + shift);
				if (!ep) break;
				pairs.push({ file_id: f.file_id, episode_id: ep.id, from: [from, f.e], to: [to, f.e + shift] });
			}
			if (pairs.length !== files.length) continue;
			fits++;
			best = { from, to, shift, pairs };
		}
	}
	return fits === 1 ? best : null;
}

export function fileName(p: string): string {
	return p.slice(p.lastIndexOf('/') + 1);
}
