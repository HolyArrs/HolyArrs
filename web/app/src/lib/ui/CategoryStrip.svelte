<script lang="ts">
	import Shelf from './Shelf.svelte';
	import Icon from '../Icon.svelte';
	import { to } from '../paths.ts';
	import { fitTile } from '../tmdb-image.ts';
	import { browseHref, type BrowseTile } from '../search-browse.ts';
	import { SECTION_LABEL } from '../category-strip.ts';
	import type { ExploreType } from '../explore.ts';
	import '@fontsource/anton/latin-400.css';

	// The category strip under the Explore hero: heavy condensed wordmarks over
	// a dark, grey still of the category's art; on hover or focus the still
	// comes up in its own colour and the word breathes open. A small crimson
	// section label sits over the name; then All categories (Search Browse).
	// onopen hears which tile was opened (the strip learns from it); the look is unchanged.
	let { tiles, type, onopen }: { tiles: (BrowseTile & { section: string })[]; type: ExploreType; onopen?: (t: BrowseTile & { section: string }) => void } = $props();
	type Entry = { key: string; name: string; label: string; href: string; art: string; all?: boolean; tile?: BrowseTile & { section: string } };
	const entries = $derived<Entry[]>([
		...tiles.map((t) => ({
			key: t.category ?? `${t.kind}:${t.genre}:${t.mood ?? ''}`,
			name: t.name,
			label: SECTION_LABEL[t.section] ?? '',
			href: to(browseHref(t, type)),
			art: t.backdrop ? fitTile(t.backdrop, 240, false) : '',
			tile: t
		})),
		{ key: 'all', name: 'All categories', label: 'Browse', href: to(type === 'movies' ? '/search?kind=movie' : type === 'shows' ? '/search?kind=show' : '/search'), art: '', all: true }
	]);
</script>

<div class="cat-strip" data-row-id="category_strip">
	<Shelf title="Categories" item="clamp(184px, 15vw, 236px)" items={entries} key={(e) => e.key} lines={1}>
		{#snippet card(e: Entry)}
			<a class="cat" class:all={e.all} href={e.href} style="--chars: {Math.max(e.name.length, 6)}" onclick={() => e.tile && onopen?.(e.tile)}>
				{#if e.art}<img class="art" src={e.art} alt="" loading="lazy" decoding="async" />{/if}
				<span class="txt">
					<small>{e.label}</small>
					<strong>{e.name}{#if e.all}<Icon name="chevronRight" size={18} />{/if}</strong>
				</span>
			</a>
		{/snippet}
	</Shelf>
</div>

<style>
	/* The heading stays for screen readers only; the strip reads as part of the hero. */
	.cat-strip :global(.ui-shelf > header) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; margin: 0; }
	.cat-strip { margin-top: -12px; --ease-tile: cubic-bezier(.16, 1, .3, 1); }
	.cat-strip :global(.ui-shelf .row) { padding-block: 4px 8px; }
	.cat-strip :global(.ui-shelf .arrow) { top: calc(50% - 22px); }
	/* Tiles stay dark in both modes: their own near-black glass, white type. */
	.cat {
		position: relative; isolation: isolate; container-type: inline-size; display: block; height: 84px; overflow: hidden;
		border-radius: 14px; color: #fff; background: #120a0b; border: 1px solid rgba(255, 255, 255, .07);
		transition: transform .45s var(--ease-tile), box-shadow .45s var(--ease-tile), border-color .45s var(--ease-tile);
	}
	/* At rest a grey, dim still; on hover it comes up in its own colour. */
	.art { position: absolute; inset: 0; z-index: -3; width: 100%; height: 100%; object-fit: cover; filter: grayscale(1) contrast(1.2); opacity: .3; transition: opacity .5s var(--ease-tile), filter .5s var(--ease-tile), transform .7s var(--ease-tile); }
	/* The dark glass from the left keeps the wordmark legible. */
	.cat::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(90deg, #0b0708 22%, rgba(11, 7, 8, .35) 70%, transparent); transition: opacity .5s var(--ease-tile); }
	.txt { position: absolute; left: 14px; right: 12px; bottom: 10px; display: flex; flex-direction: column; gap: 4px; }
	small { font: 600 10px/1 var(--font-ui); letter-spacing: .14em; text-transform: uppercase; color: #ff4a52; }
	/* One line always: the size shrinks to the tile width by name length (Anton caps run about .5em). */
	strong {
		display: flex; align-items: center; gap: 2px; white-space: nowrap; text-transform: uppercase;
		font: 400 min(34px, calc((100cqi - 30px) / (var(--chars) * .52))) / .9 Anton, var(--font-ui), sans-serif;
		letter-spacing: .005em; transform-origin: left bottom; transition: transform .5s var(--ease-tile), letter-spacing .5s var(--ease-tile);
	}
	.cat:is(:hover, :focus-visible) .art { opacity: .6; filter: none; transform: scale(1.08); }
	.cat:is(:hover, :focus-visible) strong { transform: scale(1.02); }
	.cat:hover { transform: translateY(-2px); }
	.cat:active { transform: scale(.98); }
	.cat:focus-visible { outline: none; box-shadow: 0 0 0 2px var(--bg, #0b0708), 0 0 0 4px #d0202a; }
	.cat:focus:not(:focus-visible) { outline: none; }
	.cat.all { background: linear-gradient(120deg, #3a0609, #120a0b 70%); }
	.cat.all small { color: #ffc9cb; }
	/* Light mode: the tile keeps its dark glass; a soft lift seats it on the light page. */
	:global(:root[data-theme='light']) .cat { box-shadow: 0 6px 18px -10px rgba(20, 8, 9, .45); border-color: rgba(0, 0, 0, .12); }
	@media (prefers-color-scheme: light) { :global(:root:not([data-theme='dark'])) .cat { box-shadow: 0 6px 18px -10px rgba(20, 8, 9, .45); border-color: rgba(0, 0, 0, .12); } }
	@media (prefers-reduced-motion: reduce) {
		.cat, .art, .cat::before, .cat::after, strong { transition: none; }
		.cat:hover, .cat:active, .cat:is(:hover, :focus-visible) .art, .cat:is(:hover, :focus-visible) strong { transform: none; }
	}
</style>
