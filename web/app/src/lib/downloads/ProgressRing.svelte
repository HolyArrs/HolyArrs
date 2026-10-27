<script lang="ts">
	import Icon, { type IconName } from '../Icon.svelte';

	// A round progress meter: the ring fills with the percent; the middle shows
	// the percent, or a glyph (pause, alert, check) when the state says more.
	let {
		value,
		label,
		size = 44,
		stroke = 3.5,
		tone = 'info',
		icon,
		showValue = true,
		on = 'surface'
	}: {
		value: number;
		label: string;
		size?: number;
		stroke?: number;
		tone?: 'info' | 'ok' | 'fail' | 'idle' | 'wait';
		icon?: IconName;
		showValue?: boolean;
		on?: 'surface' | 'art';
	} = $props();
	const pct = $derived(Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0)));
	const r = $derived((size - stroke) / 2);
	const C = $derived(2 * Math.PI * r);
</script>

<span class="ring {tone} {on}" style="--size: {size}px" role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)} aria-valuetext="{Math.floor(pct)}%">
	<svg viewBox="0 0 {size} {size}" width={size} height={size} aria-hidden="true">
		<circle class="track" cx={size / 2} cy={size / 2} {r} fill="none" stroke-width={stroke} />
		<circle class="fill" cx={size / 2} cy={size / 2} {r} fill="none" stroke-width={stroke} stroke-linecap="round" stroke-dasharray="{(pct / 100) * C} {C}" transform="rotate(-90 {size / 2} {size / 2})" />
	</svg>
	<span class="mid" aria-hidden="true">
		{#if icon}<Icon name={icon} size={Math.round(size * 0.4)} stroke={2.2} />{:else if showValue}{Math.floor(pct)}<small>%</small>{/if}
	</span>
</span>

<style>
	.ring { position: relative; display: inline-grid; place-items: center; width: var(--size); height: var(--size); flex: none; --c: var(--info); }
	.ring.ok { --c: var(--ok); }
	.ring.fail { --c: var(--fail); }
	.ring.wait { --c: var(--wait); }
	.ring.idle { --c: var(--text-faint); }
	.ring.art { --c: var(--info-on-art); color: var(--on-art); }
	.ring.art.ok { --c: var(--ok-on-art); }
	.ring.art.fail { --c: var(--fail-on-art); }
	.ring.art.wait { --c: var(--wait-on-art); }
	.ring.art.idle { --c: var(--on-art-muted); }
	svg { position: absolute; inset: 0; }
	.track { stroke: color-mix(in srgb, currentColor 16%, transparent); }
	.art .track { stroke: var(--on-art-ring); }
	.fill { stroke: var(--c); transition: stroke-dasharray var(--dur-slow) var(--ease-out); }
	.mid { position: relative; display: inline-flex; align-items: baseline; font: var(--t-caption); font-size: calc(var(--size) * 0.27); font-variant-numeric: tabular-nums; letter-spacing: -0.02em; color: inherit; }
	.mid small { font-size: 0.7em; opacity: 0.75; }
	.ring:not(.art) .mid { color: var(--text); }
	.fail .mid, .idle .mid { color: var(--c); }
	@media (prefers-reduced-motion: reduce) { .fill { transition: none; } }
</style>
