<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Needs a decision now lives on the Downloads rows (Problems > Needs a decision; a row's sheet holds the choices). Old links land there.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';

	onMount(() => {
		void goto(to('/downloads?show=problems.decide'), { replaceState: true });
	});
</script>

<svelte:head><title>{t('downloads.page.title')}</title></svelte:head>
