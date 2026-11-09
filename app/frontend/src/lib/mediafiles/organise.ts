// Types and helpers for naming, adopting and removing files in the Library.

export interface RenameStep {
	file_id: number;
	title_id: number;
	title_name: string;
	origin: string;
	from: string;
	to?: string;
	skip?: string;
	album_id?: number;
	subtitles?: { from: string; to?: string; skip?: string }[];
}

// A background job as the server reports it.
export interface JobView {
	ID: number;
	State: 'queued' | 'running' | 'done' | 'failed' | 'cancelled';
	Progress: number;
	ProgressNote?: string | null;
	Result?: Record<string, unknown> | null;
	Error?: { code: string; message: string } | null;
}

// One possible title for a folder.
export interface AdoptMatch {
	id?: string;
	tmdb_id: string;
	name: string;
	year: number;
	score: number;
}

export type AdoptStatus = 'matched' | 'unsure' | 'unmatched' | 'no_video' | 'in_library' | 'never_add';

export interface AdoptCandidate {
	folder: string;
	parsed_name: string;
	parsed_year?: number;
	videos: { path: string; size: number }[];
	main?: string;
	match?: AdoptMatch;
	others: AdoptMatch[];
	status: AdoptStatus;
	why: string;
	title_id?: number;
	files?: AdoptEpisodeFile[];
	episodes?: number;
	unknown?: number;
	albums?: AdoptAlbum[];
	tracks?: number;
	cut?: boolean;
}

export interface AdoptTrackFile {
	path: string;
	size: number;
	track?: string;
	why?: string;
}

export interface AdoptAlbum {
	folder: string;
	parsed_name: string;
	parsed_year?: number;
	match?: AdoptMatch;
	pressing?: string;
	tracks?: number;
	files: AdoptTrackFile[];
	matched: number;
	unknown: number;
	cut?: boolean;
	status: 'matched' | 'unsure' | 'unmatched' | 'not_included';
	why: string;
}

export interface AdoptEpisodeFile {
	path: string;
	size: number;
	episodes: string[];
	why?: string;
}

export interface AdoptPlan {
	kind?: 'movie' | 'show' | 'music';
	warning?: string;
	library_folder_id: number;
	path: string;
	folders: AdoptCandidate[];
	counts: Record<string, number>;
	limit: number;
	more: boolean;
	next?: string;
	partial?: boolean;
}

export interface AdoptResult {
	folder: string;
	title_id?: number;
	file?: string;
	error?: string;
	episodes?: number;
	unknown?: string[];
}

export interface RemoveFile {
	file_id: number;
	origin: string;
	path: string;
	to?: string;
	why: string;
	episodes?: string[];
}

export interface RemovePlan {
	title_id: number;
	name: string;
	scope?: 'title' | 'season' | 'episodes' | 'album' | 'tracks';
	season?: number;
	episodes?: string[];
	album_id?: number;
	album?: string;
	tracks?: string[];
	choice: { delete_files: boolean; never_add: boolean };
	applied: boolean;
	held: RemoveFile[];
	kept: RemoveFile[];
	holding_days: number;
	active_downloads: number;
}

export interface NamingSettings {
	file_pattern: string;
	folder_pattern: string;
	colon: string;
	illegal: string;
	rename_on_import: boolean;
	adopt_scan_limit: number;
	music_adopt_scan_limit?: number;
	show_folder_pattern?: string;
	season_folders?: boolean;
	season_folder_pattern?: string;
	specials_folder?: string;
	episode_pattern?: string;
	daily_pattern?: string;
	anime_pattern?: string;
	multi_episode_style?: string;
	air_date_style?: string;
	absolute_digits?: number;
	artist_folder_pattern?: string;
	album_folder_pattern?: string;
	track_pattern?: string;
	multi_disc_track_pattern?: string;
	disc_folders?: boolean;
	disc_folder_pattern?: string;
	music_read_tags?: boolean;
}

export interface MusicExamples {
	artist_folder?: string;
	album_folder?: string;
	track?: string;
	multi_disc?: string;
	various?: string;
}

