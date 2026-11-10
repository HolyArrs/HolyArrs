<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { createSuggest, live, step, suggestHref, SUGGEST_MIN, type SuggestAnswer } from './live-search.ts';
	import GlassDropdown from './GlassDropdown.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { appPath, to } from '../core/paths.ts';
	import { dlScope, registerField, rememberReturn, searchField, searchHooks } from '../core/search-field.svelte.ts';
	import { api } from '../core/api.ts';
	import { inSettings, settingsFind } from '../core/settings-find.svelte.ts';

	// The one search field in the top bar. Typing or Enter opens the Search page with the words; focusing or clicking an empty field opens it on Browse. Escape clears the words; a second Escape closes Search.
	let { searchKey, testid = 'search-box', id = 'q', closable = true }: { searchKey: string; testid?: string; id?: string; closable?: boolean } = $props();

	let el = $state<HTMLInputElement>();
	$effect(() => (el ? registerField(el) : undefined));
	const onSearch = () => appPath(page.url.pathname).startsWith('/search');
	// On Downloads the field searches that list (a chip says so); removing the chip searches all of HolyArrs again.
	const scoped = $derived(dlScope.on && !dlScope.off && appPath(page.url.pathname).startsWith('/downloads'));
	// In Settings the field searches settings instead, with results on the Settings home.
	const settings = $derived(inSettings(appPath(page.url.pathname)));
	const shown = $derived(settings ? settingsFind.term : scoped ? dlScope.term : searchField.term);
	function settingsTyped(v: string) {
		settingsFind.term = v;
		if (appPath(page.url.pathname) !== '/settings' && v.trim()) void goto(to('/settings')).then(() => el?.focus());
	}
	function settingsKey(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			if (settingsFind.term.trim() && settingsFind.first) {
				const href = settingsFind.first;
				settingsFind.term = '';
				el?.blur();
				void goto(href);
			}
		} else if (e.key === 'Escape') {
			e.preventDefault();
			if (settingsFind.term) settingsFind.term = '';
			else el?.blur();
		}
	}
	function leaveScope() {
		searchField.term = dlScope.term;
		dlScope.term = '';
		dlScope.off = true;
		el?.focus();
		if (searchField.term.trim()) typed();
	}

	function open(q: string) {
		if (onSearch()) return;
		const t = q.trim();
		rememberReturn();
		void goto(to(t ? `/search?q=${encodeURIComponent(t)}` : '/search'), { reset: false }).then(() => scrollTo(0, 0));
	}
	// The live dropdown under the field, shared by every page.
	let answer = $state<SuggestAnswer | null>(null);
	let active = $state(-1);
	// Words typed and suggestions on the way: the panel is already open with loading rows.
	let waiting = $state(false);
	const listId = $derived(`${id}-suggest`);
	const suggest = createSuggest(
		(q, signal) => api.get<SuggestAnswer>(`/search/suggest?q=${encodeURIComponent(q)}${kindParam()}`, signal),
		(a) => {
			answer = a && (a.completions.length || a.items.length) ? a : null;
			waiting = false;
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
		if (scoped) return;
		open(searchField.term);
		waiting = [...searchField.term.trim()].length >= SUGGEST_MIN && live.on !== false && !answer;
		suggest.type(searchField.term);
	}
	function submitted(e: SubmitEvent) {
		e.preventDefault();
		if (scoped) return;
		close();
		if (onSearch()) searchHooks.submit?.();
		else open(searchField.term);
	}
	function focused() {
		if (scoped) return;
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
		if (scoped) {
			e.preventDefault();
			if (dlScope.term) dlScope.term = '';
			else el?.blur();
			return;
		}
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
	// On Search one × does both jobs, as Escape does: it clears the words, then closes Search.
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
	<label class="sr-only" for={id}>{settings ? t('discover.searchField.searchSettings') : t('discover.searchField.searchForATitle')}</label>
	{#if settings}
	<input {id} bind:this={el} type="search" value={shown} oninput={(e) => settingsTyped(e.currentTarget.value)} onkeydown={settingsKey} placeholder={t('discover.searchField.searchSettings')} autocomplete="off" spellcheck="false" enterkeyhint="search" data-testid={testid} />
	{#if shown}<button type="button" class="clear" tabindex="-1" aria-label={t('discover.searchField.clearTheSearch')} onclick={() => { settingsFind.term = ''; el?.focus(); }}><Icon name="x" size={14} /></button>{:else}<kbd class="hint" aria-hidden="true">{searchKey}</kbd>{/if}
	{:else}
	<input {id} class:scoped bind:this={el} type="search" value={shown} oninput={(e) => { if (scoped) dlScope.term = e.currentTarget.value; else { searchField.term = e.currentTarget.value; typed(); } }} onfocus={focused} onclick={focused} onblur={() => { close(); searchHooks.blur?.(); }} role="combobox" aria-expanded={!!answer} aria-controls={answer ? listId : undefined} aria-autocomplete="list" aria-activedescendant={answer && active >= 0 ? `${listId}-${active}` : undefined} onkeydown={keyed} placeholder={scoped ? t('discover.searchField.searchDownloads') : t('discover.searchField.showsMoviesPeople')} autocomplete="off" spellcheck="false" enterkeyhint="search" data-testid={testid} />
	{#if scoped}<span class="scope" data-testid="dl-scope-chip">{t('discover.searchField.downloads')}<button type="button" aria-label={t('discover.searchField.searchAllOfHolyArrs')} title={t('discover.searchField.searchAllOfHolyArrs2')} onclick={leaveScope} data-testid="dl-scope-chip-x"><Icon name="x" size={11} /></button></span>{/if}
	{#if shown && !one}
		<button type="button" class="clear" aria-label={t('discover.searchField.clearTheSearch')} onclick={() => { if (scoped) dlScope.term = ''; else searchField.term = ''; close(); el?.focus(); }}><Icon name="x" size={14} /></button>
	{:else if !here}
		<kbd class="hint" aria-hidden="true">{searchKey}</kbd>
	{/if}
	{/if}
	{#if one && !settings}<button type="button" class="close" aria-label={searchField.term ? t('discover.searchField.clear') : t('discover.searchField.close')} title={searchField.term ? t('discover.searchField.clear') : t('discover.searchField.close')} data-testid="search-close" onclick={clearOrClose}><Icon name="x" size={16} /></button>{/if}
	{#if answer || waiting}<GlassDropdown {answer} {active} id={listId} onpick={pick} onword={word} />{/if}
</form>

<style>
	.ui-search { position: relative; display: flex; align-items: center; width: clamp(280px, 36vw, 560px); max-width: 100%; border-radius: 999px; box-shadow: 0 8px 24px rgb(0 0 0 / 0.18), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	.glow { position: absolute; inset: -6px; border-radius: 999px; box-shadow: 0 0 0 6px color-mix(in srgb, var(--accent) 16%, transparent), 0 0 28px color-mix(in srgb, var(--accent) 22%, transparent); opacity: 0; pointer-events: none; }
	.ic { position: absolute; z-index: 1; left: 15px; display: grid; color: var(--text-muted); pointer-events: none; }
	input { flex: 1; min-width: 0; height: 44px; padding: 0 64px 0 42px; border: 1px solid transparent; border-radius: 999px; background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); color: var(--text); font: var(--t-body); }
	.scope { position: absolute; z-index: 1; left: 40px; display: inline-flex; align-items: center; gap: 2px; height: 28px; padding: 0 3px 0 12px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text); font: var(--t-subhead); font-size: 13px; transition: background var(--dur-fast) var(--ease-out); }
	.scope:hover { background: var(--surface-3); }
	.scope button { display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 50%; background: none; color: inherit; opacity: 0.75; cursor: pointer; }
	.scope button:hover { opacity: 1; background: rgb(127 127 127 / 18%); }
	.scope button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	input.scoped { padding-left: 164px; }
	input { text-overflow: ellipsis; }
	input::placeholder { color: var(--text-muted); text-overflow: ellipsis; }
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
