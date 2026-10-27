<script lang="ts">
	import type { Snippet } from 'svelte';
	import { holdFor, loadStage, stageTimers, FADE_MS, type LoadStage } from '../../silhouette.ts';
	import Orbit from './Orbit.svelte';

	// A region that loads. While `loading`, it shows `skeleton` (in the layout from the first
	// frame, visible at 150 ms). After 500 ms an Orbit with `status` sits over it; after 10 s a
	// reassurance line and a retry. Content swaps in once the skeleton has shown for 100 ms (at once when `cached`),
	// cross-fading over 200 ms. Timing rules: lib/silhouette.ts.
	let {
		loading,
		skeleton,
		children,
		status = '',
		label = 'Loading',
		cached = false,
		onretry
	}: { loading: boolean; skeleton: Snippet; children?: Snippet; status?: string; label?: string; cached?: boolean; onretry?: () => void } = $props();

	const now = () => (typeof performance === 'undefined' ? Date.now() : performance.now());
	let showSkeleton = $state(true);
	let stage = $state<LoadStage>('quiet');
	let fade = false;
	let start = now();
	let content = $state<HTMLElement>();

	$effect(() => {
		if (loading) {
			start = now();
			showSkeleton = true;
			stage = 'quiet';
			const timers = stageTimers(0).map((t) => setTimeout(() => (stage = t.stage), t.at));
			return () => timers.forEach(clearTimeout);
		}
		const elapsed = now() - start;
		fade = elapsed >= 150;
		const wait = holdFor(elapsed, cached);
		if (!wait) {
			showSkeleton = false;
			return;
		}
		const t = setTimeout(() => (showSkeleton = false), wait);
		return () => clearTimeout(t);
	});

	$effect(() => {
		if (showSkeleton || !content || !fade) return;
		if (matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
		for (const el of Array.from(content.children)) el.animate?.([{ opacity: 0 }, { opacity: 1 }], { duration: FADE_MS, easing: 'ease-out' });
		fade = false;
	});
</script>

{#if showSkeleton}
	<div class="sil" aria-busy="true" aria-label={label}>
		{@render skeleton()}
		{#if stage === 'orbit' || stage === 'reassure'}
			<div class="status">
				<Orbit status={status || label + '…'} />
				{#if stage === 'reassure'}
					<p>Still working. The server is slow to answer.</p>
					<button type="button" class="retry" onclick={() => (onretry ? onretry() : location.reload())}>Try again</button>
				{/if}
			</div>
		{/if}
	</div>
{:else}
	<div class="sil-in" bind:this={content}>{@render children?.()}</div>
{/if}

<style>
	.sil { position: relative; min-width: 0; }
	.sil-in { display: contents; }
	.status {
		position: absolute; z-index: 2; left: 50%; top: min(38vh, 50%); transform: translate(-50%, -50%);
		display: flex; flex-direction: column; align-items: center; gap: 8px; max-width: min(360px, calc(100% - 32px));
		padding: 12px 18px; border-radius: var(--r-panel); text-align: center;
		background: color-mix(in srgb, var(--surface-1) 88%, transparent); backdrop-filter: blur(12px); box-shadow: var(--shadow-card);
		animation: sil-pop var(--dur, 200ms) ease-out both;
	}
	p { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	.retry { min-height: 36px; padding: 0 14px; border-radius: var(--r-btn); background: var(--accent-fill); color: var(--text-on-red); font: var(--t-callout); font-weight: 600; }
	.retry:hover { background: var(--accent-fill-hover); }
	@keyframes sil-pop { from { opacity: 0; } }
</style>
