<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	// Plex sends its sign-in window here when the person is done. Close it; a
	// phone tab or a browser that refuses gets a short note instead.
	let note = $state(false);
	onMount(() => {
		window.close();
		const id = setTimeout(() => (note = true), 300);
		return () => clearTimeout(id);
	});
</script>

<svelte:head><title>{t('accounts.plexDone.title')}</title></svelte:head>

<main class="done" data-testid="plex-done">
	{#if note}<p>{t('accounts.plexDone.closeTab')}</p>{/if}
</main>

<style>
	.done {
		min-height: 100dvh;
		display: grid;
		place-items: center;
		padding: 16px;
		text-align: center;
		background: var(--bg, #0b0b0f);
		color: var(--text, #f2f2f5);
	}
	p { margin: 0; font-size: 1rem; }
</style>