export interface EpisodeExamples {
	show_folder?: string;
	episode?: string;
	multi?: string;
	daily?: string;
	anime?: string;
	special?: string;
}

export interface NamingView {
	naming: NamingSettings;
	words: Record<string, string>;
	example: string;
	example_folder: string;
	example_episodes?: EpisodeExamples;
	example_music?: MusicExamples;
}

export interface FileRules {
	holding_days: number;
	min_free_mb: number;
	skip_free_space_check: boolean;
}

export interface DeleteScope {
	season?: number;
	episodeIds?: number[];
	albumId?: number;
	trackIds?: number[];
	label?: string;
}

export const musicWords = ['artist', 'artist_sort', 'album', 'album_type', 'disc', 'track', 'track_title', 'track_artist', 'format'];

export const showWords = ['season', 'season_padded', 'episode', 'episode_title', 'air_date', 'absolute'];

export const colonChoices: [string, string][] = [
	['smart', 'Smart: a colon and space become " - ", any other colon "-"'],
	['dash', 'A dash: "-"'],
	['space_dash', 'A space and a dash: " -"'],
	['space_dash_space', 'A dash with spaces: " - "'],
	['remove', 'Drop the colon']
];

export const multiEpisodeChoices: [string, string][] = [
	['range', 'First and last: S01E02-E04'],
	['short_range', 'First and last, short: S01E02-04'],
	['each', 'Every episode: S01E02E03E04'],
	['each_dash', 'Every episode with dashes: S01E02-E03-E04']
];

export const airDateChoices: [string, string][] = [
	['dash', 'Dashes: 2026-03-14'],
	['dot', 'Dots: 2026.03.14'],
	['space', 'Spaces: 2026 03 14']
];

export const illegalChoices: [string, string][] = [
	['replace', 'Swap them for safe ones ( / \\ * become -, " becomes \' )'],
	['remove', 'Drop them']
];

// Display order of adopt groups, with a line explaining each.
export const groupOrder: [AdoptStatus, string, string][] = [
	['matched', 'Matched', 'HolyArrs is sure which movie these are.'],
	['unsure', 'Unsure', 'A likely movie, but check it. Pick the right one or look it up.'],
	['unmatched', 'Not matched', 'No movie found from the folder name. Look each one up to add it.'],
	['in_library', 'Already in the Library', 'Left as they are.'],
	['never_add', 'On the Never-add list', 'Left as they are.'],
	['no_video', 'No video file', 'Nothing to adopt in these folders.']
];

// Skips examples the server did not send.
export function musicExampleRows(ex: MusicExamples | undefined): [string, string][] {
	if (!ex) return [];
	const order: [keyof MusicExamples, string][] = [
		['track', 'A track'],
		['multi_disc', 'A track of a two-disc album'],
		['various', 'A track of a various-artists album']
	];
	return order.filter(([k]) => !!ex[k]).map(([k, label]) => [label, ex[k] as string]);
}

export function albumSteps(steps: RenameStep[], albumId: number | undefined): RenameStep[] {
	return albumId === undefined ? steps : steps.filter((s) => s.album_id === albumId);
}

export function musicFolderLine(c: Pick<AdoptCandidate, 'tracks' | 'unknown' | 'albums'>): string {
	const t = c.tracks ?? 0;
	const a = (c.albums ?? []).filter((x) => !!x.match).length;
	const left = c.unknown ?? 0;
	const parts = [`${t} track${t === 1 ? '' : 's'} in ${a} album${a === 1 ? '' : 's'}`];
	if (left) parts.push(`${left} file${left === 1 ? '' : 's'} left alone`);
	return parts.join('; ');
}

// Says how well the album matches and how many files hold its tracks.
export function albumFolderLine(a: AdoptAlbum): string {
	const n = a.cut ? a.matched + a.unknown : a.files.length;
	if (!a.match) return `No album fits; ${n} file${n === 1 ? '' : 's'} left alone`;
	const name = `${a.match.name}${a.match.year ? ` (${a.match.year})` : ''} · ${score(a.match.score)}`;
	if (!a.pressing) return `${name} · ${a.why}`;
	return `${name} · ${a.matched} of ${n} file${n === 1 ? '' : 's'} hold a track of ${a.pressing}`;
}

