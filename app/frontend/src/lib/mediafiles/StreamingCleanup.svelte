<!-- Titles with files that stream on the owner's services. It only suggests: each one is removed through the usual delete sheet, which the owner confirms. -->
<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { bytes } from '#lib/core/format.ts';
	import { to } from '#lib/core/paths.ts';
	import Button from '#lib/ui/Button.svelte';
	import DeleteSheet from './DeleteSheet.svelte';
	import { cleanupLine, type CleanupCheck, type OnServicesTitle } from './streaming-cleanup.ts';

	let { ready, dirty }: { ready: boolean; dirty: boolean } = $props();
	let check = $state<CleanupCheck | null>(null);
	let error = $state('');
	let busy = $state(false);
	let removing = $state<OnServicesTitle | null>(null);
	let sheetOpen = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;
	const line = $derived(cleanupLine(check));

	onMount(() => void read());
	onDestroy(() => clearTimeout(timer));

	async function read() {
		try {
			check = await api.get<CleanupCheck>('/settings/asks/streaming-cleanup');
			error = '';
		} catch (e) {
			error = message(e);
		}
		clearTimeout(timer);
		if (check?.running) timer = setTimeout(read, 2000);
	}

	async function start() {
		busy = true;
		error = '';
		try {
			check = await api.post<CleanupCheck>('/settings/asks/streaming-cleanup', {});
			if (check.running) timer = setTimeout(read, 1000);
		} catch (e) {
			error = serverWords(message(e));
		} finally {
			busy = false;
		}
	}

	function remove(x: OnServicesTitle) {
		removing = x;
		sheetOpen = true;
	}

	function removed() {
		if (check && removing) check.found = check.found.filter((f) => f.id !== removing?.id);
		removing = null;
	}
</script>

<div class="cleanup" data-testid="streaming-cleanup">
	<div><Button variant="plain" type="button" onclick={start} disabled={!ready || dirty || busy || check?.running} aria-busy={busy || check?.running} data-testid="streaming-check">{t('settings.requests.cleanupCheck')}</Button></div>
	{#if dirty && ready}<p class="hint">{t('settings.requests.cleanupSaveFirst')}</p>{/if}
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	{#if line}<p class="hint" aria-live="polite" data-testid="streaming-check-line">{line}</p>{/if}
	{#if check?.found.length}
		<ul>
			{#each check.found as x (x.id)}
				<li data-testid="streaming-found">
					<span class="what"><a href={to(`/library/${x.id}`)}><b>{x.name}</b></a>{#if x.year}<span class="muted"> ({x.year})</span>{/if}<br /><span class="xs muted">{t('settings.requests.cleanupOn', { service: x.service, size: bytes(x.bytes) })}</span></span>
					<Button variant="destructive" type="button" onclick={() => remove(x)}>{t('settings.requests.cleanupRemove')}</Button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
{#if removing}
	<DeleteSheet bind:open={sheetOpen} titleId={removing.id} name={removing.name} kind={removing.kind} ondone={removed} />
{/if}

<style>
	.cleanup { display: grid; gap: var(--s-2); min-width: 0; }
	ul { margin: 0; padding: 0; list-style: none; }
	li { display: flex; align-items: center; justify-content: space-between; gap: var(--s-3); padding: 8px 0; border-top: 1px solid var(--line); min-width: 0; }
	li:first-child { border-top: 0; }
	.what { min-width: 0; overflow-wrap: anywhere; }
	li :global(button) { flex: none; }
</style>
