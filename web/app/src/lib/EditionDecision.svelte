<script lang="ts">
	import { api, message } from './api.ts';
	import { editionDecisionBody } from './copy-controls.ts';
	import { toasts } from './toasts.svelte.ts';

	let { downloadId, onchange = () => {} }: { downloadId: number; onchange?: () => void | Promise<void> } = $props();
	let edition = $state('');
	let checked = $state(false);
	let busy = $state(false);
	let error = $state('');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			await api.put(`/downloads/${downloadId}/edition`, editionDecisionBody(edition));
			toasts.push('The checked edition is saved. HolyArrs is trying the import again.', 'ok');
			await onchange();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
</script>

<form class="edition-decision" onsubmit={submit} data-testid="edition-decision">
	<p class="small"><b>Check this movie's edition</b></p>
	<p class="xs muted">Enter the edition shown by the download, such as Theatrical or Director cut. HolyArrs repeats every protection, audio and placement check before importing.</p>
	<div class="field">
		<label for="download-edition-{downloadId}">Edition</label>
		<input id="download-edition-{downloadId}" class="input" bind:value={edition} maxlength="80" required />
	</div>
	<label class="checkline small"><input type="checkbox" bind:checked={checked} /> I checked this download and confirm the edition above</label>
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	<button class="btn btn-red" type="submit" disabled={busy || !edition.trim() || !checked} aria-busy={busy}>Save edition and try import again</button>
</form>

<style>
	.edition-decision {
		display: grid;
		gap: var(--s-2);
		margin-top: var(--s-3);
		padding-top: var(--s-3);
		border-top: 1px solid var(--line);
	}
	.edition-decision p { margin: 0; }
</style>
