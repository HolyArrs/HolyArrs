// The welcome step for choosing the parts: Movies, Shows, Music, Subtitles and Requests, as tiles (first run's choice screen and a later visit to /welcome).
import { type IconName } from '#lib/ui/Icon.svelte';
import { partsNow, refreshParts, needsProblem, type PartName } from '#lib/mediafiles/index.ts';
import { applyParts } from './controls.ts';
import { message } from '#lib/core/api.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createWelcomeParts(ctx: any) {
	function partsOn(): Record<PartName, boolean> {
		const on = (p: PartName) => partsNow.answer?.parts.find((x) => x.part === p)?.on === true;
		return { movies: on('movies'), shows: on('shows'), music: on('music'), subtitles: on('subtitles'), wishlist: on('wishlist'), sources: on('sources') };
	}

	type Choice = 'movies' | 'shows' | 'music' | 'subtitles' | 'wishlist';
	let want = $state<Record<Choice, boolean>>({ movies: true, shows: true, music: false, subtitles: true, wishlist: false });
	const choices: { part: Choice; label: string; desc: string; icon: IconName }[] = [
		{ part: 'movies', get label() { return t('setup.movies'); }, get desc() { return t('welcome.parts.d.movies'); }, icon: 'film' },
		{ part: 'shows', get label() { return t('setup.tvShows'); }, get desc() { return t('welcome.parts.d.shows'); }, icon: 'tv' },
		{ part: 'music', get label() { return t('setup.music'); }, get desc() { return t('welcome.parts.d.music'); }, icon: 'music' },
		{ part: 'subtitles', get label() { return t('setup.subtitles'); }, get desc() { return t('welcome.parts.d.subtitles'); }, icon: 'subtitle' },
		{ part: 'wishlist', get label() { return t('welcome.parts.requests'); }, get desc() { return t('welcome.parts.d.wishlist'); }, icon: 'requests' }
	];
	// The tiles start as the server has the parts, once any media part is on there.
	async function loadParts() {
		await refreshParts().catch(() => {});
		const on = partsOn();
		if (partsNow.answer && (on.movies || on.shows || on.music)) want = { movies: on.movies, shows: on.shows, music: on.music, subtitles: on.subtitles, wishlist: on.wishlist };
	}
	// Import found these kinds of library folder in the old apps: tick their parts (never unticks one).
	function tickFound(kinds: string[]) {
		if (kinds.includes('movie')) want.movies = true;
		if (kinds.includes('show')) want.shows = true;
		if (kinds.includes('music')) want.music = true;
	}
	// Sources is not asked about here, so it stays as it is.
	const full = $derived<Partial<Record<PartName, boolean>>>({ ...want });
	const problem = $derived(needsProblem(full));
	async function submitParts(e: SubmitEvent) {
		e.preventDefault();
		if (problem) return void (ctx.error = problem);
		ctx.error = '';
		ctx.busy = true;
		try {
			await applyParts(full);
			await refreshParts();
			await (ctx.after ? ctx.after() : ctx.home());
		} catch (err) {
			ctx.error = message(err);
		} finally {
			ctx.busy = false;
		}
	}

	// Media: a switch and a Library folder per part, on one screen.

	return {
		partsOn,
		submitParts,
		get choices() { return choices; },
		get want() { return want; },
		set want(v) { want = v; },
		loadParts,
		tickFound,
		get problem() { return problem; },
	};
}
