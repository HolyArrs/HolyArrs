import type { Component } from 'svelte';
import type { Ask, AskAlbum, EpisodeSlot, AskState, LibraryFolder, PickRulesView, TitleView } from '../core/types.ts';

export interface PressingTrack {
	id: string;
	recording_id: string;
	disc: number;
	number: number;
	position: number;
	name: string;
	length_ms?: number;
}

/** One Pressing (edition) of an album with its tracks. */
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

/** An album of a looked-up artist as the ask dialog shows it. */
export interface AskableAlbum {
	mbid: string;
	name: string;
	type: string;
	secondary_types: string[];
	release_date?: string;
	kept: boolean;
	left_out: boolean;
}

/** A MusicBrainz artist with what the Library knows of it. */
export interface LookupArtist {
	mbid: string;
	name: string;
	sort_name?: string;
	type?: string;
	disambiguation?: string;
	country?: string;
	status: string;
	score?: number;
	/** The artist's picture (music picture route). */
	image?: string;
	title_id?: number;
	never_add?: boolean;
	ask?: { id: number; state: AskState; state_label: string; mine: boolean; people: number; albums?: AskAlbum[] };
	ask_albums?: AskableAlbum[];
}

/** The add-an-artist defaults and MusicBrainz settings. */
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

/** The ask and add dialogs, handed in by the page because a music part cannot import requests. */
export interface ArtistSheets {
	ask: Component<
		{
			onasked: (artist: LookupArtist, r: { ask: Ask; joined: boolean; already: boolean }) => void;
			onrefused?: (artist: LookupArtist, body: Record<string, unknown>) => void;
		},
		{ open: (a: LookupArtist, album?: string) => Promise<void> }
	>;
	add: Component<
		{
			folders: LibraryFolder[];
			rules: PickRulesView[];
			defaults: MusicSettings | null;
			onadded: (artist: LookupArtist, title: TitleView) => void;
		},
		{ open: (a: LookupArtist) => void }
	>;
}
