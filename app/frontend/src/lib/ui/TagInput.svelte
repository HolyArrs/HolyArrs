<script lang="ts">
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
	const offered = $derived(known.filter((t) => !value.includes(t)));

	onMount(() => {
		void tagsInUse().then((ts) => (known = ts.map((t) => t.label)));
	});

	function commit() {
		if (!text.trim()) return;
		value = addTags(value, text);
		text = '';
	}
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

<div class="ti" data-testid={testid}>
	{#if value.length}
		<div class="chips" role="group" aria-label="Tags">
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
		placeholder={value.length ? 'Add another tag' : 'Type a tag, then Enter'}
		aria-describedby={describedby}
		data-testid="{testid}-input"
	/>
	<datalist id="{id}-known">{#each offered as t (t)}<option value={t}></option>{/each}</datalist>
</div>

<style>
	.ti { display: grid; gap: 8px; }
	.chips { display: flex; flex-wrap: wrap; gap: 6px; }
</style>
