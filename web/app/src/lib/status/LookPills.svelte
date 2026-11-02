<script lang="ts">
	// The preview picker at the bottom of Status: section surface, raised (1)
	// or frosted (3). It follows the address (?surface=).
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { surfaceOf } from './status.ts';

	const choices = [
		{ n: 1, label: 'Raised' },
		{ n: 3, label: 'Frosted' }
	];
	const now = $derived(surfaceOf(page.url.searchParams.get('surface')));

	function pick(n: number) {
		const u = new URL(page.url.href);
		u.searchParams.set('surface', String(n));
		void goto(u.pathname + u.search);
	}
</script>

<nav class="pills" aria-label="Surface" data-testid="status-look-pills">
	{#each choices as c (c.n)}
		<button type="button" class="pl" aria-pressed={now === c.n} onclick={() => pick(c.n)}>{c.n} · {c.label}</button>
	{/each}
</nav>

<style>
	.pills {
		position: fixed; left: 50%; z-index: 40; transform: translateX(-50%);
		bottom: calc(16px + env(safe-area-inset-bottom, 0px));
		display: flex; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-1) 78%, transparent);
		-webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 10px 30px -10px rgb(0 0 0 / 0.45);
	}
	.pl { height: 38px; padding: 0 16px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.pl:hover { color: var(--text); }
	.pl[aria-pressed='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.pl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) { .pills { bottom: calc(var(--tabbar) + 10px + env(safe-area-inset-bottom, 0px)); } }
</style>
