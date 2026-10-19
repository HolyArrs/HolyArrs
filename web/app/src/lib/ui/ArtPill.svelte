<script lang="ts" module>
	export type ArtTone = 'plain' | 'ok' | 'info' | 'wait' | 'fail';
</script>

<script lang="ts">
	import Icon, { type IconName } from '../Icon.svelte';

	// The one badge over artwork: a small dark glass pill, hairline ring,
	// sentence case. Colour lives only in the glyph; the word stays white.
	// `compact` hides the word below a card width (container query) and keeps
	// the glyph, so a narrow poster never gets two long pills.
	let {
		text,
		icon,
		tone = 'plain',
		percent,
		label,
		compact = false,
		testid
	}: {
		text: string;
		icon?: IconName;
		tone?: ArtTone;
		/** 0–100: draws a thin progress ring as the glyph. */
		percent?: number;
		/** Accessible name when it says more than the visible word. */
		label?: string;
		compact?: boolean;
		testid?: string;
	} = $props();
	const pct = $derived(percent === undefined ? undefined : Math.max(0, Math.min(100, percent)));
	// Ring: r=7 on a 24 grid, so it sits on the same optical size as the icons.
	const C = 2 * Math.PI * 7;
</script>

<span class="ui-art-pill {tone}" class:compact class:glyph={!!icon || pct !== undefined} role={label ? 'img' : undefined} aria-label={label} title={label} data-testid={testid}>
	{#if pct !== undefined}
		<svg class="g ring" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
			<circle cx="12" cy="12" r="7" fill="none" stroke="currentColor" stroke-opacity="0.28" stroke-width="2.4" />
			<circle cx="12" cy="12" r="7" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-dasharray="{(pct / 100) * C} {C}" transform="rotate(-90 12 12)" />
		</svg>
	{:else if icon}
		<span class="g"><Icon name={icon} size={16} stroke={2} /></span>
	{/if}
	<span class="w" aria-hidden={label ? 'true' : undefined}>{text}</span>
</span>

<style>
	.ui-art-pill {
		--tint: var(--on-art);
		display: inline-flex;
		align-items: center;
		gap: 5px;
		height: 26px;
		max-width: 100%;
		padding: 0 10px;
		border-radius: var(--r-pill);
		color: var(--on-art);
		font: var(--t-caption);
		letter-spacing: 0.005em;
		white-space: nowrap;
		font-variant-numeric: tabular-nums;
		background: var(--on-art-scrim-mid);
		box-shadow: inset 0 0 0 1px var(--on-art-ring), inset 0 0.5px 0 var(--on-art-fill), 0 1px 3px var(--on-art-scrim-none);
		-webkit-backdrop-filter: blur(14px) saturate(160%);
		backdrop-filter: blur(14px) saturate(160%);
	}
	.glyph { padding-left: 7px; }
	.g { display: grid; place-items: center; width: 16px; height: 16px; flex: none; color: var(--tint); }
	.w { overflow: hidden; text-overflow: ellipsis; }
	.ok { --tint: var(--ok-on-art); }
	.info { --tint: var(--info-on-art); }
	.wait { --tint: var(--wait-on-art); }
	.fail { --tint: var(--fail-on-art); }
	/* Narrow cards sharing the row with a type pill: glyph only, a 28px glass circle. */
	@container art (max-width: 199px) {
		.compact.glyph { width: 28px; height: 28px; padding: 0; justify-content: center; }
		.compact.glyph .w { display: none; }
	}
</style>