// The server lists only some files; this says how many are hidden.
export function cutFilesLine(a: AdoptAlbum): string {
	const more = a.matched + a.unknown - a.files.length;
	return a.cut && more > 0 ? `and ${more} more file${more === 1 ? '' : 's'}, not listed` : '';
}

export function trackFileLine(f: AdoptTrackFile): string {
	return f.track ? f.track : `Left alone: ${f.why ?? 'no track'}`;
}

export function episodeExampleRows(ex: EpisodeExamples | undefined): [string, string][] {
	if (!ex) return [];
	const order: [keyof EpisodeExamples, string][] = [
		['episode', 'One episode'],
		['multi', 'A file holding two episodes'],
		['special', 'A special (season 0)'],
		['daily', 'A show ordered by air day'],
		['anime', 'A show ordered by running number']
	];
	return order.filter(([k]) => !!ex[k]).map(([k, label]) => [label, ex[k] as string]);
}

// Separates real renames from skipped files and ones already named right.
export function splitSteps(steps: RenameStep[]): { change: RenameStep[]; skipped: RenameStep[]; same: number } {
	const change: RenameStep[] = [];
	const skipped: RenameStep[] = [];
	let same = 0;
	for (const s of steps) {
		if (s.skip) skipped.push(s);
		else if (s.to && s.to !== s.from) change.push(s);
		else same++;
	}
	return { change, skipped, same };
}

// Drops the shared folder so only the name that changed is shown.
export function renamePair(s: Pick<RenameStep, 'from' | 'to'>): [string, string] {
	const to = s.to ?? '';
	const dir = (p: string) => p.slice(0, Math.max(0, p.lastIndexOf('/'), p.lastIndexOf('\\')));
	return dir(s.from) === dir(to) ? [fileName(s.from), fileName(to)] : [s.from, to];
}

export function fileName(p: string): string {
	const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'));
	return i < 0 ? p : p.slice(i + 1);
}

// Swaps the word movie for the shelf kind and drops empty groups.
export function groupPlan(
	folders: AdoptCandidate[],
	word = 'movie'
): { status: AdoptStatus; label: string; about: string; folders: AdoptCandidate[] }[] {
	return groupOrder
		.map(([status, label, about]) => ({
			status,
			label: status === 'no_video' && word === 'artist' ? 'No audio file' : label,
			about: about.replace('movie', word),
			folders: folders.filter((f) => f.status === status)
		}))
		.filter((g) => g.folders.length > 0);
}

export function canAdopt(f: AdoptCandidate): boolean {
	return f.status === 'matched' || f.status === 'unsure' || f.status === 'unmatched';
}

export function matchKey(m: AdoptMatch): string {
	return m.id || m.tmdb_id;
}

// Only sure matches start picked.
export function startPicks(folders: AdoptCandidate[]): Record<string, string> {
	const out: Record<string, string> = {};
	for (const f of folders) if (f.status === 'matched' && f.match) out[f.folder] = matchKey(f.match);
	return out;
}

export function choices(
	picks: Record<string, string>,
	kind: 'movie' | 'show' | 'music' = 'movie'
): ({ folder: string; tmdb_id: string } | { folder: string; id: string })[] {
	return Object.entries(picks)
		.filter(([, id]) => !!id)
		.map(([folder, id]) => (kind === 'movie' ? { folder, tmdb_id: id } : { folder, id }));
}

// Prefers the show source id, then the usual id types.
export function showIdOf(s: { source: string; ids: Record<string, string> }): string {
	for (const k of [s.source, 'tmdb', 'tvdb', 'tvmaze', 'imdb']) if (s.ids[k]) return `${k}:${s.ids[k]}`;
	return '';
}

export function episodeFileLine(f: AdoptEpisodeFile): string {
	return f.episodes.length ? f.episodes.join(', ') : `Left alone: ${f.why ?? 'no episode'}`;
}

