<script lang="ts" module>
	// Shared across loaders so aria-busy stays on while any one is busy.
	let busyCount = 0;
	function markBusy(on: boolean): void {
		busyCount = Math.max(0, busyCount + (on ? 1 : -1));
		const main = typeof document === 'undefined' ? null : document.getElementById('main');
		if (!main) return;
		if (busyCount) main.setAttribute('aria-busy', 'true');
		else main.removeAttribute('aria-busy');
	}
</script>

<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Shows a calm loader after a short delay and fades the page in when data arrives.
	import { untrack, type Snippet } from 'svelte';
	import { pageHoldFor, pageLine, pageTimers, PAGE_FADE_MS, type PageStage } from '../core/pageload.ts';

	let {
		loading,
		children,
		status = '',
		label = t('ui.pageLoader.loading'),
		immediate = false,
		cached = false,
		onretry
	}: { loading: boolean; children?: Snippet; status?: string; label?: string; immediate?: boolean; cached?: boolean; onretry?: () => void } = $props();

	let held = $state(false);
	let stage = $state<PageStage>(untrack(() => (loading ? 'hidden' : 'orbit')));
	let start = now();
	let fade = false;
	let content = $state<HTMLElement>();

	const busy = $derived(loading || held);
	const line = $derived(pageLine(stage, status, immediate));

	// Runs before paint so the loader never flashes the old content.
	$effect.pre(() => {
		if (loading) {
			start = now();
			held = false;
			stage = 'hidden';
			const timers = pageTimers(0).map((t) => setTimeout(() => (stage = t.stage), t.at));
			return () => timers.forEach(clearTimeout);
		}
		const wait = untrack(() => pageHoldFor(now() - start, cached));
		fade = untrack(() => stage) !== 'hidden';
		if (!wait) return;
		held = true;
		const t = setTimeout(() => (held = false), wait);
		return () => clearTimeout(t);
	});

	$effect(() => {
		if (!busy) return;
		markBusy(true);
		return () => markBusy(false);
	});

	$effect(() => {
		if (busy || !content || !fade) return;
		fade = false;
		if (matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
		for (const el of Array.from(content.children)) el.animate?.([{ opacity: 0 }, { opacity: 1 }], { duration: PAGE_FADE_MS, easing: 'ease-out' });
	});

	// Falls back to Date when performance is missing.
	function now() {
		return typeof performance === 'undefined' ? Date.now() : performance.now();
	}
</script>

{#if busy}
	<div class="pl" class:on={stage !== 'hidden'} data-testid="page-loader" aria-label={label}>
		{#if stage !== 'hidden'}
			<div class="ow" aria-hidden="true">
				<svg class="ms" viewBox="0 0 100 100"
					><defs><clipPath id="plc"><rect x="17.5" y="17.5" width="65" height="65" rx="4" transform="rotate(45 50 50)" /></clipPath></defs><circle cx="50" cy="50" r="46" fill="none" stroke="currentColor" stroke-opacity=".3" stroke-width="1.6" /><g transform="translate(14 14) scale(.72)"
						><rect x="17.5" y="17.5" width="65" height="65" rx="4" transform="rotate(45 50 50)" fill="none" stroke="#3a3d44" stroke-width="2.2" /><g clip-path="url(#plc)"
							><rect width="100" height="100" fill="#1c1d21" /><path d="M50 0C42 18 46 32 58 41C70 50 66 70 54 100L110 100L110 0Z" fill="#f23a2e" /><path d="M50 0C42 18 46 32 58 41C70 50 66 70 54 100L110 100L110 0Z" fill="#55585e" transform="rotate(180 50 50)" /></g
						><path d="M50 26.8Q54.5 44.3 72 48.8Q54.5 53.3 50 70.8Q45.5 53.3 28 48.8Q45.5 44.3 50 26.8Z" fill="#fff" /></g
					></svg
				><svg class="rs" viewBox="0 0 100 100"><circle cx="50" cy="50" r="46" fill="none" stroke="var(--accent)" stroke-width="1.8" stroke-linecap="round" stroke-dasharray="60 229" /></svg>
			</div>
		{/if}
		<p class="line" role="status" aria-live="polite">{line}</p>
		{#if stage === 'reassure'}
			<p class="more">{t('ui.pageLoader.stillWorkingTheServer')}</p>
			<button type="button" class="retry" onclick={() => (onretry ? onretry() : location.reload())}>{t('ui.pageLoader.tryAgain')}</button>
		{/if}
	</div>
{:else}
	<div class="pl-in" bind:this={content}>{@render children?.()}</div>
{/if}

<style>
	.pl {
		min-height: calc(100dvh - var(--topbar-h, 56px) - 96px);
		display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 14px;
		padding: 24px 16px; text-align: center; color: var(--text-muted);
	}
	.pl.on { animation: pl-in var(--dur-fast) var(--ease-out) both; }
	.pl-in { display: contents; }
	.ow { position: relative; width: 64px; height: 64px; }
	.ow svg { position: absolute; inset: 0; width: 100%; height: 100%; overflow: visible; }
	/* Only this will-change is worth it: the ring spins forever. */
	.rs { animation: pl-turn 1.17s linear infinite; will-change: transform; }
	.ms { animation: pl-breathe 2.6s ease-in-out infinite; }
	.line { margin: 0; min-height: 1.3em; max-width: min(360px, 80vw); font: var(--t-callout); color: var(--text-muted); }
	.more { margin: 0; max-width: min(360px, 80vw); font: var(--t-footnote); color: var(--text-muted); }
	.retry { min-height: 36px; padding: 0 16px; border-radius: var(--r-btn); background: var(--accent-fill); color: var(--text-on-red); font: var(--t-callout); font-weight: 600; }
	.retry:hover { background: var(--accent-fill-hover); }
	@keyframes pl-turn { to { transform: rotate(360deg); } }
	@keyframes pl-breathe { 0%, 100% { opacity: .8; } 50% { opacity: 1; } }
	@keyframes pl-in { from { opacity: 0; } }
	@keyframes pl-pulse { 50% { opacity: .4; } }
	@media (prefers-reduced-motion: reduce) {
		.pl.on { animation: none; }
		/* Pulse instead of spin; important beats the base rule. */
		.rs { animation: pl-pulse 1.6s ease-in-out infinite !important; animation-duration: 1.6s !important; animation-iteration-count: infinite !important; }
		.ms { animation: none !important; }
	}
</style>
