<script lang="ts">
	// One box for a pasted YouTube link: Add asks the server what it points at and hands that on for the confirm card.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { message } from '#lib/core/api.ts';
	import Button from '#lib/ui/Button.svelte';
	import { ytProbe } from './api.ts';
	import type { YtProbe } from './view.ts';

	let { onprobed, disabled = false }: { onprobed: (probe: YtProbe, url: string) => void; disabled?: boolean } = $props();
	const id = $props.id();

	let url = $state('');
	let busy = $state(false);
	let error = $state('');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		const link = url.trim();
		if (!link || busy) return;
		busy = true;
		error = '';
		try {
			onprobed(await ytProbe(link), link);
			url = '';
		} catch (err) {
			// The server's own words (yt-dlp's line about this link included), as it sends them.
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<form class="add" onsubmit={submit} data-testid="youtube-add">
	<label class="sr-only" for="{id}-url">{t('youtube.add.label')}</label>
	<input
		id="{id}-url"
		class="input"
		type="url"
		inputmode="url"
		autocomplete="off"
		spellcheck="false"
		placeholder={t('youtube.add.placeholder')}
		bind:value={url}
		aria-invalid={error ? 'true' : undefined}
		aria-describedby={error ? `${id}-err` : undefined}
		{disabled}
	/>
	<Button variant="primary" type="submit" icon="plus" {busy} disabled={disabled || busy || !url.trim()}>{t('youtube.add.button')}</Button>
</form>
{#if error}<p class="form-error" id="{id}-err" role="alert" data-testid="youtube-add-error">{error}</p>{/if}

<style>
	.add { display: flex; gap: var(--s-2); align-items: center; }
	.add .input { flex: 1; min-width: 0; }
</style>
