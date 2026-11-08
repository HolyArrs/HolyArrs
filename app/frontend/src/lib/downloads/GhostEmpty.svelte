<script lang="ts">
	// Empty state used across Downloads: faint ghost rows in one container, a muted title and one line.
	import type { Snippet } from 'svelte';
	import { fade } from 'svelte/transition';
	import { reducedMotion } from '../core/motion.ts';

	let { title, text = '', compact = false, testid = 'dl-empty', action }: { title: string; text?: string; compact?: boolean; testid?: string; action?: Snippet } = $props();
	const dur = reducedMotion() ? 0 : 180;
</script>

<div class="ge" class:compact data-testid={testid} in:fade={{ duration: dur }}>
	<ul class="box" aria-hidden="true">
		{#each [0.5, 0.78, 0.62] as w, i (i)}
			<li class="row"><i class="ck"></i><i class="pt"></i><i class="b1"></i><i class="b2" style="width: {w * 100}%"></i><i class="pl"></i></li>
		{/each}
	</ul>
	<p class="t">{title}</p>
	{#if text}<p class="x">{text}</p>{/if}
	{#if action}<div class="a">{@render action()}</div>{/if}
</div>

<style>
	.ge { display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: min(340px, 56vh); padding: 32px 16px; box-sizing: border-box; text-align: center; }
	.ge.compact { min-height: 0; padding: 20px 12px; }
	.box { display: flex; flex-direction: column; gap: 8px; width: min(260px, 100%); margin: 0 0 18px; padding: 10px; list-style: none; border-radius: 12px; background: var(--surface-2); }
	.compact .box { width: min(200px, 100%); gap: 6px; padding: 8px; margin-bottom: 12px; }
	.row { display: flex; align-items: center; gap: 8px; height: 26px; padding: 0 8px; border-radius: 7px; background: var(--surface-1); opacity: 0.9; }
	.compact .row { height: 20px; }
	.row i { display: block; flex: none; height: 6px; border-radius: 3px; background: var(--surface-3); }
	.ck { width: 8px; height: 8px !important; border-radius: 2px !important; }
	.pt { width: 10px; height: 14px !important; border-radius: 2px !important; }
	.compact .pt { height: 10px !important; }
	.b1 { width: 22px; }
	.b2 { flex: 1 1 auto !important; max-width: 120px; }
	.pl { width: 18px; height: 8px !important; border-radius: 4px !important; margin-left: auto; }
	.t { margin: 0; color: var(--text-muted); font: var(--t-callout); font-weight: 500; overflow-wrap: anywhere; }
	.x { margin: 3px 0 0; max-width: 36ch; color: var(--text-muted); font: var(--t-footnote); }
	.a { margin-top: 8px; }
	.a :global(button), .a :global(a) { padding: 4px 6px; border: 0; background: none; color: var(--text-muted); font: var(--t-footnote); text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
	.a :global(button:focus-visible), .a :global(a:focus-visible) { outline: 2px solid var(--text-muted); outline-offset: 2px; border-radius: 4px; }
</style>
