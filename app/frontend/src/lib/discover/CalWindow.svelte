<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { entryArt, entryHref, itemDot } from './calendar.ts';
	import { entryTitle, groupTag, type CalGroup } from './calendar-groups.ts';
	import { cardLines, rowWords, type CaptionLook } from './calendar-captions.ts';
	import { epText, movieText } from './episode-text.svelte.ts';
	import type { CalendarEntry } from '#lib/core/types.ts';
	import { to } from '#lib/core/paths.ts';
	import { sized } from '#lib/core/tmdb-image.ts';

	// Details window of a calendar card or row (an episode's description, a movie's or album's facts, a way to the page). Shared by Cards and List.
	let { g, today, look, open = $bindable(false) }: { g: CalGroup; today: string; look: CaptionLook; open: boolean } = $props();

	const e = $derived<CalendarEntry>(g.lead);
	const src = $derived(e.kind === 'album' ? (e.poster ?? null) : entryArt(e));
	const lines = $derived(cardLines(g, look, today).slice(0, 2));
	const have = $derived(g.entries.every((x) => itemDot(x, today) === 'have'));
	const isEp = $derived(e.kind === 'episode');
	const words = $derived(rowWords(g, look, today));
	// An album says its name once (the window title), then the artist once; repeats are dropped.
	const title = $derived(entryTitle(e));
	const artist = $derived(e.kind === 'album' && e.name && e.name !== title ? e.name : '');
	const extra = $derived(words.lines.filter((l) => l !== title && l !== e.name));
	const page = $derived(to(entryHref(e)));
	let failed = $state(false);
</script>

<Sheet bind:open {title} size="md">
		<div class="ep-win" data-testid="cal-window">
			{#if (e.still || src) && !failed}
				<div class="ep-art" class:album={e.kind === 'album'}>
					<img class="ep-still" src={sized(e.still || src || '', e.kind === 'album' ? 'poster' : 'backdrop', 780)} alt="" />				</div>
			{/if}
			<p class="ep-when" class:tag={e.kind === 'album'}>{lines[0]}{#if isEp && groupTag(g)}{' · '}{groupTag(g)}{/if}</p>
			{#if isEp}
				{#each g.entries as x (x.item_id ?? x.episode)}
					{@const text = epText(x.tmdb, x.season, x.episode)}
					{@const named = !!x.episode_name && !/^Episode \d+$/.test(x.episode_name)}
					<div class="ep-one">
						<p class="ep-num">{t('discover.calWindow.seasonEpisode', { season: x.season ?? 0, episode: x.episode ?? 0 })}</p>
						{#if named}<h3 class="ep-name">{x.episode_name}</h3>{/if}
						{#if text}<p class="ep-text">{text}</p>{:else}<p class="ep-text none">{t('discover.calWindow.noDescriptionYet')}</p>{/if}
					</div>
				{/each}
			{:else}
				{#if artist}<p class="ep-artist">{artist}</p>{/if}
				{#if e.kind !== 'album' && open}{@const story = movieText(e.tmdb)}{#if story}<p class="ep-text story">{story}</p>{/if}{/if}
				{#if extra.length}<p class="ep-text">{extra.join(' · ')}</p>{/if}
			{/if}
			{#if have}<p class="ep-text have">{t('discover.calWindow.inTheLibrary')}</p>{/if}
		</div>
		{#snippet footer()}
			<Button variant="primary" href={page}>Go to {e.kind === 'album' ? 'album' : e.kind === 'episode' ? 'show' : 'movie'}</Button>
			<Button onclick={() => (open = false)}>{t('discover.calWindow.close')}</Button>
		{/snippet}
</Sheet>

<style>
	/* The description box is always two lines tall so every card in a row lines up. */
	.ep-win { display: flex; flex-direction: column; gap: 12px; }
	.ep-art { position: relative; border-radius: 10px; overflow: hidden; }
	.ep-still { display: block; width: 100%; aspect-ratio: 16 / 9; object-fit: cover; }
	.ep-art.album .ep-still { object-position: center 30%; }
	.ep-art.album { aspect-ratio: 16 / 9; box-shadow: 0 20px 50px rgb(0 0 0 / .35); }
	.ep-when.tag { align-self: flex-start; padding: 5px 10px; border-radius: 999px; background: var(--surface-2); font: 700 12px/1 var(--font-ui); text-transform: uppercase; letter-spacing: .06em; }
	.ep-artist { margin: 0; color: var(--text); font: 700 20px/1.25 var(--font-display); letter-spacing: -.01em; }
	.ep-when { margin: 0; color: var(--text); font: var(--t-subhead); font-weight: 700; font-variant-numeric: tabular-nums; }
	.ep-one { display: flex; flex-direction: column; gap: 4px; }
	.ep-num { margin: 0; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.ep-name { margin: 0; font: var(--t-headline); }
	.ep-text { margin: 0; color: var(--text-muted); font: var(--t-body); }
	.ep-text.story { color: var(--text); }
	.ep-text.none { opacity: .7; }
	.ep-text.have { color: var(--ok-text, var(--ok)); }
</style>
