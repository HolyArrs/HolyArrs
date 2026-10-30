<script lang="ts">
	// The live status on the right of the Downloads status tab row: two pill
	// tracks side by side, grouped like the toolbar's capsules. Speed holds
	// Down and Up; Storage holds free space over a meter, today's total and the
	// time left. Both are siblings of the status tab track.
	import type { Snippet } from 'svelte';
	import { reviewWord, timeWords } from './status-views.ts';

	interface Props {
		/** Speeds as words ("26 MB/s"). */
		down: string;
		up: string;
		/** Time until everything finishes, short ("25m"). */
		eta: string;
		free: string;
		/** The whole disk as words ("4 TB"). */
		disk: string;
		/** Free space as a share of the disk, 0..1. */
		freeShare: number | null;
		today: string;
		used: string;
		review: number;
		onreview: () => void;
		more: Snippet;
	}
	let { down, up, eta, free, disk, freeShare, today, used, review, onreview, more }: Props = $props();

	const time = $derived(timeWords(eta));
	const timeTip = $derived(eta ? `${eta} left until everything finishes` : 'Nothing is downloading');
	const freePct = $derived(freeShare == null ? null : Math.round(Math.min(1, Math.max(0, freeShare)) * 100));
</script>

{#snippet num(text: string)}{#key text}<b class="tk">{text}</b>{/key}{/snippet}

<div class="st" data-testid="dl-stat">
	<!-- Speed: Down and Up. -->
	<span class="t" role="group" aria-label="Speed">
		<span class="sg sp dn" data-testid="sb-down"><span class="sl">Down</span><span class="n" data-testid="dl-speed">{@render num(`↓ ${down}`)}</span></span>
		<span class="sg sp x k1" data-testid="sb-up"><span class="sl">Up</span><span class="n" data-testid="dl-up">{@render num(`↑ ${up}`)}</span></span>
	</span>
	<!-- Storage: free words over a thin free-space meter, today, time left. -->
	<span class="t" role="group" aria-label="Storage">
		<span class="sg mp" data-testid="sb-free" title="{used} used">
			<span class="ml"><span class="n">{@render num(free)}</span> <span class="sl">free of {disk}</span></span>
			{#if freePct != null}<span class="mtr" role="meter" aria-label="Disk space free" aria-valuemin={0} aria-valuemax={100} aria-valuenow={freePct} data-testid="dl-meter"><i style="width: {freePct}%"></i></span>{/if}
		</span>
		<span class="sg x k1" data-testid="sb-today"><span class="sl">Today</span><span class="n">{@render num(today)}</span></span>
		<span class="sg x k2" title={timeTip} data-testid="sb-time"><span class="sr-only">Time left: </span><span class="n tm">{@render num(time)}</span></span>
	</span>
	{#if review}<button type="button" class="rv x" onclick={onreview} data-testid="dl-need" title="Open Problems › Review"><i aria-hidden="true"></i>{reviewWord(review)}</button>{/if}
	<span class="mr x" data-testid="sb-more">{@render more()}</span>
</div>

<style>
	.st { display: flex; flex: 1 0 auto; align-items: center; justify-content: flex-end; gap: 6px; min-width: 0; height: 100%; margin: 0 0 0 16px; padding: 0; white-space: nowrap; color: var(--text); font-variant-numeric: tabular-nums; }
	.st * { margin: 0; }

	/* The track: the status tab track exactly (2px inset, 999px, surface-2). */
	.t { display: inline-flex; flex: none; align-items: center; gap: 2px; padding: 2px; border-radius: 999px; background: var(--surface-2); }
	.sg { position: relative; display: inline-flex; align-items: center; flex: none; gap: 6px; height: 28px; box-sizing: border-box; padding: 0 12px; border-radius: 999px; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: var(--t-footnote); white-space: nowrap; cursor: default; transition: background-color 200ms var(--ease-out), color 200ms var(--ease-out), box-shadow 300ms var(--ease-out); }
	.sg:hover { color: var(--text); background: color-mix(in srgb, var(--surface-3) 55%, transparent); }
	.sl { color: inherit; opacity: 0.8; }
	.sg .n { color: inherit; font-weight: 600; font-variant-numeric: tabular-nums; }
	.sg .n :global(b) { font-weight: inherit; }
	.dn .n { color: color-mix(in srgb, var(--ok) 45%, var(--text)); }

	/* Numbers keep their width as they change: tabular digits plus a floor. */
	.sp .n { display: inline-block; min-width: 7.2ch; text-align: right; }
	.mp .n { display: inline-block; min-width: 5.5ch; text-align: right; }
	.sg[data-testid='sb-today'] .n { display: inline-block; min-width: 5.5ch; text-align: right; }
	.tm { display: inline-block; min-width: 7ch; text-align: right; }

	/* Free words over a thin meter (the free part in green). */
	.mp { flex-direction: column; justify-content: center; align-items: stretch; gap: 3px; }
	.ml { line-height: 1; }
	.mtr { display: block; height: 3px; border-radius: 2px; background: var(--surface-3); overflow: hidden; }
	.mtr i { display: block; height: 100%; border-radius: inherit; background: var(--ok); transition: width 600ms var(--ease-out); }

	/* The review call: its own red pill at track height, after the two tracks. */
	.rv { display: inline-flex; flex: none; align-items: center; gap: 6px; height: 32px; padding: 0 12px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--fail); font: var(--t-footnote); font-weight: 600; cursor: pointer; transition: background-color 160ms var(--ease-out); }
	.rv i { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
	.rv:hover { background: color-mix(in srgb, var(--fail) 22%, transparent); }
	.rv:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.mr { display: inline-flex; flex: none; }
	.mr :global(button) { width: 32px; height: 32px; min-width: 0; padding: 0; border-radius: 999px; }
	.mr :global(svg) { width: 15px; height: 15px; }

	/* Numbers tick: the new value settles in from just below. */
	.tk { display: inline-block; animation: tick 220ms var(--ease-out, ease-out); }
	@keyframes tick { from { opacity: 0.4; transform: translateY(3px); } to { opacity: 1; transform: none; } }
	@media (prefers-reduced-motion: reduce) { .tk { animation: none; } .sg, .rv, .mtr i { transition: none; } }

	/* Narrower rows drop the lesser pills (they stay in the More menu) so the
	   status never runs past the row's right edge. */
	@container tr (max-width: 1180px) { .k1 { display: none; } }
	@container tr (max-width: 980px) { .k2 { display: none; } }
	/* Phones and narrow windows: only the Down pill and free space, pinned to the
	   row's right edge so they stay in view while the tabs scroll beneath. */
	@container tr (max-width: 880px) {
		.st { position: sticky; right: 0; z-index: 1; flex: none; margin-left: 8px; padding-left: 14px; gap: 4px; background: linear-gradient(90deg, transparent, var(--bg) 14px); }
		.x { display: none; }
		.sg { padding: 0 10px; }
		.sl { display: none; }
	}
</style>
