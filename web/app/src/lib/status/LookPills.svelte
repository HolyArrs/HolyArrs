<script lang="ts">
	// The preview picker at the bottom of Status. On the Status tab it picks
	// the shape of the parts (?parts=), on Jobs the shape of Coming up and
	// Recent runs (?runs=). Preview only; it goes once Luis has chosen.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { viewOf } from './status.ts';

	const parts = ['Lattice', 'Tiles', 'Ledger', 'Rail', 'Three across'];
	const runs = ['Ledger', 'Rail', 'Countdown', 'Buckets', 'Timeline'];
	const onJobs = $derived(page.url.searchParams.get('tab') === 'jobs');
	const key = $derived(onJobs ? 'runs' : 'parts');
	const labels = $derived(onJobs ? runs : parts);
	const now = $derived(viewOf(page.url.searchParams.get(key)));

	function pick(n: number) {
		const u = new URL(page.url.href);
		u.searchParams.set(key, String(n));
		void goto(u.pathname + u.search);
	}
</script>

{#if page.url.searchParams.get('tab') === 'jobs' || !page.url.searchParams.get('tab')}
	<nav class="pills" aria-label={onJobs ? 'Runs view' : 'Parts view'} data-testid="status-look-pills">
		<span class="k">{onJobs ? 'Runs' : 'Parts'}</span>
		{#each labels as l, i (l)}
			<button type="button" class="pl" aria-pressed={now === i + 1} onclick={() => pick(i + 1)}>{i + 1} · {l}</button>
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
	}
	.k { padding: 0 8px 0 12px; font: 600 12px/1 var(--font-ui); color: var(--text-muted); white-space: nowrap; }
	.pl { height: 38px; padding: 0 14px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.pl:hover { color: var(--text); }
	.pl[aria-pressed='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.pl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) { .pills { bottom: calc(var(--tabbar) + 10px + env(safe-area-inset-bottom, 0px)); } .pl { padding: 0 11px; font-size: 12.5px; } }
</style>
