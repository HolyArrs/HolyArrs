<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Icon from '../Icon.svelte';
	import { appPath, to } from '../paths.ts';
	import { registerField, searchField, searchHooks } from '../search-field.svelte.ts';

	// The one search field (spec: top bar, centre). Typing or Enter anywhere
	// opens the Search page with the words; a focus or click on an empty field
	// opens it on Browse. Escape clears the words, a second Escape leaves.
	let { searchKey, testid = 'search-box', id = 'q' }: { searchKey: string; testid?: string; id?: string } = $props();

	let el = $state<HTMLInputElement>();
	$effect(() => (el ? registerField(el) : undefined));
	const onSearch = () => appPath(page.url.pathname).startsWith('/search');

	function open(q: string) {
		if (onSearch()) return;
		const t = q.trim();
		void goto(to(t ? `/search?q=${encodeURIComponent(t)}` : '/search'), { reset: false }).then(() => scrollTo(0, 0));
	}
	function typed() {
		open(searchField.term);
	}
	function submitted(e: SubmitEvent) {
		e.preventDefault();
		if (onSearch()) searchHooks.submit?.();
		else open(searchField.term);
	}
	function focused() {
		if (!searchField.term.trim()) open('');
	}
	function keyed(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		if (searchField.term) {
			e.preventDefault();
			searchField.term = '';
		} else el?.blur();
	}
</script>

<form class="ui-search" role="search" onsubmit={submitted}>
	<span class="ic" aria-hidden="true"><Icon name="search" size={18} /></span>
	<label class="sr-only" for={id}>Search for a title, an artist or a person</label>
	<input {id} bind:this={el} type="search" bind:value={searchField.term} oninput={typed} onfocus={focused} onclick={focused} onblur={() => searchHooks.blur?.()} onkeydown={keyed} placeholder="Shows, movies and people" autocomplete="off" spellcheck="false" enterkeyhint="search" data-testid={testid} />
	{#if searchField.term}
		<button type="button" class="clear" aria-label="Clear the search" onclick={() => { searchField.term = ''; el?.focus(); }}><Icon name="x" size={14} /></button>
	{:else}
		<kbd class="hint" aria-hidden="true">{searchKey}</kbd>
	{/if}
</form>

<style>
	.ui-search { position: relative; display: flex; align-items: center; width: clamp(280px, 36vw, 560px); max-width: 100%; border-radius: 999px; box-shadow: 0 8px 24px rgb(0 0 0 / 0.18), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	.ic { position: absolute; z-index: 1; left: 15px; display: grid; color: var(--text-muted); pointer-events: none; }
	input { flex: 1; min-width: 0; height: 44px; padding: 0 64px 0 42px; border: 1px solid transparent; border-radius: 999px; background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); color: var(--text); font: var(--t-body); }
	input::placeholder { color: var(--text-muted); }
	input::-webkit-search-cancel-button { display: none; }
	input:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.hint { position: absolute; right: 14px; padding: 2px 7px; border-radius: 6px; border: 1px solid var(--line); color: var(--text-muted); font: 500 11px/1.4 var(--font-ui); pointer-events: none; }
	.clear { position: absolute; z-index: 1; right: 4px; display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 999px; background: none; color: var(--text-muted); cursor: pointer; }
	.clear:hover { color: var(--text); }
	.clear:focus-visible { outline: none; box-shadow: var(--focus-ring); }
</style>
