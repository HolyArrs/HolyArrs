<script lang="ts" module>
	import type { OwnState } from './fresh.ts';
	export type CardOwned = { state: OwnState; label: string };
</script>

<script lang="ts">
	// Corner marks for a card: ownership state and a freshness tag.
	import Icon from './Icon.svelte';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { freshLabel, type Fresh } from './fresh.ts';

	let { fresh = null, owned = null, size = 'md' }: { fresh?: Fresh | null; owned?: CardOwned | null; size?: 'sm' | 'md' } = $props();

	// Download percent parsed from the label; NaN means unknown.
	const pct = $derived(owned?.state === 'downloading' ? Number(/(\d+)%/.exec(owned.label)?.[1] ?? NaN) : NaN);
	// Ring radius and length in the 24px viewbox, for the progress arc.
	const R = 9;
	const C = 2 * Math.PI * R;
</script>

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
		<span aria-hidden="true">{serverWords(fresh.text)}</span>
	</span>
{/if}

<style>
	/* Marks are drawn over the card art and must not catch clicks. */
	.mark, .fresh { position: absolute; z-index: 2; pointer-events: none; }
	.mark {
		right: 8px; top: 8px;
		width: 28px; height: 28px;
		display: grid; place-items: center;
		border-radius: 50%;
		color: var(--mark-ink);
		box-shadow: 0 0 0 1.5px var(--mark-ring), 0 1px 4px rgb(0 0 0 / 35%);
	}
	.mark :global(svg) { width: 16px; height: 16px; }
	.mark.downloading :global(svg) { width: 23px; height: 23px; }
	.available, .partial { background: var(--mark-have); }
	.downloading { background: var(--mark-dl); }
	.requested { background: var(--mark-ask); color: var(--mark-ask-ink); }
	.declined { background: var(--mark-no); }
	.fresh {
		left: 50%; bottom: 0; transform: translateX(-50%);
		max-width: calc(100% - 12px);
		height: 17px; padding: 0 6px;
		display: inline-flex; align-items: center;
		border-radius: 2px 2px 0 0;
		background: var(--accent-fill);
		color: var(--accent-ink, #fff);
		font: 700 11px/1 var(--font-ui);
		letter-spacing: 0.015em;
		white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
	}
	.sm.mark { width: 20px; height: 20px; right: 6px; top: 6px; }
	.sm.mark :global(svg) { width: 12px; height: 12px; }
	.sm.mark.downloading :global(svg) { width: 17px; height: 17px; }
	.sm.fresh { height: 16px; font-size: 10px; padding: 0 5px; }
	/* Colors are dropped in forced colors, so add an outline. */
	@media (forced-colors: active) {
		.mark, .fresh { border: 1px solid CanvasText; }
	}
</style>
