<script lang="ts">
	// The preview switch: a floating glass pill at the bottom of History that
	// moves between the looks (?look=). It follows the address, so Back
	// returns to the look before.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { actsList, actsOf, looks, lookOf } from './dirs.ts';

	const now = $derived(lookOf(page.url.searchParams.get('look')));

	const acts = $derived(actsOf(page.url.searchParams.get('acts')));
	const search = $derived(now === 'plain' || now === 'pill');

	function pickActs(key: string) {
		const u = new URL(page.url.href);
		if (key === 'find') u.searchParams.delete('acts');
		else u.searchParams.set('acts', key);
		void goto(u.pathname + u.search, { reset: false });
	}

	function pick(key: string) {
		const u = new URL(page.url.href);
		if (key === 'plain') u.searchParams.delete('look');
		else u.searchParams.set('look', key);
		// Each look reads its own filters; only the look itself carries over.
		for (const k of ['title', 'episode', 'offset', 'q', 'step', 'type']) u.searchParams.delete(k);
		void goto(u.pathname + u.search, { reset: false });
	}
</script>

{#if search}
	<div class="pills acts" role="group" aria-label="Actions" data-testid="history-acts-pills">
		<span class="al" aria-hidden="true">Actions</span>
		{#each actsList as a (a.key)}
			<button type="button" class="pl" aria-pressed={acts === a.key} aria-label="Actions: {a.label}" data-acts={a.key} onclick={() => pickActs(a.key)}>{a.label}</button>
		{/each}
	</div>
{/if}
<nav class="pills" aria-label="History look" data-testid="history-look-pills">
	{#each looks as l (l.key)}
		<button type="button" class="pl" aria-pressed={now === l.key} data-look={l.key} onclick={() => pick(l.key)}>{l.label}</button>
	{/each}
</nav>

<style>
	.pills {
		position: fixed; left: 50%; z-index: 40; transform: translateX(-50%);
		bottom: calc(16px + env(safe-area-inset-bottom, 0px));
		display: flex; gap: 2px; padding: 3px; max-width: calc(100vw - 32px); box-sizing: border-box; overflow-x: auto; scrollbar-width: none; overscroll-behavior-x: contain;
		border: 1px solid var(--line); border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-1) 72%, transparent);
		-webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 10px 30px -10px rgb(0 0 0 / 0.45);
	}
	/* The Actions switch sits just above the look pills. */
	.acts { bottom: calc(16px + 44px + env(safe-area-inset-bottom, 0px)); align-items: center; }
	.al { padding: 0 8px 0 10px; color: var(--text-muted); font: 500 12px/1 var(--font-ui); white-space: nowrap; }
	.acts .pl { height: 30px; padding: 0 12px; font-size: 13px; }
	.pills::-webkit-scrollbar { display: none; }
	.pl {
		flex: 1 0 auto; height: 34px; min-width: 0; padding: 0 16px; border: 0; border-radius: var(--r-pill);
		background: none; color: var(--text-muted); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer;
		transition: color 200ms var(--ease-out), background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out);
	}
	.pl:hover { color: var(--text); }
	.pl[aria-pressed='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	.pl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .pl { height: 38px; } .acts .pl { height: 34px; } .acts { bottom: calc(16px + 50px + env(safe-area-inset-bottom, 0px)); } }
	/* Phones: above the tab bar; six looks scroll sideways inside the pill at 360px. */
	@media (max-width: 767px) {
		.pills { bottom: calc(var(--tabbar) + 10px + env(safe-area-inset-bottom, 0px)); }
		.pl { padding: 0 12px; }
		.acts { bottom: calc(var(--tabbar) + 10px + 50px + env(safe-area-inset-bottom, 0px)); }
	}
	@media (prefers-reduced-motion: reduce) { .pl { transition: none; } }
</style>
