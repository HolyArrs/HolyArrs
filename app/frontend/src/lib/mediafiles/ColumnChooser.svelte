<!-- Popover to show, hide and reorder table columns. -->
<script lang="ts">
	import Popover from '#lib/ui/Popover.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { moveColumn, type Column } from './table.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let { columns, onchange, onreset }: { columns: Column[]; onchange: (c: Column[]) => void; onreset: () => void } = $props();

	let dragFrom = $state<number | null>(null);
	let dragOver = $state<number | null>(null);
	let list = $state<HTMLElement | null>(null);
	let said = $state('');

	function toggle(i: number) {
		onchange(columns.map((c, j) => (j === i ? { ...c, on: !c.on } : c)));
	}
	function drop(i: number) {
		if (dragFrom !== null) onchange(moveColumn(columns, dragFrom, i));
		dragFrom = dragOver = null;
	}
	// Arrow keys reorder for people who cannot drag; focus follows the moved row.
	function handleKey(e: KeyboardEvent, i: number) {
		const to = e.key === 'ArrowUp' ? i - 1 : e.key === 'ArrowDown' ? i + 1 : -1;
		if (to < 0 || to >= columns.length) return;
		e.preventDefault();
		onchange(moveColumn(columns, i, to));
		said = t('mediafiles.columns.moved', { label: columns[i].label, place: to + 1, total: columns.length });
		requestAnimationFrame(() => list?.querySelectorAll<HTMLElement>('.grip')[to]?.focus());
	}
</script>

<Popover label={t('mediafiles.columns.title')} icon="eye" text={t('mediafiles.columns.title')} variant="button" width={260} align="end">
	{#snippet children()}
		<div class="chooser">
			<p class="head">{t('mediafiles.columns.title')}</p>
			<ul bind:this={list}>
				{#each columns as c, i (c.id)}
					<li
						class:over={dragOver === i && dragFrom !== i}
						class:dragging={dragFrom === i}
						draggable="true"
						ondragstart={(e) => { dragFrom = i; e.dataTransfer?.setData('text/plain', c.id); }}
						ondragover={(e) => { e.preventDefault(); dragOver = i; }}
						ondragleave={() => (dragOver = dragOver === i ? null : dragOver)}
						ondrop={(e) => { e.preventDefault(); drop(i); }}
						ondragend={() => (dragFrom = dragOver = null)}
					>
						<label><input type="checkbox" checked={c.on} onchange={() => toggle(i)} /> <span>{c.label}</span></label>
						<button type="button" class="grip" aria-label={t('mediafiles.columns.move', { label: c.label })} onkeydown={(e) => handleKey(e, i)}><Icon name="menu" size={15} /></button>
					</li>
				{/each}
			</ul>
			<button type="button" class="reset" onclick={onreset}>{t('mediafiles.columns.reset')}</button>
			<span class="sr-only" aria-live="polite">{said}</span>
		</div>
	{/snippet}
</Popover>

<style>
	.chooser { display: flex; flex-direction: column; gap: 4px; }
	.head { margin: 2px 8px 6px; font: var(--t-footnote); font-weight: 600; color: var(--text-muted); }
	ul { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; }
	li { display: flex; align-items: center; gap: 4px; min-height: 36px; padding: 0 2px 0 8px; border-radius: 8px; cursor: grab; }
	li:hover { background: var(--surface-3); }
	li.over { box-shadow: inset 0 2px 0 var(--accent); }
	li.dragging { opacity: .45; }
	label { flex: 1; display: flex; align-items: center; gap: 10px; min-width: 0; font: var(--t-callout); cursor: pointer; }
	input { width: 16px; height: 16px; accent-color: var(--accent); }
	.grip { display: grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: grab; }
	.grip:focus-visible, .reset:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Sticky so Reset stays reachable when the list scrolls. */
	.reset { margin-top: 4px; padding: 8px; border: 0; border-top: 1px solid var(--line); color: var(--accent); font: var(--t-callout); text-align: left; cursor: pointer; position: sticky; bottom: -6px; margin-bottom: -6px; padding-bottom: 14px; background: var(--surface-1); z-index: 1; }
</style>
