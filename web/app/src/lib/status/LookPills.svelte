<script lang="ts">
	// The preview switch: a floating glass pill at the bottom of Status that
	// moves between the three looks (?look=). It follows the address.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { looks, lookOf } from './status.ts';

	const now = $derived(lookOf(page.url.searchParams.get('look')));

	function pick(key: string) {
		const u = new URL(page.url.href);
		u.searchParams.set('look', key);
		u.searchParams.delete('tab');
		void goto(u.pathname + u.search);
	}
</script>

<nav class="pills" aria-label="Status look" data-testid="status-look-pills">
	{#each looks as l (l.key)}
		<button type="button" class="pl" aria-pressed={now === l.key} data-look={l.key} onclick={() => pick(l.key)}>{l.label}</button>
	{/each}
</nav>

<style>
	.pills {
		position: fixed; left: 50%; z-index: 40; transform: translateX(-50%);
		bottom: calc(16px + env(safe-area-inset-bottom, 0px));
		display: flex; gap: 2px; padding: 3px; max-width: calc(100vw - 32px); box-sizing: border-box;
		border: 1px solid var(--line); border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-1) 72%, transparent);
		-webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 10px 30px -10px rgb(0 0 0 / 0.45);
	}
	.pl {
		flex: 1 1 auto; height: 38px; min-width: 0; padding: 0 16px; border: 0; border-radius: var(--r-pill);
		background: none; color: var(--text-muted); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer;
		transition: color 200ms var(--ease-out), background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out);
	}
	.pl:hover { color: var(--text); }
	.pl[aria-pressed='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	.pl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .pl { height: 44px; } }
	@media (max-width: 767px) {
		.pills { bottom: calc(var(--tabbar) + 10px + env(safe-area-inset-bottom, 0px)); }
		.pl { padding: 0 12px; }
	}
	@media (prefers-reduced-motion: reduce) { .pl { transition: none; } }
</style>
