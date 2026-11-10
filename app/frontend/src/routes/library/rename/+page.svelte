<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { RenamePreview, LibraryToolPage } from '#lib/mediafiles/index.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let changeCount = $state(0);
</script>

<svelte:head><title>{t('library.rename.title')} · HolyArrs</title></svelte:head>

<LibraryToolPage title={t('library.rename.title')} count={changeCount ? t('mediafiles.unit.file.' + (changeCount === 1 ? 'one' : 'other'), { n: changeCount }) : ''}>
	{#snippet bar()}
		{#if session.can('settings')}
			<span class="tp-gap"></span>
			<span class="tp-cap"><Button href={to('/settings/naming')}>{t('library.rename.naming')}</Button></span>
		{/if}
	{/snippet}

	<RenamePreview page bind:changeCount />
</LibraryToolPage>
