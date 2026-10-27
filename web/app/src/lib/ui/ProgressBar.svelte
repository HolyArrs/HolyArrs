<script lang="ts">
	import Icon from '../Icon.svelte';

	// The fill slides (transform, no layout); at 100 a green tick pulses once (spec §3).
	let {
		value,
		label,
		showValue = false,
		size = 'md',
		tone = 'accent',
		on = 'surface'
	}: { value: number; label: string; showValue?: boolean; size?: 'sm' | 'md'; tone?: 'accent' | 'info' | 'ok'; on?: 'surface' | 'art' } = $props();
	const pct = $derived(Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0)));
	const done = $derived(pct >= 100);
</script>

<div class="ui-progress {size} {on}" class:done>
	<div class="track" role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)} aria-valuetext={done ? 'Finished' : `${Math.round(pct)}%`}>
		<i class={done ? 'ok' : tone} style="transform: translateX({pct - 100}%)"></i>
	</div>
	{#if done}
		<span class="tick" aria-hidden="true"><Icon name="check" size={size === 'sm' ? 11 : 13} /></span>
	{:else if showValue}
		<span class="val" aria-hidden="true">{Math.round(pct)}%</span>
	{/if}
</div>

<style>
	.ui-progress { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.track { position: relative; flex: 1; height: 6px; border-radius: 3px; background: var(--surface-3); overflow: hidden; }
	.sm .track { height: 4px; border-radius: 2px; }
	.art .track { background: rgb(255 255 255 / 28%); }
	i { position: absolute; inset: 0; border-radius: inherit; background: var(--accent); transition: transform var(--dur-slow) var(--ease-out), background var(--dur) var(--ease-out); }
	i.info { background: var(--info); }
	i.ok { background: var(--ok); }
	.tick { width: 18px; height: 18px; display: grid; place-items: center; border-radius: 50%; background: var(--ok); color: var(--on-art); animation: pulse var(--dur-slow) var(--ease-out) 1; }
	.sm .tick { width: 15px; height: 15px; }
	.val { min-width: 3.2ch; text-align: right; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.art .val { color: var(--on-art); }
	@keyframes pulse { 0% { transform: scale(1); } 50% { transform: scale(1.15); } 100% { transform: scale(1); } }
</style>