export function showFolderLine(c: Pick<AdoptCandidate, 'episodes' | 'unknown' | 'files'>): string {
	const eps = c.episodes ?? 0;
	const files = c.files?.length ?? 0;
	const left = c.unknown ?? 0;
	const parts = [`${eps} episode${eps === 1 ? '' : 's'} in ${files - left} file${files - left === 1 ? '' : 's'}`];
	if (left) parts.push(`${left} file${left === 1 ? '' : 's'} with no episode, left alone`);
	return parts.join('; ');
}

// Turns a 0 to 1 score into a percent.
export function score(s: number): string {
	return `${Math.round(Math.max(0, Math.min(1, s)) * 100)}%`;
}

// Lists, in plain words, what a delete will do.
export function deleteSummary(p: RemovePlan, kind: 'movie' | 'show' | 'artist' = 'movie'): string[] {
	const out: string[] = [];
	const s = (n: number) => (n === 1 ? '' : 's');
	if (p.scope === 'album' || p.scope === 'tracks') {
		const n = p.tracks?.length ?? 0;
		out.push(p.scope === 'album' ? `${p.album}: ${n} track${s(n)} stop being kept up to date` : `${(p.tracks ?? []).join(', ')}: no longer kept up to date`);
	} else if (p.scope === 'season' || p.scope === 'episodes') {
		const n = p.episodes?.length ?? 0;
		out.push(p.scope === 'season' ? `Season ${p.season}: ${n} episode${s(n)} stop being kept up to date` : `${(p.episodes ?? []).join(', ')}: no longer kept up to date`);
	} else {
		out.push(`Leaves your Library. History stays.`);
	}
	if (p.held.length) {
		const n = p.held.length;
		out.push(p.holding_days > 0 ? `${n} file${s(n)} to the holding folder for ${p.holding_days} day${s(p.holding_days)}` : `${n} file${s(n)} to the holding folder`);
	}
	if (p.kept.length) out.push(`${p.kept.length} file${s(p.kept.length)} stay${p.kept.length === 1 ? 's' : ''} on disk`);
	if (!p.held.length && !p.kept.length) out.push('No files on disk');
	if (p.choice.never_add) out.push('Lists never add it again');
	if (p.active_downloads) out.push(`${p.active_downloads} download${s(p.active_downloads)} keep running`);
	return out;
}

export function finished(j: Pick<JobView, 'State'>): boolean {
	return j.State === 'done' || j.State === 'failed' || j.State === 'cancelled';
}

export async function followJob(
	get: (id: number) => Promise<JobView>,
	id: number,
	tick: (j: JobView) => void,
	opts: { every?: number; poke?: (wake: () => void) => () => void; timeout?: number } = {}
): Promise<JobView> {
	const every = opts.every ?? 700;
	const until = Date.now() + (opts.timeout ?? 10 * 60_000);
	let wake: (() => void) | null = null;
	const off = opts.poke?.(() => wake?.());
	try {
		for (;;) {
			const j = await get(id);
			tick(j);
			if (finished(j)) return j;
			if (Date.now() > until) throw new Error('This is taking a long time. It keeps running; check back later.');
			await new Promise<void>((res) => {
				const t = setTimeout(res, every);
				wake = () => {
					clearTimeout(t);
					res();
				};
			});
			wake = null;
		}
	} finally {
		off?.();
	}
}

// Picks the narrowest scope given; a whole title also carries the never-add choice.
export function deleteBody(scope: DeleteScope, files: boolean, never: boolean, apply: boolean): Record<string, unknown> {
	const b: Record<string, unknown> = { delete_files: files, apply };
	if (scope.albumId !== undefined) b.album_id = scope.albumId;
	else if (scope.trackIds?.length) b.track_ids = scope.trackIds;
	else if (scope.season !== undefined) b.season = scope.season;
	else if (scope.episodeIds?.length) b.episode_ids = scope.episodeIds;
	else b.never_add = never;
	return b;
}

// True when no season, episode, album or track limits the delete.
export function wholeScope(s: DeleteScope): boolean {
	return s.season === undefined && !s.episodeIds?.length && s.albumId === undefined && !s.trackIds?.length;
}
