<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Badge from '#lib/ui/Badge.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { entryArt, entryHref, itemDot, type Follows } from './calendar.ts';
	import { dayChip, entryTitle, groupTag, type CalGroup } from './calendar-groups.ts';
	import { cardLines, rowWords, DEFAULT_LOOK, type CaptionLook } from './calendar-captions.ts';
	import FollowWho from './FollowWho.svelte';
	import CalWindow from './CalWindow.svelte';
	import { epCode } from './episode-text.svelte.ts';
	import type { CalendarEntry } from '#lib/core/types.ts';
	import { to } from '#lib/core/paths.ts';
	import { sized } from '#lib/core/tmdb-image.ts';

	// Coming Soon's card for rows: the Day view's backdrop card at row size, with the name on the art and its badges. Several episodes of one show on one day are one card, drawn as a small stack ("3 episodes").
	let {
		g,
		today,
		chip = false,
		mode = 'library',
		look = { ...DEFAULT_LOOK, region: 'global' },
		onfollow,
	}: {
		g: CalGroup;
		today: string;
		/** Put the day ("Wed 28") on the art. */ chip?: boolean;
		/** Everyone's follows names who follows the title. */ mode?: Follows;
		/** Which caption each kind shows. */ look?: CaptionLook;
		/** Follow or stop following the title (a bell on the art). */ onfollow?: (e: CalendarEntry, on: boolean) => void;
	} = $props();

	const e = $derived<CalendarEntry>(g.lead);
	const n = $derived(g.entries.length);
	const src = $derived(e.kind === 'album' ? (e.poster ?? null) : entryArt(e));
	const badges = $derived([chip ? dayChip(g.day, today) : '', groupTag(g)].filter(Boolean));
	// The time (or Cinema, At home, Out now) first, then each line that has real data.
	const lines = $derived(cardLines(g, look, today).slice(0, 2));
	const have = $derived(g.entries.every((x) => itemDot(x, today) === 'have'));
	let failed = $state(false);
	// A click anywhere on the card opens a window with the details and a way to the page.
	const isEp = $derived(e.kind === 'episode');
	const code = $derived(isEp ? epCode(g.entries) : '');
	const epLines = $derived(isEp ? [[lines[0], code].filter(Boolean).join(' · '), n > 1 ? t('discover.calTile.episodes', { n }) : e.episode_name || lines[1] || ''] : lines);
	const words = $derived(rowWords(g, look, today));
	const page = $derived(to(entryHref(e)));
	function show(ev: MouseEvent) {
		// A modified click (new tab, new window) still goes to the page.
		if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.button !== 0) return;
		ev.preventDefault();
		open = true;
	}
	let open = $state(false);

</script>

<article class="ct" class:album={e.kind === 'album'} class:stacked={n > 1} class:past={g.day < today} data-testid="cal-tile">
	<div class="pile">
		{#if n > 1}<span class="layer l2" aria-hidden="true"></span><span class="layer l1" aria-hidden="true"></span>{/if}
		<a class="art" href={page} onclick={show} aria-haspopup="dialog" aria-label={[entryTitle(e), groupTag(g), ...lines].filter(Boolean).join(', ')}>
			{#if src && !failed}
				<img class="bg" src={sized(src, e.kind === 'album' ? 'poster' : 'backdrop', 400)} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />
			{:else}
				<ArtPlaceholder name={entryTitle(e)} shape="wide" />
			{/if}
			<span class="scrim" aria-hidden="true"></span>
			<span class="on-art" aria-hidden="true">
				<span class="tags">{#each badges as b (b)}<Badge label={b} on="art" />{/each}</span>
				<span class="name">{entryTitle(e)}</span>
			</span>
		</a>
		{#if onfollow}
			<span class="bell" class:on={e.following}>
				<IconButton icon={e.following ? 'bellOn' : 'bell'} size="sm" variant="glass" label={e.following ? t('discover.calTile.unfollow', { name: entryTitle(e) }) : t('discover.calTile.follow', { name: entryTitle(e) })} aria-pressed={!!e.following} onclick={() => onfollow(e, !e.following)} data-testid="cal-follow" />
			</span>
		{/if}
	</div>
	{#snippet capLines()}
		{#each lines as l, i (i)}<p class="cap" class:when={i === 0} data-testid="cal-cap">{l}{#if have && i === lines.length - 1}<span class="have"> {t('discover.calTile.inTheLibrary')}</span>{/if}</p>{/each}
	{/snippet}
	<button type="button" class="caps tap" onclick={() => (open = true)} aria-haspopup="dialog" data-testid="cal-open">{@render capLines()}</button>
	<CalWindow bind:open {g} {today} {look} />
	{#if mode === 'everyone'}<p class="who"><FollowWho {e} size={16} /></p>{/if}
</article>

<style>
	.ct { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	/* A past day fades its picture only; the words keep full contrast. */
	.ct.past .pile { opacity: 0.75; }
	/* Every card keeps 8px under its art so a stack's layers peek out there and all tops line up. */
	.pile { position: relative; display: flex; flex-direction: column; padding-bottom: 8px; }
	.layer { position: absolute; border-radius: var(--r-card); background: var(--surface-3); box-shadow: var(--shadow-card); }
	.l1 { bottom: 4px; left: 4%; right: 4%; height: 40px; opacity: 0.9; }
	.l2 { bottom: 0; left: 8%; right: 8%; height: 40px; opacity: 0.55; }
	.art { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card), 0 8px 22px -10px rgb(0 0 0 / 0.22); transition: transform var(--dur-slow) var(--ease-out), box-shadow var(--dur-slow) var(--ease-out); }
	.art:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.bg { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.album .bg { object-position: center 30%; }
	.scrim { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 0.75), rgb(0 0 0 / 0.08) 60%, transparent); pointer-events: none; }
	.on-art { position: absolute; left: 6%; right: 6%; bottom: 8%; display: flex; flex-direction: column; align-items: flex-start; gap: 6px; }
	.tags { display: flex; flex-wrap: wrap; gap: 4px; }
	.name { color: #fff; font: 700 clamp(17px, 1.6vw, 22px)/1.08 var(--font-display); letter-spacing: -0.02em; text-shadow: 0 2px 12px rgb(0 0 0 / 0.5); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.caps { display: flex; flex-direction: column; gap: 2px; min-width: 0; height: 38px; overflow: hidden; }
	.cap { color: var(--text-muted); font: var(--t-footnote); line-height: 18px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.cap.when { color: var(--text); font-weight: 700; font-variant-numeric: tabular-nums; }
	.tap { all: unset; display: flex; flex-direction: column; gap: 2px; min-width: 0; height: 38px; overflow: hidden; cursor: pointer; border-radius: 6px; }
	.tap:hover .cap.when { text-decoration: underline; text-underline-offset: 3px; }
	.tap:focus-visible { box-shadow: var(--focus-ring); }
	.have { color: var(--ok-text, var(--ok)); }
	.bell { position: absolute; top: 8px; right: 8px; }
	.bell.on :global(.ui-ibtn) { color: var(--accent-text); background: color-mix(in srgb, var(--surface-1) 88%, transparent); }
	.who { display: flex; min-width: 0; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.art:hover { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	}
	/* Phones: the caption button's tap area grows to 44px, with padding and margin cancelling so the card keeps its size. */
	@media (max-width: 767px) and (pointer: coarse) { .tap { position: relative; padding-block: 3px; margin-block: -3px; } }
</style>
