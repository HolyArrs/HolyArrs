<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { COL } from './columns.ts';
	import type { Columns } from './columns-grid.svelte.ts';

	let { cols }: { cols: Columns } = $props();
</script>

{#if cols.ghost}<div class="ghost" style="left: {cols.ghost.x}px; top: {cols.ghost.y}px" aria-hidden="true" data-testid="dl-ghost">{COL[cols.ghost.id].label}</div>{/if}

{#if cols.pop}
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<div class="picker" role="dialog" aria-label={t('downloads.col.columns')} tabindex="-1" bind:this={cols.pickEl} style="left: {cols.pop.x}px; top: {cols.pop.y}px" data-testid="dl-picker" onkeydown={cols.pickerKey}>
		<p class="ph-t">{t('downloads.col.columns')}</p>
		<ul class="pl">
			{#each cols.layout.order as id (id)}
				{@const c = COL[id]}
				<li data-id={id} class:held={cols.held === id} class:off={c.missing}>
					<button type="button" class="hd" data-handle={id} aria-label={t('downloads.col.move', { label: c.label })} aria-pressed={cols.held === id} aria-describedby="dl-pick-help" disabled={c.missing} onkeydown={(e) => cols.handleKey(e, id)} onpointerdown={(e) => cols.handleDown(e, id)}><svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true"><g fill="currentColor"><circle cx="5" cy="3" r="1.2"/><circle cx="9" cy="3" r="1.2"/><circle cx="5" cy="7" r="1.2"/><circle cx="9" cy="7" r="1.2"/><circle cx="5" cy="11" r="1.2"/><circle cx="9" cy="11" r="1.2"/></g></svg></button>
					<label class="pkl"><input type="checkbox" checked={!c.missing && !cols.layout.hidden.includes(id)} disabled={c.missing} onchange={() => cols.flip(id)} data-testid="dl-pick-{id}" /><span>{c.label}</span>{#if c.missing}<span class="nr">{t('downloads.col.notReported')}</span>{/if}</label>
				</li>
			{/each}
		</ul>
		<p id="dl-pick-help" class="help">{t('downloads.col.help')}</p>
		<div class="pf"><button type="button" class="lk" onclick={cols.resetAll} data-testid="dl-pick-reset">{t('downloads.col.reset')}</button></div>
		<span class="sr" aria-live="polite">{cols.said}</span>
	</div>
{/if}

<style>
	.lk:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.ghost { position: fixed; z-index: 1000; transform: translate(-50%, -130%); padding: 3px 10px; border-radius: var(--r-sm); background: var(--surface-1); box-shadow: 0 4px 14px rgb(0 0 0 / 0.25); color: var(--text); font: var(--t-caption); font-weight: 600; pointer-events: none; white-space: nowrap; }
	.picker { position: fixed; z-index: 1000; width: 260px; max-height: min(440px, calc(100vh - 16px)); overflow: auto; padding: 8px; border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--surface-1); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28); }
	.ph-t { margin: 2px 6px 6px; color: var(--text-muted); font: var(--t-caption); font-weight: 600; }
	.pl { list-style: none; margin: 0; padding: 0; }
	.pl li { display: flex; align-items: center; gap: 4px; height: 30px; border-radius: 6px; }
	.pl li.held { background: var(--surface-3); }
	.hd { display: grid; place-items: center; width: 24px; height: 26px; padding: 0; border: 0; border-radius: 5px; background: none; color: var(--text-muted); cursor: grab; touch-action: none; }
	.hd:disabled { opacity: 0.3; cursor: default; }
	.hd:focus-visible, .pkl input:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pkl { display: flex; flex: 1; align-items: center; gap: 8px; min-width: 0; color: var(--text); font: var(--t-footnote); cursor: pointer; }
	.pkl > span:first-of-type { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.off .pkl { color: var(--text-muted); cursor: default; }
	.nr { margin-left: auto; padding-right: 4px; color: var(--text-muted); font: var(--t-caption); }
	.help { margin: 6px 6px 0; color: var(--text-muted); font: var(--t-caption); }
	.pf { display: flex; justify-content: flex-end; margin-top: 4px; border-top: 1px solid var(--line); padding-top: 4px; }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.nr { margin: 8px 0 0; color: var(--text-muted); font: var(--t-footnote); }
	.lk { min-height: 28px; padding: 0 4px; border: 0; border-radius: 6px; background: none; color: var(--accent-text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.ghost, .picker { box-shadow: var(--shadow-lift); }
	.picker::-webkit-scrollbar { width: 12px; height: 12px; background: transparent; }
	.picker::-webkit-scrollbar-track { margin: 4px; border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 6%, transparent) padding-box; }
	.picker::-webkit-scrollbar-button { display: none; width: 0; height: 0; }
	.picker::-webkit-scrollbar-thumb { border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 26%, transparent) padding-box; }
	.picker::-webkit-scrollbar-thumb:hover { background: color-mix(in srgb, var(--text) 40%, transparent) padding-box; }
	@media (max-width: 640px) {
		.picker { display: none; }
		.lk { min-height: 32px; }
	}
	@supports not selector(::-webkit-scrollbar) {
		.picker { scrollbar-width: thin; scrollbar-color: color-mix(in srgb, var(--text) 24%, transparent) transparent; }
	}
</style>
