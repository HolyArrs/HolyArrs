<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Chip list with a text box that turns typed words into tags.
	import { onMount } from 'svelte';
	import Chip from './Chip.svelte';
	import { addTags, tagsInUse } from './tags.ts';

	let {
		id,
		value = $bindable([]),
		disabled = false,
		describedby,
		testid = 'tags'
	}: { id: string; value?: string[]; disabled?: boolean; describedby?: string; testid?: string } = $props();

	let text = $state('');
	let known = $state<string[]>([]);
	// suggestions skip tags already chosen
	const offered = $derived(known.filter((t) => !value.includes(t)));

	onMount(() => {
		void tagsInUse().then((ts) => (known = ts.map((t) => t.label)));
	});

	// Blur and comma also commit so a half typed tag is never lost.
	function commit() {
		if (!text.trim()) return;
		value = addTags(value, text);
		text = '';
	}
	// isComposing guard stops Enter from committing mid IME input.
	function key(e: KeyboardEvent) {
		if (e.isComposing) return;
		if (e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			commit();
		} else if (e.key === 'Backspace' && !text && value.length) {
			value = value.slice(0, -1);
		}
	}
</script>

<!-- datalist below offers tags already used elsewhere -->
<div class="ti" data-testid={testid}>
	{#if value.length}
		<div class="chips" role="group" aria-label={t('ui.tagInput.tags')}>
			{#each value as t (t)}<Chip label={t} onremove={disabled ? undefined : () => (value = value.filter((x) => x !== t))} />{/each}
		</div>
	{/if}
	<input
		{id}
		class="input"
		type="text"
		bind:value={text}
		onkeydown={key}
		oninput={() => {
			if (text.includes(',')) commit();
		}}
		onblur={commit}
		{disabled}
		list="{id}-known"
		autocomplete="off"
		spellcheck="false"
		maxlength="60"
		placeholder={value.length ? t('ui.tagInput.addAnother') : t('ui.tagInput.typeATag')}
		aria-describedby={describedby}
		data-testid="{testid}-input"
	/>
	<datalist id="{id}-known">{#each offered as t (t)}<option value={t}></option>{/each}</datalist>
</div>

<style>
	.ti { display: grid; gap: 8px; }
	.chips { display: flex; flex-wrap: wrap; gap: 6px; }
</style>
