// Episode numbering shared by the show screens and the subtitle words.

/** episodeCode is "S01E02". */
export function episodeCode(season: number, episode: number): string {
	const two = (n: number) => String(n).padStart(2, '0');
	return `S${two(season)}E${two(episode)}`;
}
