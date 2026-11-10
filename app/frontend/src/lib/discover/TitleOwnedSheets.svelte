<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import type { OwnedTitle } from './title-owned.svelte.ts';
	import { ReportSheet } from '#lib/requests/index.ts';
	import { ReleaseSheet, DeleteSheet, RenamePreview } from '#lib/mediafiles/index.ts';

	let { own }: { own: OwnedTitle } = $props();
</script>

{#if own.owned}
	{@const o = own.owned}
	{#if own.mayReport}<ReportSheet bind:open={own.reportOpen} title={o} />{/if}
	{#if own.manage}
		<ReleaseSheet bind:open={own.pickOpen} titleId={o.id} name={o.name} scope={own.pickScope} onchosen={() => own.loadOwned(o.id)} />
		<Sheet bind:open={own.renameOpen} title={t('discover.titleOwnedSheets.renameFiles')} description={o.name} size="md">
			{#if own.renameOpen}<RenamePreview titleId={o.id} onDone={() => { own.renameOpen = false; void own.loadOwned(o.id); }} />{/if}
		</Sheet>
		<DeleteSheet bind:open={own.deleteOpen} titleId={o.id} name={o.name} kind={o.kind === 'show' ? 'show' : 'movie'} ondone={() => { own.owned = null; own.ownedEvents = []; own.ownTab = 'overview'; }} />
	{/if}
{/if}
