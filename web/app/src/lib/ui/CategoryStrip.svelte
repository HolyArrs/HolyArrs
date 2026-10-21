<script lang="ts">
	import Shelf from './Shelf.svelte';
	import Icon from '../Icon.svelte';
	import { to } from '../paths.ts';
	import { fitTile } from '../tmdb-image.ts';
	import { browseHref, tileHue, type BrowseTile } from '../search-browse.ts';
	import { SECTION_LABEL } from '../category-strip.ts';
	import type { ExploreType } from '../explore.ts';

	// The category strip under the Explore hero: frosted tiles over a soft,
	// blurred hint of the category's art, its hue glowing from the left edge,
	// a small section label over the name; then All categories (Search Browse).
	let { tiles, type }: { tiles: (BrowseTile & { section: string })[]; type: ExploreType } = $props();
	type Entry = { key: string; name: string; label: string; href: string; hue: string; art: string; all?: boolean };
	const entries = $derived<Entry[]>([
		...tiles.map((t) => ({
			key: t.category ?? `${t.kind}:${t.genre}:${t.mood ?? ''}`,
			name: t.name,
			label: SECTION_LABEL[t.section] ?? '',
			href: to(browseHref(t, type)),
			hue: tileHue(t.name),
			art: t.backdrop ? fitTile(t.backdrop, 200, false) : ''
		})),
		{ key: 'all', name: 'All categories', label: 'Browse', href: to(type === 'movies' ? '/search?kind=movie' : type === 'shows' ? '/search?kind=show' : '/search'), hue: 'var(--accent)', art: '', all: true }
	]);
</script>

<div class="cat-strip" data-row-id="category_strip">
	<Shelf title="Categories" item="clamp(172px, 14vw, 224px)" items={entries} key={(e) => e.key} lines={1}>
		{#snippet card(e: Entry)}
			<a class="cat" class:all={e.all} href={e.href} style="--hue: {e.hue}">
				{#if e.art}<img class="art" src={e.art} alt="" loading="lazy" decoding="async" />{/if}
				<span class="txt">
					<small>{e.label}</small>
					<strong>{e.name}{#if e.all}<Icon name="chevronRight" size={16} />{/if}</strong>
				</span>
			</a>
		{/snippet}
	</Shelf>
</div>

<style>
	/* The heading stays for screen readers only; the strip reads as part of the hero. */
	.cat-strip :global(.ui-shelf > header) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; margin: 0; }
	.cat-strip { margin-top: -12px; --glass: color-mix(in srgb, var(--surface-1) 62%, transparent); }
	.cat-strip :global(.ui-shelf .row) { padding-block: 4px 8px; }
	.cat-strip :global(.ui-shelf .arrow) { top: calc(50% - 22px); }
	.cat {
		position: relative; isolation: isolate; display: flex; align-items: center; height: 72px; padding: 0 16px 0 18px; overflow: hidden;
		border-radius: 14px; color: var(--text); background: var(--surface-2);
		border: 1px solid color-mix(in srgb, var(--text) 10%, transparent);
		transition: transform var(--dur-fast) var(--ease-out), border-color var(--dur-fast) ease, box-shadow var(--dur-fast) ease;
	}
	/* The art: blurred and faint, tinted toward the hue, under the glass. */
	.art { position: absolute; inset: -12px; z-index: -3; width: calc(100% + 24px); height: calc(100% + 24px); object-fit: cover; filter: blur(14px) saturate(130%); opacity: .55; transform: scale(1.05); }
	/* The hue glows from the left edge; the glass sits over all of it. */
	.cat::before { content: ''; position: absolute; inset: 0; z-index: -2; background: radial-gradient(90% 160% at 0% 50%, color-mix(in srgb, var(--hue) 70%, transparent), transparent 70%); }
	.cat::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(90deg, transparent 0%, var(--glass) 55%); -webkit-backdrop-filter: blur(6px); backdrop-filter: blur(6px); }
	.txt { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	small { font: 600 10.5px/1 var(--font-ui); letter-spacing: .09em; text-transform: uppercase; color: color-mix(in srgb, var(--text) 66%, transparent); }
	strong { display: flex; align-items: center; gap: 2px; font: var(--t-title3); letter-spacing: var(--t-title2-track); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.cat:hover { transform: translateY(-2px); border-color: color-mix(in srgb, var(--hue) 55%, transparent); box-shadow: 0 10px 24px -12px color-mix(in srgb, var(--hue) 70%, transparent); }
	.cat:active { transform: scale(.98); }
	.cat:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.cat:focus:not(:focus-visible) { outline: none; }
	.cat.all { color: var(--accent-text); }
	.cat.all strong { color: var(--accent-text); }
	/* Light mode: lighter glass and a softer art so the names stay dark and crisp. */
	:global(:root[data-theme='light']) .cat-strip, :global(.light) .cat-strip { --glass: color-mix(in srgb, var(--surface-1) 78%, transparent); }
	:global(:root[data-theme='light']) .art { opacity: .35; }
	@media (prefers-color-scheme: light) { :global(:root:not([data-theme='dark'])) .cat-strip { --glass: color-mix(in srgb, var(--surface-1) 78%, transparent); } :global(:root:not([data-theme='dark'])) .art { opacity: .35; } }
	@media (prefers-reduced-motion: reduce) { .cat { transition: none; } .cat:hover { transform: none; } }
</style>
