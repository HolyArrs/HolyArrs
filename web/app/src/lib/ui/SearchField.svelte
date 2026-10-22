<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Icon from '../Icon.svelte';
	import { appPath, to } from '../paths.ts';
	import { registerField, rememberReturn, searchField, searchHooks } from '../search-field.svelte.ts';
	import { api } from '../api.ts';
	import { createSuggest, step, suggestHref, type SuggestAnswer } from '../live-search.ts';
	import GlassDropdown from './GlassDropdown.svelte';

	// The one search field (spec: top bar, centre). Typing or Enter anywhere
	// opens the Search page with the words; a focus or click on an empty field
	// opens it on Browse. Escape clears the words; a second Escape closes Search
	// (back to the page you came from), elsewhere it leaves the field.
	let { searchKey, testid = 'search-box', id = 'q', closable = true }: { searchKey: string; testid?: string; id?: string; closable?: boolean } = $props();

	let el = $state<HTMLInputElement>();
	$effect(() => (el ? registerField(el) : undefined));
	const onSearch = () => appPath(page.url.pathname).startsWith('/search');

	function open(q: string) {
		if (onSearch()) return;
		const t = q.trim();
		rememberReturn();
		void goto(to(t ? `/search?q=${encodeURIComponent(t)}` : '/search'), { reset: false }).then(() => scrollTo(0, 0));
	}
	// Live search: the glass dropdown under the field (shared by every page).
	let answer = $state<SuggestAnswer | null>(null);
	let active = $state(-1);
	const listId = $derived(`${id}-suggest`);
	const suggest = createSuggest(
		(q, signal) => api.get<SuggestAnswer>(`/search/suggest?q=${encodeURIComponent(q)}${kindParam()}`, signal),
		(a) => {
			answer = a && (a.completions.length || a.items.length) ? a : null;
			active = -1;
		}
	);
	function kindParam() {
		const k = page.url.searchParams.get('kind');
		return appPath(page.url.pathname).startsWith('/search') && k && ['movie', 'show', 'music', 'people'].includes(k) ? `&kind=${k}` : '';
	}
	const count = $derived(answer ? answer.completions.length + answer.items.length : 0);
	function close() {
		suggest.clear();
	}
	function pick(href: string) {
		close();
		el?.blur();
		void goto(href);
	}
	function word(w: string) {
		searchField.term = w;
		close();
		if (onSearch()) searchHooks.submit?.();
		else open(w);
	}
	function typed() {
		open(searchField.term);
		suggest.type(searchField.term);
	}
	function submitted(e: SubmitEvent) {
		e.preventDefault();
		close();
		if (onSearch()) searchHooks.submit?.();
		else open(searchField.term);
	}
	function focused() {
		if (!searchField.term.trim()) open('');
	}
	function keyed(e: KeyboardEvent) {
		if (answer && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
			e.preventDefault();
			active = step(active, count, e.key === 'ArrowDown' ? 1 : -1);
			return;
		}
		if (answer && e.key === 'Enter' && active >= 0) {
			e.preventDefault();
			const n = answer.completions.length;
			if (active < n) word(answer.completions[active]);
			else pick(suggestHrefAt(active - n));
			return;
		}
		if (e.key !== 'Escape') return;
		if (answer) {
			e.preventDefault();
			close();
			return;
		}
		if (searchField.term) {
			e.preventDefault();
			searchField.term = '';
		} else if (onSearch() && searchHooks.exit) {
			e.preventDefault();
			searchHooks.exit();
		} else el?.blur();
	}
	function suggestHrefAt(j: number) {
		return answer ? suggestHref(answer.items[j]) : '';
	}
	const here = $derived(appPath(page.url.pathname).startsWith('/search'));
	// On Search one round × does both jobs, as Escape does: it clears the words, then (empty) closes Search.
	const one = $derived(here && closable);
	function clearOrClose() {
		close();
		if (searchField.term) {
			searchField.term = '';
			el?.focus();
		} else searchHooks.exit?.();
	}
</script>

<form class="ui-search" role="search" onsubmit={submitted}>
	<span class="glow" aria-hidden="true"></span>
	<span class="ic" aria-hidden="true"><Icon name="search" size={18} /></span>
	<label class="sr-only" for={id}>Search for a title, an artist or a person</label>
	<input {id} bind:this={el} type="search" bind:value={searchField.term} oninput={typed} onfocus={focused} onclick={focused} onblur={() => { close(); searchHooks.blur?.(); }} role="combobox" aria-expanded={!!answer} aria-controls={answer ? listId : undefined} aria-autocomplete="list" aria-activedescendant={answer && active >= 0 ? `${listId}-${active}` : undefined} onkeydown={keyed} placeholder="Shows, movies and people" autocomplete="off" spellcheck="false" enterkeyhint="search" data-testid={testid} />
	{#if searchField.term && !one}
		<button type="button" class="clear" aria-label="Clear the search" onclick={() => { searchField.term = ''; close(); el?.focus(); }}><Icon name="x" size={14} /></button>
	{:else if !here}
		<kbd class="hint" aria-hidden="true">{searchKey}</kbd>
	{/if}
	{#if one}<button type="button" class="close" aria-label={searchField.term ? 'Clear the search' : 'Close Search'} title={searchField.term ? 'Clear the search' : 'Close Search'} data-testid="search-close" onclick={clearOrClose}><Icon name="x" size={16} /></button>{/if}
	{#if answer}<GlassDropdown {answer} {active} id={listId} onpick={pick} onword={word} />{/if}
</form>

<style>
	.ui-search { position: relative; display: flex; align-items: center; width: clamp(280px, 36vw, 560px); max-width: 100%; border-radius: 999px; box-shadow: 0 8px 24px rgb(0 0 0 / 0.18), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	.glow { position: absolute; inset: -6px; border-radius: 999px; box-shadow: 0 0 0 6px color-mix(in srgb, var(--accent) 16%, transparent), 0 0 28px color-mix(in srgb, var(--accent) 22%, transparent); opacity: 0; pointer-events: none; }
	.ic { position: absolute; z-index: 1; left: 15px; display: grid; color: var(--text-muted); pointer-events: none; }
	input { flex: 1; min-width: 0; height: 44px; padding: 0 64px 0 42px; border: 1px solid transparent; border-radius: 999px; background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); color: var(--text); font: var(--t-body); }
	input::placeholder { color: var(--text-muted); }
	input::-webkit-search-cancel-button { display: none; }
	input:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.hint { position: absolute; right: 14px; padding: 2px 7px; border-radius: 6px; border: 1px solid var(--line); color: var(--text-muted); font: 500 11px/1.4 var(--font-ui); pointer-events: none; }
	.close { position: absolute; z-index: 1; right: 4px; display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); cursor: pointer; }
	.close:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	form:has(.close) .clear { right: 42px; }
	form:has(.close) input { padding-right: 84px; }
	.clear { position: absolute; z-index: 1; right: 4px; display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 999px; background: none; color: var(--text-muted); cursor: pointer; }
	.clear:hover { color: var(--text); }
	.clear:focus-visible { outline: none; box-shadow: var(--focus-ring); }
</style>
