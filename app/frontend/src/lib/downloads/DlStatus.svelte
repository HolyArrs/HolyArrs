<script lang="ts">
	// Live status at the right of the status tabs: a Speed track (Down, Up, optional limit) and a Storage track (free space, today, time left).
	import Icon from '#lib/ui/Icon.svelte';
	import { reviewWord, timeWords } from './status-views.ts';
	import type { Snippet } from 'svelte';

	interface Props {
		down: string;
		up: string;
		eta: string;
		free: string;
		today: string;
		session?: string;
		review: number;
		onreview: () => void;
		more: Snippet;
		limit?: string;
		/** When set, the Speed pills form the button that opens the speed limit. */
		onlimit?: () => void;
	}
	let { down, up, eta, free, today, session = '', review, onreview, more, limit = '', onlimit }: Props = $props();

	const time = $derived(timeWords(eta));
	const timeTip = $derived(eta ? `${eta} left until everything finishes` : 'Nothing is downloading');
</script>

{#snippet num(text: string)}{#key text}<b class="tk">{text}</b>{/key}{/snippet}

<div class="st" data-testid="dl-stat">
	{#snippet speeds()}
		<span class="sg sp dn" data-testid="sb-down"><span class="sl">Down</span><span class="n" data-testid="dl-speed">{@render num(`↓ ${down}`)}</span></span>
		{#if up}<span class="sg sp x" data-testid="sb-up"><span class="sl">Up</span><span class="n" data-testid="dl-up">{@render num(`↑ ${up}`)}</span></span>{/if}
		{#if limit}<span class="sg x cap" data-testid="sb-limit"><Icon name="gauge" size={13} /><span class="sr-only">Speed limit </span><span class="n">{limit}</span></span>{/if}
	{/snippet}
	{#if onlimit}
		<button type="button" class="t tb" onclick={onlimit} title={limit ? `Speed limit: ${limit}. Change it` : 'Set a speed limit'} data-testid="dl-speed-limit"><span class="sr-only">Speed: </span>{@render speeds()}{#if !limit}<span class="sr-only">. Set a speed limit</span>{/if}</button>
	{:else}
		<span class="t" role="group" aria-label="Speed">{@render speeds()}</span>
	{/if}
	<span class="t" role="group" aria-label="Storage">
		{#if free}<span class="sg mp" data-testid="sb-free"><span class="ml"><span class="n">{@render num(free)}</span> <span class="sl">free</span></span></span>{/if}
		{#if today}<span class="sg x k1" data-testid="sb-today"><span class="sl">Today</span><span class="n">{@render num(today)}</span></span>
		{:else if session}<span class="sg x k1" data-testid="sb-today"><span class="sl">Session</span><span class="n">{@render num(session)}</span></span>{/if}
		{#if eta}<span class="sg x k2" title={timeTip} data-testid="sb-time"><span class="sr-only">Time left: </span><span class="n tm">{@render num(time)}</span></span>{/if}
	</span>
	{#if review}<button type="button" class="rv x" onclick={onreview} data-testid="dl-need" title="Open Problems › Review"><i aria-hidden="true"></i>{reviewWord(review)}</button>{/if}
	<span class="mr x" data-testid="sb-more">{@render more()}</span>
</div>

<style>
	.st { display: flex; flex: 1 0 auto; align-items: center; justify-content: flex-end; gap: 6px; min-width: 0; height: 100%; margin: 0 0 0 16px; padding: 0; white-space: nowrap; color: var(--text); font-variant-numeric: tabular-nums; }
	.st * { margin: 0; }

	.t { display: inline-flex; flex: none; align-items: center; gap: 2px; padding: 2px; border-radius: 999px; background: var(--surface-2); }
	.sg { position: relative; display: inline-flex; align-items: center; flex: none; gap: 6px; height: 28px; box-sizing: border-box; padding: 0 12px; border-radius: 999px; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: var(--t-footnote); white-space: nowrap; cursor: default; transition: background-color 200ms var(--ease-out), color 200ms var(--ease-out), box-shadow 300ms var(--ease-out); }
	.sg:hover { color: var(--text); background: color-mix(in srgb, var(--surface-3) 55%, transparent); }
	.sl { color: inherit; }
	.tb { border: 0; margin: 0; font: inherit; color: inherit; cursor: pointer; }
	.tb .sg { cursor: pointer; }
	.tb:hover .sg { color: var(--text); background: color-mix(in srgb, var(--surface-3) 55%, transparent); }
	.tb:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.cap :global(svg) { flex: none; }
	.sg .n { color: inherit; font-weight: 600; font-variant-numeric: tabular-nums; }
	.sg .n :global(b) { font-weight: inherit; }
	.dn .n { color: color-mix(in srgb, var(--ok) 45%, var(--text)); }

	.sp .n { display: inline-block; min-width: 7.2ch; text-align: right; }
	.mp .n { display: inline-block; min-width: 5.5ch; text-align: right; }
	.sg[data-testid='sb-today'] .n { display: inline-block; min-width: 5.5ch; text-align: right; }
	.tm { display: inline-block; min-width: 7ch; text-align: right; }

	.mp { flex-direction: column; justify-content: center; align-items: stretch; gap: 3px; }
	.ml { line-height: 1; }

	.rv { display: inline-flex; flex: none; align-items: center; gap: 6px; height: 32px; padding: 0 12px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--fail-text); font: var(--t-footnote); font-weight: 600; cursor: pointer; transition: background-color 160ms var(--ease-out); }
	.rv i { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
	.rv:hover { background: color-mix(in srgb, var(--fail) 22%, transparent); }
	.rv:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.mr { display: inline-flex; flex: none; padding: 2px; border-radius: 999px; background: var(--surface-2); }
	.mr :global(button) { width: 28px; height: 28px; min-width: 0; padding: 0; border: 0; border-radius: 999px; background: transparent; box-shadow: none; color: color-mix(in srgb, var(--text) 76%, var(--bg)); transition: background-color 200ms var(--ease-out), color 200ms var(--ease-out); }
	.mr :global(button:hover), .mr :global(button[aria-expanded='true']) { background: color-mix(in srgb, var(--surface-3) 55%, transparent); color: var(--text); }
	.mr :global(svg) { width: 15px; height: 15px; }

	.tk { display: inline-block; animation: tick 220ms var(--ease-out, ease-out); }
	@keyframes tick { from { opacity: 0.4; transform: translateY(3px); } to { opacity: 1; transform: none; } }
	@media (prefers-reduced-motion: reduce) { .tk { animation: none; } .sg, .rv { transition: none; } }

	/* Narrow rows drop the lesser pills; they stay in the More menu. */
	@container tr (max-width: 1180px) { .k1 { display: none; } }
	@container tr (max-width: 980px) { .k2 { display: none; } }
	/* Phones keep only the Down pill and free space, pinned to the right edge while the tabs scroll. */
	@container tr (max-width: 880px) {
		.st { position: sticky; right: 0; z-index: 1; flex: none; margin-left: 8px; padding-left: 14px; gap: 4px; background: linear-gradient(90deg, transparent, var(--bg) 14px); }
		.x { display: none; }
		.sg { padding: 0 10px; }
		.sl { display: none; }
	}
</style>
