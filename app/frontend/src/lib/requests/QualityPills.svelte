<script lang="ts">
	// The Quality row: one button per line of what people can ask for. The same pills on the request sheets and in the approver's request window.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import type { QualityPill } from './quality-row.ts';

	let { pills, selected = $bindable(), stacked = false, testid = 'quality-row' }: { pills: QualityPill[]; selected: string; /** The label sits above the buttons (sheets), not beside them. */ stacked?: boolean; testid?: string } = $props();
	const id = $props.id();
</script>

<div class="rules" class:stacked role="group" aria-labelledby="{id}-q" data-testid={testid}>
	<span class="rl" id="{id}-q">{t('requests.window.quality')}</span>
	<div class="pills">
		{#each pills as p (p.key)}
			<button type="button" class="rule" class:off={p.disabled} aria-pressed={selected === p.key} disabled={p.disabled} data-quality={p.key} onclick={() => (selected = p.key)}>
				{#if p.disabled}<Icon name="check" size={14} />{/if}{p.label}{#if p.note}<span class="dim">· {p.note}</span>{/if}
			</button>
		{/each}
	</div>
</div>

<style>
	.rules { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.pills { display: flex; flex-wrap: wrap; gap: 6px; }
	.rl { margin-right: 6px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.stacked { flex-direction: column; align-items: flex-start; gap: 10px; margin-top: var(--s-4); }
	.stacked .rl { margin: 0; color: var(--text); font: var(--t-callout); font-weight: 600; }
	.rule { display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding: 0 12px; border: 0; border-radius: var(--r-pill); background: var(--surface-2); color: inherit; font: var(--t-subhead); font-weight: 600; cursor: pointer; transition: background 200ms var(--ease-out), color 200ms var(--ease-out); }
	.rule:hover:not(:disabled):not([aria-pressed='true']) { background: var(--surface-3); }
	.rule[aria-pressed='true'] { background: var(--text); color: var(--bg); }
	.rule:focus-visible { outline: none; box-shadow: var(--focus-ring); }
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
		.rule { transition: none; }
	}
</style>
