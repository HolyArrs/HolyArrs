// Sizes in plain units with 1000-based steps: 999 KB, 1 MB, 1.7 GB, 18 GB, 1 TB.

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];

/** size writes bytes as "553 MB", "1.7 GB", "18 GB" (one decimal under 10, none above). */
export function size(n: number | null | undefined): string {
	if (!n || n < 0 || !Number.isFinite(n)) return '0 B';
	let v = n;
	let i = 0;
	while (i < UNITS.length - 1 && Math.round(v * 10) / 10 >= 1000) {
		v /= 1000;
		i++;
	}
	const t = i === 0 || v >= 10 ? String(Math.round(v)) : String(Math.round(v * 10) / 10);
	// Rounding 999.6 up gives 1000: step to the next unit.
	if (t === '1000' && i < UNITS.length - 1) return `1 ${UNITS[i + 1]}`;
	return `${t} ${UNITS[i]}`;
}

/** sizePair is a part of a whole: "553 MB / 3 GB". */
export function sizePair(got: number, total: number): string {
	return `${size(got)} / ${size(total)}`;
}

/** protocolOf names a Downloader's protocol from what the row carries. */
export function protocolOf(r: { protocol?: string; downloader_kind?: string }): 'torrent' | 'usenet' | '' {
	const p = (r.protocol || '').toLowerCase();
	if (p === 'torrent' || p === 'usenet') return p;
	const k = (r.downloader_kind || '').toLowerCase();
	if (k === 'sabnzbd' || k === 'nzbget') return 'usenet';
	if (k === 'qbittorrent' || k === 'transmission' || k === 'deluge') return 'torrent';
	return '';
}

export type KindKey = '' | 'movie' | 'show' | 'artist';
export const KIND_WORD: Record<Exclude<KindKey, ''>, string> = { movie: 'Movie', show: 'TV show', artist: 'Music' };
export const KIND_TABS: { key: KindKey; label: string }[] = [
	{ key: '', label: 'All' },
	{ key: 'movie', label: 'Movies' },
	{ key: 'show', label: 'TV shows' },
	{ key: 'artist', label: 'Music' }
];
