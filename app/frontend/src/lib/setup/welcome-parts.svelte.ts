// The welcome step for choosing the parts.
import { type IconName } from '#lib/ui/Icon.svelte';
import { partsNow, refreshParts, needsProblem, type PartName } from '#lib/mediafiles/index.ts';
import { applyParts } from './controls.ts';
import { message } from '#lib/core/api.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createWelcomeParts(ctx: any) {
	function partsOn(): Record<PartName, boolean> {
		const on = (p: PartName) => partsNow.answer?.parts.find((x) => x.part === p)?.on === true;
		return { movies: on('movies'), shows: on('shows'), music: on('music'), subtitles: on('subtitles'), wishlist: on('wishlist') };
	}

	// Parts: a later visit to /welcome, outside first run.
	let want = $state<Record<'movies' | 'shows' | 'music' | 'subtitles', boolean>>({ movies: true, shows: true, music: false, subtitles: true });
	let family = $state<'yes' | 'no'>('no');
	const choices: { part: 'movies' | 'shows' | 'music' | 'subtitles'; label: string; icon: IconName }[] = [
		{ part: 'movies', get label() { return t('setup.movies'); }, icon: 'film' },
		{ part: 'shows', get label() { return t('setup.tvShows'); }, icon: 'tv' },
		{ part: 'music', get label() { return t('setup.music'); }, icon: 'music' },
		{ part: 'subtitles', get label() { return t('setup.subtitles'); }, icon: 'subtitle' }
	];
	const full = $derived<Record<PartName, boolean>>({ ...want, wishlist: family === 'yes' });
	const problem = $derived(needsProblem(full));
	async function submitParts(e: SubmitEvent) {
		e.preventDefault();
		if (problem) return void (ctx.error = problem);
		ctx.error = '';
		ctx.busy = true;
		try {
			await applyParts(full);
			await refreshParts();
			await ctx.home();
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
		get family() { return family; },
		set family(v) { family = v; },
		get problem() { return problem; },
	};
}
