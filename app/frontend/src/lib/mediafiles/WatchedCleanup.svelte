<!-- Titles the chosen people finished watching, biggest first. It only suggests: each one is removed through the usual delete sheet, which the owner confirms. -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api, message } from '#lib/core/api.ts';
	import { bytes, ago } from '#lib/core/format.ts';
	import { to } from '#lib/core/paths.ts';
	import Button from '#lib/ui/Button.svelte';
	import DeleteSheet from './DeleteSheet.svelte';
	import { cleanupQuery, watchedLine, type WatchedCleanup, type WatchedTitle } from './watched-cleanup.ts';

	let list = $state<WatchedCleanup | null>(null);
	let days = $state(30);
	let picked = $state<string[]>([]);
	let error = $state('');
	let removing = $state<WatchedTitle | null>(null);
	let sheetOpen = $state(false);
	const line = $derived(watchedLine(list));

	onMount(() => void read('?days=30'));

	async function read(q: string) {
		try {
			list = await api.get<WatchedCleanup>('/library/watched-cleanup' + q);
			days = list.days;
			picked = list.people.filter((p) => p.chosen).map((p) => p.key);
			error = '';
		} catch (e) {
			error = message(e);
		}
	}

	function refresh() {
		const q = cleanupQuery(Math.max(0, Math.round(days || 0)), picked, list?.people ?? []);
		if (q) void read(q);
		else if (list) list = { ...list, found: [] };
	}

	function toggle(key: string, on: boolean) {
		picked = on ? [...picked, key] : picked.filter((k) => k !== key);
		refresh();
	}

	function removed() {
		if (list && removing) list.found = list.found.filter((f) => f.id !== removing?.id);
		removing = null;
	}
</script>

{#if list?.enabled && list.people.length}
	<section class="tp-sheet cleanup" data-testid="watched-cleanup">
		<h2 class="fh">{t('library.stats.cleanup')}</h2>
		<p class="hint">{t('library.stats.cleanupDesc')}</p>
		<label class="days">{t('library.stats.cleanupDays')} <input type="number" min="0" max="3650" inputmode="numeric" bind:value={days} onchange={refresh} data-testid="cleanup-days" /></label>
		<fieldset>
			<legend>{t('library.stats.cleanupWho')}</legend>
			{#each list.people as p (p.key)}
				<label class="who"><input type="checkbox" checked={picked.includes(p.key)} onchange={(e) => toggle(p.key, e.currentTarget.checked)} /> {p.name} <span class="muted xs">{p.server}</span></label>
			{/each}
		</fieldset>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		{#if !picked.length}<p class="hint">{t('library.stats.cleanupNobody')}</p>{:else if line}<p class="hint" aria-live="polite" data-testid="cleanup-line">{line}</p>{/if}
		{#if picked.length && list.found.length}
			<ul>
				{#each list.found as x (x.id)}
					<li data-testid="cleanup-found">
						<span class="what"><a href={to(`/library/${x.id}`)}><b>{x.name}</b></a>{#if x.year}<span class="muted"> ({x.year})</span>{/if}<br /><span class="xs muted">{t('library.stats.cleanupLast', { ago: ago(x.last_watched_at), size: bytes(x.bytes) })}</span></span>
						<Button variant="destructive" type="button" onclick={() => { removing = x; sheetOpen = true; }}>{t('library.stats.cleanupRemove')}</Button>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{/if}
{#if removing}
	<DeleteSheet bind:open={sheetOpen} titleId={removing.id} name={removing.name} kind={removing.kind} ondone={removed} />
{/if}

<style>
	.cleanup { display: grid; gap: var(--s-2); min-width: 0; margin-top: var(--s-4); }
	.days { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s-2); }
	.days input { width: 6em; }
	fieldset { border: 0; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: var(--s-1) var(--s-3); min-width: 0; }
	legend { padding: 0; margin-bottom: var(--s-1); }
	.who { display: inline-flex; align-items: center; gap: 6px; overflow-wrap: anywhere; }
	ul { margin: 0; padding: 0; list-style: none; }
	li { display: flex; align-items: center; justify-content: space-between; gap: var(--s-3); padding: 8px 0; border-top: 1px solid var(--line); min-width: 0; }
	li:first-child { border-top: 0; }
	.what { min-width: 0; overflow-wrap: anywhere; }
	li :global(button) { flex: none; }
</style>
