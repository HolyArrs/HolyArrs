<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import '@fontsource/anton/latin-400.css';
	import Shelf from '#lib/ui/Shelf.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { browseHref, type BrowseTile } from './search-browse.ts';
	import { SECTION_LABEL } from './category-strip.ts';
	import type { ExploreType } from './explore-type.ts';
	import { to } from '../core/paths.ts';
	import { sized } from '../core/tmdb-image.ts';
	import { minimalLook } from '../core/look.svelte.ts';

	// The category strip under the Explore hero: condensed wordmarks over a dark grey still of the category's art. On hover or focus the still comes up in its own colour.
	type Genre = { key: string; name: string; href: string; art: string; hue: number };
	let { tiles = [], genres, type, onopen }: { tiles?: (BrowseTile & { section: string })[]; genres?: Genre[]; type: ExploreType; onopen?: (t: BrowseTile & { section: string }) => void } = $props();
	/** n items picked evenly across the list. */
	const spread = <T,>(xs: T[], n: number): T[] => (xs.length <= n ? xs : Array.from({ length: n }, (_, i) => xs[Math.floor((i * xs.length) / n)]));
	type Entry = { key: string; name: string; label: string; href: string; art: string; arts?: string[]; all?: boolean; tile?: BrowseTile & { section: string }; tint?: string };
	const tint = (hue: number) => `hsl(${(((hue % 24) + 24) % 24) * 15} 52% 36%)`;
	let failed = $state<Record<string, boolean>>({});
	// The minimal look draws plain tiles with no stills, so none are fetched.
	const minimal = $derived(minimalLook());
	const entries = $derived<Entry[]>(genres ? [
		...genres.map((g) => ({ key: g.key, name: g.name, label: t('discover.categoryStrip.genre'), href: g.href, art: g.art, tint: tint(g.hue) })),
		{ key: 'all', name: t('discover.categoryStrip.allGenres'), label: t('discover.categoryStrip.browse'), href: to('/search?kind=music'), art: '', all: true }
	] : [
		...tiles.map((t) => ({
			key: t.category ?? `${t.kind}:${t.genre}:${t.mood ?? ''}`,
			name: t.name,
			label: SECTION_LABEL[t.section] ?? '',
			href: to(browseHref(t, type)),
			// A 142 to 236 px grey still: w780 through HolyArrs' artwork cache, never TMDB's original.
			art: t.backdrop ? sized(t.backdrop, 'backdrop', 240) : '',
			tile: t
		})),
		// All categories: four stills from tiles spread across the row (never the tile right beside it), so it reads as every category at once.
		{ key: 'all', name: t('discover.categoryStrip.allCategories'), label: t('discover.categoryStrip.browse'), href: to(type === 'movies' ? '/search?kind=movie' : type === 'shows' ? '/search?kind=show' : '/search'), art: '', arts: spread(tiles.slice(0, -1).filter((t) => t.backdrop).map((t) => sized(t.backdrop!, 'backdrop', 240)), 4), all: true }
	]);
</script>

