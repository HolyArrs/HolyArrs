<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { escStep } from './keys.ts';

	// Downloads search: filters the list as you type. × and Esc clear the words; Ctrl K and "/" focus it (handled by the page).
	let {
		term = $bindable(''),
		shown,
		total,
		searchKey = 'Ctrl K',
		placeholder = undefined,
		note = '',
		testid = 'dl-find'
	}: { term?: string; shown: number; total: number; searchKey?: string; /** The field's words (the Library uses its own). */ placeholder?: string; /** A quiet line at the right, e.g. "36 TV shows". */ note?: string; testid?: string } = $props();

	const ph = $derived(placeholder ?? t('downloads.list.search'));
	let el = $state<HTMLInputElement>();
	const has = $derived(!!term.trim());

	function key(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		e.preventDefault();
		if (escStep(term) === 'clear') term = '';
		else el?.blur();
	}
</script>

<div class="dlfind" class:has data-testid={testid}>
	<span class="well">
		<span class="mag" aria-hidden="true"><Icon name="search" size={16} /></span>
		<input bind:this={el} bind:value={term} type="text" inputmode="search" placeholder={ph} aria-label={ph} autocomplete="off" spellcheck="false" onkeydown={key} data-dl-search data-testid="{testid}-input" />
		{#if note}<span class="note" aria-live="polite">{note}</span>{/if}
		{#if has}
			<button type="button" class="clr" aria-label={t('downloads.list.clearSearch')} onclick={() => { term = ''; el?.focus(); }} data-testid="{testid}-clear"><Icon name="x" size={14} /></button>
		{:else if searchKey}
			<kbd class="kb" aria-hidden="true">{searchKey}</kbd>
		{/if}
	</span>
</div>

<style>
	.dlfind { --ez: cubic-bezier(0.32, 0.72, 0, 1); --ring: color-mix(in srgb, var(--accent) 55%, transparent); display: flex; flex: 1 1 260px; min-width: 220px; height: 52px; box-sizing: border-box; margin: 0; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); transition: border-color 220ms var(--ez), background 220ms var(--ez), box-shadow 260ms var(--ez) }
	.dlfind > *, .well > * { margin: 0; }
	.well { position: relative; display: flex; flex: 1; align-items: center; gap: 8px; min-width: 0; height: 44px; padding: 0 8px 0 12px; border-radius: 10px; transition: background 220ms var(--ez), box-shadow 220ms var(--ez); }
	.mag { display: grid; place-items: center; flex: none; color: var(--text-muted); transition: color 180ms var(--ez); }
	input { flex: 1; min-width: 0; height: 100%; padding: 0; border: 0; background: none; color: var(--text); font: var(--t-callout); outline: none; }
	input::placeholder { color: var(--text-muted); }
	.note { flex: none; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.kb { flex: none; padding: 2px 7px; border: 1px solid var(--line); border-radius: 6px; color: var(--text-muted); font: 500 11px/1.4 var(--font-ui); white-space: nowrap; transition: opacity 160ms var(--ez); }
	.clr { display: grid; place-items: center; flex: none; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 8px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); cursor: pointer; animation: pop 160ms var(--ez); }
	.clr:hover { background: color-mix(in srgb, var(--text) 14%, transparent); }
	.clr:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.dlfind:focus-within .mag { color: var(--text); }
	@keyframes pop { from { opacity: 0; transform: scale(0.8); } }

	.dlfind:hover { border-color: color-mix(in srgb, var(--line-strong) 60%, var(--line)); }
	.dlfind:focus-within { border-color: var(--line-strong); background: color-mix(in srgb, var(--surface-1) 86%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.08), 0 0 0 3px color-mix(in srgb, var(--text) 6%, transparent), 0 8px 24px -12px rgb(0 0 0 / 0.4); }
	.dlfind:focus-within .kb { opacity: 0.5; }

	@media (prefers-reduced-motion: reduce) {
		.dlfind, .well, .mag, .kb { transition: none; }
		.clr { animation: none; }
	}
</style>
