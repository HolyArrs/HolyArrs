<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Shows a skeleton while loading, then fades the content in once timing rules allow.
	import type { Snippet } from 'svelte';
	import Orbit from './Orbit.svelte';
	import { holdFor, loadStage, stageTimers, FADE_MS, type LoadStage } from '../../core/silhouette.ts';

	let {
		loading,
		skeleton,
		children,
		status = '',
		label = t('ui.skSilhouette.loading'),
		cached = false,
		onretry
	}: { loading: boolean; skeleton: Snippet; children?: Snippet; status?: string; label?: string; cached?: boolean; onretry?: () => void } = $props();

	let showSkeleton = $state(true);
	let stage = $state<LoadStage>('quiet');
	let fade = false;
	let start = now();
	let content = $state<HTMLElement>();

	// Restarts the stage timers on each load; on finish, holds the skeleton briefly so it never flashes.
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

	// Fade only if the skeleton was visible, and never under reduced motion.
	$effect(() => {
		if (showSkeleton || !content || !fade) return;
		if (matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
		for (const el of Array.from(content.children)) el.animate?.([{ opacity: 0 }, { opacity: 1 }], { duration: FADE_MS, easing: 'ease-out' });
		fade = false;
	});

	// performance.now is steadier than Date.now, with a fallback for non-browser runs.
	function now() {
		return typeof performance === 'undefined' ? Date.now() : performance.now();
	}
</script>

{#if showSkeleton}
	<div class="sil" aria-busy="true" aria-label={label}>
		{@render skeleton()}
		{#if stage === 'orbit' || stage === 'reassure'}
			<div class="status">
				<Orbit status={status || label + '…'} />
				{#if stage === 'reassure'}
					<p>{t('ui.skSilhouette.stillWorkingTheServer')}</p>
					<button type="button" class="retry" onclick={() => (onretry ? onretry() : location.reload())}>{t('ui.skSilhouette.tryAgain')}</button>
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
		animation: sil-pop var(--dur, 200ms) var(--ease-out) both;
	}
	p { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	.retry { min-height: 36px; padding: 0 14px; border-radius: var(--r-btn); background: var(--accent-fill); color: var(--text-on-red); font: var(--t-callout); font-weight: 600; }
	.retry:hover { background: var(--accent-fill-hover); }
	@keyframes sil-pop { from { opacity: 0; } }
</style>