<div class="cat-strip" data-row-id="category_strip">
	<Shelf title={t('discover.categoryStrip.categories')} item="clamp(184px, 15vw, 236px)" items={entries} key={(e) => e.key} lines={1}>
		{#snippet card(e: Entry)}
			<a class="cat" class:all={e.all} class:tinted={e.tint && (minimal || !e.art || failed[e.key])} class:minimal href={e.href} style="--chars: {Math.max(e.name.length, 6)}" style:--tint={e.tint} onclick={() => e.tile && onopen?.(e.tile)}>
				{#if !minimal && e.art && !failed[e.key]}<img class="art" src={e.art} alt="" loading="lazy" decoding="async" onerror={() => (failed[e.key] = true)} />{/if}
				{#if !minimal && e.arts?.length}<span class="art mosaic">{#each e.arts as a, i (i)}<img src={a} alt="" loading="lazy" decoding="async" />{/each}</span>{/if}
				<span class="txt">
					<small>{e.label}</small>
					<strong>{e.name}{#if e.all}<Icon name="chevronRight" size={18} />{/if}</strong>
				</span>
			</a>
		{/snippet}
	</Shelf>
</div>

<style>
	/* The heading stays for screen readers only. */
	.cat-strip :global(.ui-shelf > header) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; margin: 0; }
	.cat-strip { margin-top: -12px; --ease-tile: var(--ease-out); }
	.cat-strip :global(.ui-shelf .row) { padding-block: 4px 8px; }
	.cat-strip :global(.ui-shelf .arrow) { top: calc(50% - 22px); }
	/* Tiles stay dark in both themes. */
	.cat {
		position: relative; isolation: isolate; container-type: inline-size; display: block; height: 84px; overflow: hidden;
		border-radius: 14px; color: #fff; background: #120a0b; border: 1px solid rgba(255, 255, 255, .07);
		transition: transform var(--dur-slow) var(--ease-tile), box-shadow var(--dur-slow) var(--ease-tile), border-color var(--dur-slow) var(--ease-tile);
	}
	.art { position: absolute; inset: 0; z-index: -3; width: 100%; height: 100%; object-fit: cover; filter: grayscale(1) contrast(1.2); opacity: .3; transition: opacity var(--dur-slow) var(--ease-tile), filter var(--dur-slow) var(--ease-tile), transform var(--dur-slow) var(--ease-tile); }
	/* Dark glass from the left keeps the wordmark legible. */
	.cat::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(90deg, #0b0708 22%, rgba(11, 7, 8, .35) 70%, transparent); transition: opacity var(--dur-slow) var(--ease-tile); }
	.txt { position: absolute; left: 14px; right: 12px; bottom: 10px; display: flex; flex-direction: column; gap: 4px; }
	small { font: 600 10px/1 var(--font-ui); letter-spacing: .14em; text-transform: uppercase; color: #ff4a52; }
	/* One line always: the size shrinks to the tile width by name length (Anton caps run about .5em). */
	strong {
		display: flex; align-items: center; gap: 2px; white-space: nowrap; text-transform: uppercase;
		font: 400 min(34px, calc((100cqi - 30px) / (var(--chars) * .52))) / .9 Anton, var(--font-ui), sans-serif;
		letter-spacing: .005em; transform-origin: left bottom; transition: transform var(--dur-slow) var(--ease-tile), letter-spacing var(--dur-slow) var(--ease-tile);
	}
	.cat:is(:hover, :focus-visible) .art { opacity: .6; filter: none; transform: scale(1.08); }
	.cat:is(:hover, :focus-visible) strong { transform: scale(1.02); }
	.cat:hover { transform: translateY(-2px); }
	.cat:active { transform: scale(.98); }
	.cat:focus-visible { outline: none; box-shadow: 0 0 0 2px var(--bg, #0b0708), 0 0 0 4px #d0202a; }
	.cat:focus:not(:focus-visible) { outline: none; }
	.mosaic { display: flex; gap: 2px; }
	.mosaic img { flex: 1 1 0; min-width: 0; height: 100%; object-fit: cover; }
	.cat.all { background: linear-gradient(120deg, #3a0609, #120a0b 70%); }
	.cat.all small { color: #ffc9cb; }
	/* A music genre with no picture shows its hue as a gradient, never a broken image. */
	.cat.tinted { background: linear-gradient(120deg, color-mix(in srgb, var(--tint) 70%, #120a0b), #120a0b 75%); }
	:global(:root[data-theme='light']) .cat { box-shadow: 0 6px 18px -10px rgba(20, 8, 9, .45); border-color: rgba(0, 0, 0, .12); }
	@media (prefers-color-scheme: light) { :global(:root:not([data-theme='dark'])) .cat { box-shadow: 0 6px 18px -10px rgba(20, 8, 9, .45); border-color: rgba(0, 0, 0, .12); } }
	/* Minimal: no lift or zoom on hover. */
	.cat.minimal:hover, .cat.minimal:is(:hover, :focus-visible) strong { transform: none; }
	@media (prefers-reduced-motion: reduce) {
		.cat, .art, .cat::before, .cat::after, strong { transition: none; }
		.cat:hover, .cat:active, .cat:is(:hover, :focus-visible) .art, .cat:is(:hover, :focus-visible) strong { transform: none; }
	}
</style>
