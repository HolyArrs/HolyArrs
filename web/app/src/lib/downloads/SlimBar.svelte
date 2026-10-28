<script lang="ts">
	// The one progress bar of the Downloads page: a 3 px line and a small
	// percent, the same in the table, the phone list and every details view.
	let { value, tone = 'info', label, showValue = true, width }: { value: number; tone?: string; label?: string; showValue?: boolean; width?: string } = $props();
	const pct = $derived(Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0)));
</script>

<span class="slim {tone}" style:--slim-w={width} role={label ? 'progressbar' : undefined} aria-label={label} aria-valuemin={label ? 0 : undefined} aria-valuemax={label ? 100 : undefined} aria-valuenow={label ? Math.floor(pct) : undefined} aria-hidden={label ? undefined : 'true'}>
	<span class="track"><i style="transform: translateX({pct - 100}%)"></i></span>
	{#if showValue}<span class="pv" data-testid="row-percent">{Math.floor(pct)}%</span>{/if}
</span>

<style>
	.slim { display: inline-flex; align-items: center; gap: 6px; width: var(--slim-w, 100%); min-width: 0; }
	.track { position: relative; flex: 1; min-width: 24px; height: 3px; border-radius: 2px; background: var(--surface-3); overflow: hidden; }
	i { position: absolute; inset: 0; border-radius: 2px; background: var(--info); transition: transform var(--dur-slow) var(--ease-out); }
	.ok i { background: var(--ok); }
	.fail i { background: var(--fail); }
	.idle i { background: var(--text-faint); }
	.wait i { background: var(--wait); }
	.pv { flex: none; min-width: 3ch; text-align: right; color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; }
	@media (prefers-reduced-motion: reduce) { i { transition: none; } }
</style>
