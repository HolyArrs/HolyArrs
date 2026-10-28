<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { to } from '#lib/paths.ts';
	import { entryArt, entryHref, itemDot, type Follows } from '#lib/calendar.ts';
	import { dayChip, entryTitle, groupTag, type CalGroup } from '#lib/calendar-looks.ts';
	import { cardLines, rowWords, DEFAULT_LOOK, type CaptionLook } from '#lib/calendar-captions.ts';
	import { sized } from '#lib/tmdb-image.ts';
	import Badge from '#lib/ui/Badge.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import FollowWho from './FollowWho.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { epCode, epText } from './ep-text.svelte.ts';

	// Coming Soon's card for rows: the Day view's big backdrop card made row
	// size, with the name on the art and its badges. Several episodes of one
	// show on one day are one card, drawn as a small stack ("3 episodes").
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
	// The time (or Cinema, At home, Out now) first, then each line that has real data; the day is beside the card.
	const lines = $derived(cardLines(g, look, today).slice(0, 2));
	const have = $derived(g.entries.every((x) => itemDot(x, today) === 'have'));
	let failed = $state(false);
	// A click anywhere on the card opens a window with the details (an
	// episode's description, a movie's or album's facts) and a way to the page.
	const isEp = $derived(e.kind === 'episode');
	const code = $derived(isEp ? epCode(g.entries) : '');
	const epLines = $derived(isEp ? [[lines[0], code].filter(Boolean).join(' · '), n > 1 ? `${n} episodes` : e.episode_name || lines[1] || ''] : lines);
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
				{#if e.kind === 'album'}<img class="cover" src={sized(src, 'poster', 140)} alt="" loading="lazy" decoding="async" />{/if}
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
				<IconButton icon={e.following ? 'bellOn' : 'bell'} size="sm" variant="glass" label="{e.following ? 'Unfollow' : 'Follow'} {entryTitle(e)}" aria-pressed={!!e.following} onclick={() => onfollow(e, !e.following)} data-testid="cal-follow" />
			</span>
		{/if}
	</div>
	{#snippet capLines()}
		{#each lines as l, i (i)}<p class="cap" class:when={i === 0} data-testid="cal-cap">{l}{#if have && i === lines.length - 1}<span class="have"> · In the Library</span>{/if}</p>{/each}
	{/snippet}
	<button type="button" class="caps tap" onclick={() => (open = true)} aria-haspopup="dialog" data-testid="cal-open">{@render capLines()}</button>
	<Sheet bind:open title={entryTitle(e)} size="md">
		<div class="ep-win" data-testid="cal-window">
			{#if (e.still || src) && !failed}
				<div class="ep-art" class:album={e.kind === 'album'}>
					<img class="ep-still" src={sized(e.still || src || '', e.kind === 'album' ? 'poster' : 'backdrop', 780)} alt="" />
					{#if e.kind === 'album' && src}<img class="ep-cover" src={sized(src, 'poster', 300)} alt="" />{/if}
				</div>
			{/if}
			<p class="ep-when">{lines[0]}{#if isEp && groupTag(g)}{' · '}{groupTag(g)}{/if}</p>
			{#if isEp}
				{#each g.entries as x (x.item_id ?? x.episode)}
					{@const t = epText(x.tmdb, x.season, x.episode)}
					{@const named = !!x.episode_name && !/^Episode \d+$/.test(x.episode_name)}
					<div class="ep-one">
						<p class="ep-num">Season {x.season}, Episode {x.episode}</p>
						{#if named}<h3 class="ep-name">{x.episode_name}</h3>{/if}
						{#if t}<p class="ep-text">{t}</p>{:else}<p class="ep-text none">No description yet.</p>{/if}
					</div>
				{/each}
			{:else}
				{#if e.kind === 'album' && e.album}<h3 class="ep-name">{e.album}</h3>{/if}
				{#each words.lines as l (l)}<p class="ep-text">{l}</p>{/each}
			{/if}
			{#if have}<p class="ep-text have">In the Library</p>{/if}
		</div>
		{#snippet footer()}
			<Button variant="primary" href={page}>Go to {e.kind === 'album' ? 'album' : e.kind === 'episode' ? 'show' : 'movie'}</Button>
			<Button onclick={() => (open = false)}>Close</Button>
		{/snippet}
	</Sheet>
	{#if mode === 'everyone'}<p class="who"><FollowWho {e} size={16} /></p>{/if}
</article>

<style>
	.ct { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.ct.past { opacity: 0.75; }
	/* Every card keeps 8px under its art; a stack's layers peek out there, so all tops line up. */
	.pile { position: relative; display: flex; flex-direction: column; padding-bottom: 8px; }
	.layer { position: absolute; border-radius: var(--r-card); background: var(--surface-3); box-shadow: var(--shadow-card); }
	.l1 { bottom: 4px; left: 4%; right: 4%; height: 40px; opacity: 0.9; }
	.l2 { bottom: 0; left: 8%; right: 8%; height: 40px; opacity: 0.55; }
	.art { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card), 0 8px 22px -10px rgb(0 0 0 / 0.22); transition: transform var(--dur-slow) var(--ease-out), box-shadow var(--dur-slow) var(--ease-out); }
	.art:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.art > :global(.ui-art-ph) { position: absolute; inset: 0; }
	.bg { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.album .bg { filter: blur(22px) saturate(140%) brightness(0.7); transform: scale(1.2); }
	.cover { position: absolute; right: 6%; top: 50%; translate: 0 -50%; height: 70%; aspect-ratio: 1; object-fit: cover; border-radius: 6px; box-shadow: 0 8px 24px rgb(0 0 0 / 0.5); }
	.scrim { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 0.75), rgb(0 0 0 / 0.08) 60%, transparent); pointer-events: none; }
	.on-art { position: absolute; left: 6%; right: 6%; bottom: 8%; display: flex; flex-direction: column; align-items: flex-start; gap: 6px; }
	.album .on-art { right: 46%; }
	.tags { display: flex; flex-wrap: wrap; gap: 4px; }
	.name { color: #fff; font: 700 clamp(17px, 1.6vw, 22px)/1.08 var(--font-display); letter-spacing: -0.02em; text-shadow: 0 2px 12px rgb(0 0 0 / 0.5); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	/* Caption lines, one line each. */
	.caps { display: flex; flex-direction: column; gap: 2px; min-width: 0; height: 38px; overflow: hidden; }
	.cap { color: var(--text-muted); font: var(--t-footnote); line-height: 18px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.cap.when { color: var(--text); font-weight: 700; font-variant-numeric: tabular-nums; }
	.tap { all: unset; display: flex; flex-direction: column; gap: 2px; min-width: 0; height: 38px; overflow: hidden; cursor: pointer; border-radius: 6px; }
	.tap:hover .cap.when { text-decoration: underline; text-underline-offset: 3px; }
	.tap:focus-visible { box-shadow: var(--focus-ring); }
	/* Layout 1: the description in a soft shaded box, always two lines tall so every card in a row lines up. */
	.ep-win { display: flex; flex-direction: column; gap: 12px; }
	.ep-art { position: relative; border-radius: 10px; overflow: hidden; }
	.ep-still { display: block; width: 100%; aspect-ratio: 16 / 9; object-fit: cover; }
	.ep-art.album .ep-still { filter: blur(22px) saturate(140%) brightness(.7); transform: scale(1.2); }
	.ep-cover { position: absolute; left: 50%; top: 50%; translate: -50% -50%; height: 76%; aspect-ratio: 1; object-fit: cover; border-radius: 8px; box-shadow: 0 8px 24px rgb(0 0 0 / .5); }
	.ep-when { margin: 0; color: var(--text); font: var(--t-subhead); font-weight: 700; font-variant-numeric: tabular-nums; }
	.ep-one { display: flex; flex-direction: column; gap: 4px; }
	.ep-num { margin: 0; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.ep-name { margin: 0; font: var(--t-headline); }
	.ep-text { margin: 0; color: var(--text-muted); font: var(--t-body); }
	.ep-text.none { opacity: .7; }
	.ep-text.have { color: var(--ok-text, var(--ok)); }
	.ep-go { align-self: flex-start; color: var(--accent-text); font: var(--t-subhead); font-weight: 600; }
	.have { color: var(--ok-text, var(--ok)); }
	.bell { position: absolute; top: 8px; right: 8px; }
	.bell.on :global(.ui-ibtn) { color: var(--accent-text); background: color-mix(in srgb, var(--surface-1) 88%, transparent); }
	.who { display: flex; min-width: 0; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.art:hover { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	}
</style>
