import type { AskAlbum, EpisodeSlot, AskState } from '../core/types.ts';

export interface PressingTrack {
	id: string;
	recording_id: string;
	disc: number;
	number: number;
	position: number;
	name: string;
	length_ms?: number;
}

/** PressingView is one Pressing (edition) of an album with its tracks. */
export interface PressingView {
	id: number;
	mbid: string;
	name: string;
	status: string;
	country?: string;
	date?: string;
	disambiguation?: string;
	formats: string[];
	track_count: number;
	chosen: boolean;
	chosen_by?: string;
	tracks: PressingTrack[];
}

export interface TrackView {
	id: number;
	disc: number;
	number: number;
	name?: string;
	length_ms?: number;
	recording_id?: string;
	release_date?: string;
	keep_up_to_date: boolean;
	dropped?: boolean;
	ids: Record<string, string>;
	slots: EpisodeSlot[];
}

/** AskableAlbum is an album of a looked-up artist as the ask dialog shows it. */
export interface AskableAlbum {
	mbid: string;
	name: string;
	type: string;
	secondary_types: string[];
	release_date?: string;
	kept: boolean;
	left_out: boolean;
}

/** LookupArtist is a MusicBrainz artist with what the Library knows of it. */
export interface LookupArtist {
	mbid: string;
	name: string;
	sort_name?: string;
	type?: string;
	disambiguation?: string;
	country?: string;
	status: string;
	score?: number;
	/** the artist's picture (music picture route) */
	image?: string;
	title_id?: number;
	never_add?: boolean;
	ask?: { id: number; state: AskState; state_label: string; mine: boolean; people: number; albums?: AskAlbum[] };
	ask_albums?: AskableAlbum[];
}

/** MusicSettings are the add-an-artist defaults and MusicBrainz settings. */
export interface MusicSettings {
	album_choice: string;
	new_albums: string;
	album_types: string[];
	secondary_types: string[];
	release_statuses: string[];
	pressing_rule: string;
	musicbrainz_url: string;
	cache_hours: number;
}
