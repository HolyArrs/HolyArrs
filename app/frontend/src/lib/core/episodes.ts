// Episode label like S01E02.
export function episodeCode(season: number, episode: number): string {
	const two = (n: number) => String(n).padStart(2, '0');
	return `S${two(season)}E${two(episode)}`;
}

/** An episode label with its name for a second line: "S01E02 · Flood", or the label alone while the name is unknown. "TBA" names are left out. */
export function episodeNamed(label: string, name?: string | null): string {
	const n = name?.trim();
	return n && n.toUpperCase() !== 'TBA' ? `${label} · ${n}` : label;
}

/** episodeNamed for a season and episode number ("E02" when the season is unknown). */
export function episodeWords(season: number | undefined | null, episode: number, name?: string | null): string {
	return episodeNamed(season == null ? `E${String(episode).padStart(2, '0')}` : episodeCode(season, episode), name);
}
