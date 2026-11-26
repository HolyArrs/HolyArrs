// An artist's unmatched files: found music that fills no track, with why and where it came closest.
export type MusicLeftover = {
	file_id: number;
	relative_path: string;
	size: number;
	reason: string;
	dismissed: boolean;
	music?: { album_id?: number; album_name?: string; track_id?: number; owner?: string };
};

export const fileName = (p: string) => p.slice(p.lastIndexOf('/') + 1);

// Another pressing of an album that the unmatched files fit better; only ever offered.
export type PressingHint = { album_id: number; album_name: string; pressing_id: number; pressing_name: string; track_count: number; fits: number; chosen: number; files: number };

const MUSIC = new Set(['music_no_album', 'music_two_album', 'music_album_off', 'music_no_track', 'music_unsure', 'music_has_file', 'music_duplicate', 'music_other_artist', 'music_not_listed']);
const BY_HAND = new Set(['unmatched', 'replaced_by_hand']);

/** The message key for why a music file fills no track; an unknown key falls back to "no album fits". */
export function unmatchedReason(reason: string): string {
	if (BY_HAND.has(reason)) return `discover.leftovers.reason.${reason}`;
	return `music.unmatched.reason.${MUSIC.has(reason) ? reason : 'music_no_album'}`;
}
