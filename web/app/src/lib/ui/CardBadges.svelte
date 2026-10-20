<script lang="ts" module>
	import type { OwnState } from '../fresh.ts';
	export type CardOwned = { state: OwnState; label: string };
</script>

<script lang="ts">
	import Icon from '../Icon.svelte';
	import { freshLabel, type Fresh } from '../fresh.ts';

	// The badges over a card's artwork, laid on the art box (it must be
	// position: relative). Top-left: a solid type pill (All only). Top-right:
	// a solid round library status. Bottom-centre: one freshness strip in the
	// accent colour. Same on posters, wide cards, Top 10 and music.
	let { fresh = null, owned = null, typePill = null, size = 'md' }: { fresh?: Fresh | null; owned?: CardOwned | null; typePill?: 'movie' | 'show' | 'artist' | null; size?: 'sm' | 'md' } = $props();

	const typeWords = { movie: 'Movie', show: 'Show', artist: 'Artist' } as const;
	/** Percent from a "Downloading 42%" label, for the ring. */
	const pct = $derived(owned?.state === 'downloading' ? Number(/(\d+)%/.exec(owned.label)?.[1] ?? NaN) : NaN);
	const R = 9;
	const C = 2 * Math.PI * R;
</script>

{#if typePill}<span class="type {typePill}" class:sm={size === 'sm'} data-testid="card-type">{typeWords[typePill]}</span>{/if}

{#if owned}
	<span class="mark {owned.state}" class:sm={size === 'sm'} role="img" aria-label={owned.label} title={owned.label} data-testid="card-mark" data-state={owned.state}>
		{#if owned.state === 'downloading'}
			<svg viewBox="0 0 24 24" aria-hidden="true">
				<circle cx="12" cy="12" r={R} fill="none" stroke="currentColor" stroke-opacity="0.35" stroke-width="2.5" />
				{#if Number.isFinite(pct)}<circle cx="12" cy="12" r={R} fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-dasharray="{(Math.max(0, Math.min(100, pct)) / 100) * C} {C}" transform="rotate(-90 12 12)" />{/if}
				<path d="M12 8.5v6M9.5 12.5 12 15l2.5-2.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
			</svg>
		{:else if owned.state === 'available'}
			<Icon name="check" size={14} stroke={3} />
		{:else if owned.state === 'partial'}
			<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6.5 12h11" stroke="currentColor" stroke-width="3" stroke-linecap="round" /></svg>
		{:else if owned.state === 'requested'}
			<Icon name="clock" size={14} stroke={2.4} />
		{:else}
			<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 8l8 8M16 8l-8 8" stroke="currentColor" stroke-width="2.8" stroke-linecap="round" /></svg>
		{/if}
	</span>
{/if}

{#if fresh}
	<span class="fresh {fresh.kind}" class:sm={size === 'sm'} role="img" aria-label={freshLabel(fresh)} title={freshLabel(fresh)} data-testid="card-fresh" data-kind={fresh.kind}>
		{#if fresh.kind === 'top10'}<b class="top">TOP<i>10</i></b>{#if fresh.rank}<span class="rk" aria-hidden="true">#{fresh.rank}</span>{/if}{:else}<span aria-hidden="true">{fresh.text}</span>{/if}
	</span>
{/if}

<style>
	.type, .mark, .fresh { position: absolute; z-index: 2; pointer-events: none; }
	/* Type pill: solid, rounded, small caps-like weight. */
	.type {
		left: 8px; top: 8px;
		height: 20px; padding: 0 8px;
		display: inline-flex; align-items: center;
		border-radius: var(--r-pill);
		color: var(--mark-ink);
		font: 600 11px/1 var(--font-text, inherit);
		letter-spacing: 0.02em;
		box-shadow: 0 0 0 1px var(--mark-ring), 0 1px 3px rgb(0 0 0 / 30%);
	}
	.type.movie { background: var(--type-movie); }
	.type.show { background: var(--type-show); }
	.type.artist { background: var(--type-artist); }
	/* Status circle: solid fill, white glyph (amber carries dark ink). */
	.mark {
		right: 8px; top: 8px;
		width: 24px; height: 24px;
		display: grid; place-items: center;
		border-radius: 50%;
		color: var(--mark-ink);
		box-shadow: 0 0 0 1.5px var(--mark-ring), 0 1px 4px rgb(0 0 0 / 35%);
	}
	.mark :global(svg) { width: 14px; height: 14px; }
	.mark.downloading :global(svg) { width: 20px; height: 20px; }
	.available, .partial { background: var(--mark-have); }
	.downloading { background: var(--mark-dl); }
	.requested { background: var(--mark-ask); color: var(--mark-ask-ink); }
	.declined { background: var(--mark-no); }
	/* Freshness strip: centred on the bottom edge of the art, flush to it. */
	.fresh {
		left: 50%; bottom: 0; transform: translateX(-50%);
		max-width: calc(100% - 16px);
		height: 22px; padding: 0 10px;
		display: inline-flex; align-items: center; gap: 5px;
		border-radius: 4px 4px 0 0;
		background: var(--accent-fill);
		color: #fff;
		font: 700 11px/1 var(--font-text, inherit);
		letter-spacing: 0.01em;
		white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
		box-shadow: 0 -1px 6px rgb(0 0 0 / 25%);
	}
	.top { display: inline-flex; flex-direction: column; align-items: center; font-size: 6.5px; line-height: 0.95; letter-spacing: 0.06em; }
	.top i { font-style: normal; font-size: 10px; letter-spacing: 0; }
	.rk { font-variant-numeric: tabular-nums; }
	.sm.type { height: 18px; padding: 0 6px; font-size: 10px; left: 6px; top: 6px; }
	.sm.mark { width: 20px; height: 20px; right: 6px; top: 6px; }
	.sm.mark :global(svg) { width: 12px; height: 12px; }
	.sm.mark.downloading :global(svg) { width: 17px; height: 17px; }
	.sm.fresh { height: 18px; font-size: 10px; padding: 0 8px; }
	@media (forced-colors: active) {
		.type, .mark, .fresh { border: 1px solid CanvasText; }
	}
</style>
