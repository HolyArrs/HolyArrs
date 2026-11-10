<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// The Turned down list lives in the Library; old links land there.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';

	onMount(() => {
		void goto(to('/library/turned-down'), { replaceState: true });
	});
</script>

<svelte:head><title>{t('downloads.page.turnedDownTitle')}</title></svelte:head>
