<!-- Folder picker for Import files by hand: walks only the import places on the server. -->
<script lang="ts">
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import { api, message } from '#lib/core/api.ts';
	import { appLocale, t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { FolderList } from './import-files.ts';

	let { open = $bindable(false), start = '', onpick }: { open?: boolean; start?: string; onpick: (path: string) => void } = $props();

	let list = $state<FolderList | null>(null);
	let loading = $state(false);
	let error = $state('');

	async function go(path: string) {
		loading = true;
		error = '';
		try {
			list = await api.post<FolderList>('/manual-import/folders', { path });
		} catch (err) {
			// A typed path outside the places opens the list of places instead.
			if (path && !list) return go('');
			error = message(err);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (open) {
			list = null;
			void go(start.trim());
		}
	});

	function use() {
		if (!list?.path) return;
		onpick(list.path);
		open = false;
	}
</script>

<Sheet bind:open title={t('library.byHand.pickFolder')} size="md">
	<div class="fp" aria-busy={loading}>
		<div class="here">
			{#if list?.path}
				<Button variant="plain" size="sm" icon="chevronUp" onclick={() => go(list?.parent ?? '')} disabled={loading}>{t('library.byHand.up')}</Button>
				<span class="mono small path" dir="ltr">{list.path}</span>
			{:else}
				<span class="small muted">{t('library.byHand.places')}</span>
			{/if}
		</div>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		{#if list}
			{#if list.folders.length}
				<ul class="folders">
					{#each list.folders as f (f.path)}
						<li>
							<button type="button" class="folder" onclick={() => go(f.path)} disabled={loading}>
								<Icon name="folder" size={18} />
								<span class="name" dir="ltr">{f.name}</span>
							</button>
						</li>
					{/each}
				</ul>
				{#if list.more}<p class="xs muted">{t('library.byHand.moreFolders', { n: list.folders.length.toLocaleString(appLocale()) })}</p>{/if}
			{:else if list.path}
				<EmptyState compact icon="folder" title={t('library.byHand.noFolders')} />
			{/if}
		{/if}
	</div>
	{#snippet footer()}
		<span class="xs muted count">{list?.path ? t(list.files === 1 ? 'library.byHand.filesHereOne' : 'library.byHand.filesHereMany', { n: (list?.files ?? 0).toLocaleString(appLocale()) }) : ''}</span>
		<Button variant="primary" onclick={use} disabled={!list?.path || loading}>{t('library.byHand.useFolder')}</Button>
	{/snippet}
</Sheet>

<style>
	.fp { display: flex; flex-direction: column; gap: var(--s-2); min-width: 0; }
	.here { display: flex; align-items: center; gap: var(--s-2); min-height: 44px; min-width: 0; }
	.path { overflow-wrap: anywhere; min-width: 0; }
	.folders { list-style: none; margin: 0; padding: 0; border: 1px solid var(--line); border-radius: var(--r-btn); overflow: hidden; }
	.folders li + li { border-top: 1px solid var(--line); }
	.folder { display: flex; align-items: center; gap: var(--s-2); width: 100%; min-height: 44px; padding: var(--s-2) var(--s-3); border: 0; background: none; color: var(--text); font: var(--t-callout); text-align: start; cursor: pointer; }
	.folder:hover:not(:disabled) { background: var(--surface-2); }
	.folder:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
	.name { overflow-wrap: anywhere; min-width: 0; }
	.count { flex: 1 1 auto; min-width: 0; }
</style>
