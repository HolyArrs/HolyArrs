<!-- Shared page frame for library tool pages: title bar, count and styled slots. -->
<script lang="ts">
	import type { Snippet } from 'svelte';
	let { title, count = '', bar, children }: { title: string; count?: string; bar?: Snippet; children: Snippet } = $props();
</script>

<div class="tp">
	<div class="tp-bar" role="toolbar" aria-label={title}>
		<h1 class="tp-name">{title}{#if count}<span class="tp-count">{count}</span>{/if}</h1>
		{@render bar?.()}
	</div>
	{@render children()}
</div>

<style>
	/* Page children are styled through :global so each tool page can reuse these class names. */
	.tp { display: flex; flex-direction: column; gap: 14px; padding-bottom: 48px; min-width: 0;
		--sheet-a: light-dark(#ffffff, #212126); --sheet-b: light-dark(#f6f7fa, #17171a);
		--edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 6%));
		--drop: 0 1px 2px light-dark(rgb(20 24 40 / 5%), rgb(0 0 0 / 40%)), 0 18px 32px -20px light-dark(rgb(20 24 40 / 30%), rgb(0 0 0 / 85%)); }


	.tp-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
	.tp-name { display: inline-flex; align-items: baseline; gap: 10px; min-width: 0; margin: 0 8px 0 0; font: 700 30px/1.2 var(--font-display, var(--font-ui)); letter-spacing: -0.022em; white-space: nowrap; }
	.tp-count { color: var(--text-muted); font: 500 14px/1 var(--font-ui); letter-spacing: 0; font-variant-numeric: tabular-nums; }
	.tp :global(.tp-gap) { flex: 1 1 auto; }
	/* Control capsule; the 44px children keep touch targets large. */
	.tp :global(.tp-cap) { display: flex; flex: none; align-items: center; gap: 2px; min-width: 0; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.tp :global(.tp-cap:empty) { display: none; }
	.tp :global(.tp-cap .ui-btn.secondary), .tp :global(.tp-cap .trigger.button) { height: 44px; min-height: 44px; padding-inline: 12px; border-radius: 10px; background: none; box-shadow: none; }
	.tp :global(.tp-cap .ui-btn.primary), .tp :global(.tp-cap .ui-btn.destructive) { height: 44px; min-height: 44px; border-radius: 10px; }
	.tp :global(.tp-cap .ui-btn.secondary:hover:not(:disabled)), .tp :global(.tp-cap .trigger.button:hover), .tp :global(.tp-cap .trigger.button[aria-expanded='true']) { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.tp :global(.tp-cap .ui-ibtn) { width: 44px; height: 44px; border-radius: 10px; }
	.tp :global(.tp-cap .tp-lab) { padding: 0 6px 0 12px; color: var(--text-muted); font: 500 13px/1 var(--font-ui); white-space: nowrap; }
	.tp :global(.tp-cap .tp-val) { padding: 0 12px; font: 600 13.5px/1 var(--font-ui); font-variant-numeric: tabular-nums; white-space: nowrap; }

	.tp :global(.tp-cap select), .tp :global(.tp-cap input.input) { height: 44px; min-height: 44px; border: 0; border-radius: 10px; background-color: transparent; box-shadow: none; font: 600 13.5px/1 var(--font-ui); color: var(--text); }
	.tp :global(.tp-cap select) { min-width: 0; max-width: 100%; padding: 0 30px 0 10px; cursor: pointer; text-overflow: ellipsis; }
	.tp :global(.tp-cap select:hover), .tp :global(.tp-cap input.input:hover) { background-color: color-mix(in srgb, var(--text) 7%, transparent); }
	.tp :global(.tp-cap select:focus-visible), .tp :global(.tp-cap input.input:focus-visible) { outline: none; box-shadow: var(--focus-ring); }


	.tp :global(.tp-sheet) { position: relative; isolation: isolate; min-width: 0; border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b) 420px); box-shadow: var(--edge), var(--drop); overflow: clip; }
	/* Faint noise keeps the large gradient from banding. */
	.tp :global(.tp-sheet)::before { content: ''; position: absolute; inset: 0; z-index: -1; pointer-events: none; opacity: .05; background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='180' height='180'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 .5 0 0 0 0 .5 0 0 0 0 .5 0 0 0 1 0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E"); }
	.tp :global(.tp-pad) { padding: 16px 20px; }


	.tp :global(.tp-table) { width: 100%; border-collapse: collapse; font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.tp :global(.tp-table th) { height: 40px; padding: 0 12px; text-align: left; white-space: nowrap; color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; background: color-mix(in srgb, var(--text) 3%, transparent); border-bottom: 1px solid var(--line); }
	.tp :global(.tp-table td) { padding: 12px; vertical-align: middle; }
	.tp :global(.tp-table th:first-child), .tp :global(.tp-table td:first-child) { padding-left: 20px; }
	.tp :global(.tp-table th:last-child), .tp :global(.tp-table td:last-child) { padding-right: 20px; }
	.tp :global(.tp-table tbody tr + tr td) { box-shadow: inset 0 1px 0 var(--line); }
	.tp :global(.tp-table tbody tr) { transition: background-color 120ms ease; }
	.tp :global(.tp-table tbody tr:hover) { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.tp :global(.tp-table a) { color: var(--text); text-decoration: none; font-weight: 600; }
	.tp :global(.tp-table a:hover) { text-decoration: underline; text-underline-offset: 3px; }

	.tp :global(.tp-art) { flex: none; width: 36px; border-radius: 5px; overflow: hidden; box-shadow: 0 0 0 1px var(--line); }
	.tp :global(.tp-who) { display: flex; align-items: center; gap: 12px; min-width: 0; }
	.tp :global(.tp-who > .tp-tx) { min-width: 0; }
	.tp :global(.tp-sub) { display: block; margin-top: 2px; color: var(--text-muted); font: var(--t-footnote); }


	.tp :global(.tp-rows) { margin: 0; padding: 0; list-style: none; }
	.tp :global(.tp-rows > *) { position: relative; padding: 14px 20px; transition: background-color 120ms ease; }
	.tp :global(.tp-rows > * + *)::before { content: ''; position: absolute; top: 0; left: 20px; right: 20px; height: 1px; background: var(--line); }
	.tp :global(.tp-rows > *:hover) { background: color-mix(in srgb, var(--text) 4%, transparent); }


	.tp :global(.tp-note) { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	.tp :global(.tp-msg) { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 14px; border-radius: 12px; background: color-mix(in srgb, var(--text) 5%, transparent); box-shadow: inset 0 0 0 1px var(--line); font: var(--t-callout); }
	.tp :global(.tp-msg.bad) { color: var(--fail-text, var(--fail)); }
	.tp :global(.tp-msg.good svg) { color: var(--ok); }
	.tp :global(.tp-foot) { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; }

	@media (max-width: 639px) {
		.tp-name { flex: 1 1 100%; font-size: 20px; }
		.tp :global(.tp-cap) { flex: 1 1 auto; }
		.tp :global(.tp-gap) { display: none; }
		.tp :global(.tp-pad) { padding: 14px 16px; }
		.tp :global(.tp-table th:first-child), .tp :global(.tp-table td:first-child) { padding-left: 14px; }
		.tp :global(.tp-table th:last-child), .tp :global(.tp-table td:last-child) { padding-right: 14px; }
		.tp :global(.tp-rows > *) { padding: 14px 16px; }
		.tp :global(.tp-rows > * + *)::before { left: 16px; right: 16px; }
	}
</style>
