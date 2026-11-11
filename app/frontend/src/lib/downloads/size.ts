// Sizes in plain units with 1000-based steps: 999 KB, 1 MB, 1.7 GB, 18 GB, 1 TB.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type KindKey = '' | 'movie' | 'show' | 'artist';

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];

/** Bytes as "553 MB", "1.7 GB", "18 GB" (one decimal under 10, none above). */
export function size(n: number | null | undefined): string {
	if (!n || n < 0 || !Number.isFinite(n)) return '0 B';
	let v = n;
	let i = 0;
	while (i < UNITS.length - 1 && Math.round(v * 10) / 10 >= 1000) {
		v /= 1000;
		i++;
	}
	const t = i === 0 || v >= 10 ? String(Math.round(v)) : String(Math.round(v * 10) / 10);
	// Rounding 999.6 up gives 1000, so step to the next unit.
	if (t === '1000' && i < UNITS.length - 1) return `1 ${UNITS[i + 1]}`;
	return `${t} ${UNITS[i]}`;
}

/** Free disk space as the Downloaders show it: 1024-based steps, one decimal from GB up ("14.2 TB", "512.3 GB"). */
export function freeSize(n: number | null | undefined): string {
	if (!n || n < 0 || !Number.isFinite(n)) return '0 B';
	let v = n;
	let i = 0;
	// Step on the shown (rounded) value, so 1023.96 GB reads 1.0 TB, not 1024.0 GB.
	while (i < UNITS.length - 1 && (i < 3 ? Math.round(v) : Math.round(v * 10) / 10) >= 1024) {
		v /= 1024;
		i++;
	}
	return i < 3 ? `${Math.round(v)} ${UNITS[i]}` : `${(Math.round(v * 10) / 10).toFixed(1)} ${UNITS[i]}`;
}

/** A Downloader's protocol from what the row carries. */
export function protocolOf(r: { protocol?: string; downloader_kind?: string }): 'torrent' | 'usenet' | '' {
	const p = (r.protocol || '').toLowerCase();
	if (p === 'torrent' || p === 'usenet') return p;
	const k = (r.downloader_kind || '').toLowerCase();
	if (k === 'sabnzbd' || k === 'nzbget') return 'usenet';
	if (k === 'qbittorrent' || k === 'transmission' || k === 'deluge' || k === 'rtorrent') return 'torrent';
	return '';
}

// Getters, so each word follows the language picked when it is read.
export const KIND_WORD: Record<Exclude<KindKey, ''>, string> = {
	get movie() { return t('downloads.kind.movie'); },
	get show() { return t('downloads.kind.show'); },
	get artist() { return t('downloads.kind.music'); }
};

export const KIND_TABS: { key: KindKey; label: string }[] = [
	{ key: '', get label() { return t('downloads.kind.all'); } },
	{ key: 'movie', get label() { return t('downloads.kind.movies'); } },
	{ key: 'show', get label() { return t('downloads.kind.shows'); } },
	{ key: 'artist', get label() { return t('downloads.kind.music'); } }
];

/** A client category in plain words: tv_shows reads TV show, movies reads Movie, anything else loses its underscores. */
export function categoryWords(c: string | undefined): string {
	const v = (c ?? '').trim();
	if (!v) return '';
	const low = v.toLowerCase();
	if (/(^|[^a-z])(tv|shows?|series|sonarr)([^a-z]|$)/.test(low.replace(/_/g, ' '))) return t('downloads.kind.show');
	if (/(^|[^a-z])(movies?|films?|radarr)([^a-z]|$)/.test(low.replace(/_/g, ' '))) return t('downloads.kind.movie');
	const words = v.replace(/[_-]+/g, ' ').replace(/\s+/g, ' ').trim();
	return words.charAt(0).toUpperCase() + words.slice(1);
}
