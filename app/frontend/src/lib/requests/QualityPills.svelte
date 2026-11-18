<script lang="ts">
	// The Quality row: one radio tile per line of what people can ask for. The same pills on the request sheets and in the approver's request window.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { arrowPill, type QualityPill } from './quality-row.ts';

	let { pills, selected = $bindable(), stacked = false, testid = 'quality-row' }: { pills: QualityPill[]; selected: string; /** The label sits above the buttons (sheets), not beside them. */ stacked?: boolean; testid?: string } = $props();
	const id = $props.id();
	// One tab stop for the row (the picked tile, else the first that can be picked); arrows move the pick, as in any radio group.
	const stop = $derived(pills.some((p) => p.key === selected && !p.disabled) ? selected : pills.find((p) => !p.disabled)?.key);
	let box: HTMLElement | undefined = $state();
	function onkey(e: KeyboardEvent) {
		const rtl = getComputedStyle(e.currentTarget as Element).direction === 'rtl';
		const k = rtl && e.key === 'ArrowLeft' ? 'ArrowRight' : rtl && e.key === 'ArrowRight' ? 'ArrowLeft' : e.key;
		const next = arrowPill(pills, selected, k);
		if (next === null) return;
		e.preventDefault();
		selected = next;
		box?.querySelector<HTMLElement>(`[data-quality="${CSS.escape(next)}"]`)?.focus();
	}
</script>

<div class="rules" class:stacked role="radiogroup" aria-labelledby="{id}-q" data-testid={testid}>
	<span class="rl" id="{id}-q">{t('requests.window.quality')}</span>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="pills" bind:this={box} onkeydown={onkey} style="--n: {Math.min(pills.length, 3)}">
		{#each pills as p (p.key)}
			<button type="button" class="rule" class:off={p.disabled} role="radio" aria-checked={selected === p.key} tabindex={p.key === stop ? 0 : -1} disabled={p.disabled} data-quality={p.key} onclick={() => (selected = p.key)}>
				{#if p.disabled}<Icon name="check" size={14} />{/if}<span class="nm">{p.label}</span>{#if p.note}<span class="dim">{#if !stacked}·{' '}{/if}{p.note}</span>{/if}
			</button>
		{/each}
	</div>
</div>

<style>
	.rules { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.pills { display: flex; flex-wrap: wrap; gap: 6px; }
	.rl { margin-right: 6px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.stacked { flex-direction: column; align-items: flex-start; gap: var(--s-2); margin-block: var(--s-4); }
	.stacked .rl { margin: 0; color: var(--text); font: var(--t-callout); font-weight: 600; }
	/* On the sheets: equal boxes, as many to a row as fit, the note under the name. */
	.stacked { align-items: stretch; flex-wrap: nowrap; flex: none; }
	/* Up to three equal tiles to a row; a short last row sits centred under the full ones instead of hugging the start edge. */
	.stacked .pills { display: flex; flex-wrap: wrap; justify-content: center; gap: var(--s-2); }
	.stacked .rule { flex: 0 1 calc((100% - (var(--n) - 1) * var(--s-2)) / var(--n)); flex-direction: column; align-items: center; justify-content: center; gap: 2px; min-width: min(100%, 96px); min-height: 56px; padding: var(--s-2); border-radius: 12px; text-align: center; transition: background-color 150ms var(--ease-out), color 150ms var(--ease-out), transform 150ms var(--ease-out); }
	/* Names wrap (they are at most 30 letters) instead of being cut. */
	.stacked .nm, .stacked .dim { white-space: normal; overflow-wrap: break-word; hyphens: auto; line-height: 1.3; text-wrap: balance; }
	.stacked .rule:active:not(:disabled) { transform: scale(0.96); }
	.stacked .rule :global(svg) { display: none; }
	.stacked .dim { font: var(--t-footnote); }
	.nm, .dim { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
	.rule { display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding: 0 12px; border: 0; border-radius: var(--r-pill); background: var(--surface-2); color: inherit; font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: background 200ms var(--ease-out), color 200ms var(--ease-out); }
	@media (hover: hover) and (pointer: fine) {
		.rule:hover:not(:disabled):not([aria-checked='true']) { background: var(--surface-3); }
	}
	.rule[aria-checked='true'] { background: var(--text); color: var(--bg); }
	.rule:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.rule[aria-checked='true']:focus-visible { box-shadow: 0 0 0 2px var(--bg), 0 0 0 4px var(--text); }
	.rule.off { background: transparent; box-shadow: inset 0 0 0 1px var(--line-strong); color: var(--text-muted); cursor: default; }
	.dim { opacity: 0.75; font-weight: 400; }
	@media (max-width: 767px) {
		.rules:not(.stacked) { flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; overscroll-behavior-x: contain; margin: 0 -16px; padding: 0 16px; }
		.rules:not(.stacked)::-webkit-scrollbar { display: none; }
		.rules:not(.stacked) .pills { flex-wrap: nowrap; }
		.rule, .rl { flex: none; white-space: nowrap; }
	}
	@media (pointer: coarse) {
		.rule { min-height: 44px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.rule, .stacked .rule { transition: none; }
		.stacked .rule:active:not(:disabled) { transform: none; }
	}
</style>
