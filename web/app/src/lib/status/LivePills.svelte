<script lang="ts">
	// The preview picker at the bottom of the Jobs tab: the ten shapes of the
	// Timeline plate's right half (?live=). Preview only; it goes once Luis
	// has chosen.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { liveViews, liveViewOf } from './liveviews.ts';

	const onJobs = $derived(page.url.searchParams.get('tab') === 'jobs');
	const now = $derived(liveViewOf(page.url.searchParams.get('live')));

	function pick(id: string) {
		const u = new URL(page.url.href);
		u.searchParams.set('live', id);
		void goto(u.pathname + u.search, { noScroll: true, keepFocus: true });
	}
</script>

{#if onJobs}
	<nav class="pills" aria-label="Right-half view" data-testid="status-live-pills">
		<span class="k">Right half</span>
		{#each liveViews as v, i (v.id)}
			<button type="button" class="pl" aria-pressed={now === v.id} onclick={() => pick(v.id)}>{i + 1} · {v.label}</button>
		{/each}
	</nav>
{/if}

<style>
	.pills {
		position: fixed; left: 50%; z-index: 40; transform: translateX(-50%);
		bottom: calc(16px + env(safe-area-inset-bottom, 0px)); max-width: calc(100vw - 24px); overflow-x: auto;
		display: flex; align-items: center; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-1) 78%, transparent);
		-webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 10px 30px -10px rgb(0 0 0 / 0.45);
		scrollbar-width: none;
	}
	.pills::-webkit-scrollbar { display: none; }
	.k { padding: 0 8px 0 12px; font: 600 12px/1 var(--font-ui); color: var(--text-muted); white-space: nowrap; }
	.pl { height: 36px; padding: 0 12px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 500 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.pl:hover { color: var(--text); }
	.pl[aria-pressed='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.pl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) { .pills { bottom: calc(var(--tabbar) + 10px + env(safe-area-inset-bottom, 0px)); } .pl { padding: 0 10px; font-size: 12.5px; } }
</style>
