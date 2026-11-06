<script lang="ts">
	import { api, message } from './api.ts';
	import { editionDecisionBody } from './copy-controls.ts';
	import { toasts } from './toasts.svelte.ts';
	import Button from './ui/Button.svelte';
	import Icon from './Icon.svelte';
	import { plainReason } from './plain.ts';

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
			toasts.push('Edition saved', 'ok', { body: 'Trying the import again' });
			await onchange();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
</script>

<form class="edition-decision" onsubmit={submit} data-testid="edition-decision">
	<div class="decision-title"><span class="ic" aria-hidden="true"><Icon name="film" size={16} /></span><p class="t">Check this movie's edition</p></div>
	<p class="note">Enter the edition shown by the download, such as Theatrical or Director cut. HolyArrs repeats every protection, audio and placement check before importing.</p>
	<div class="field">
		<label for="download-edition-{downloadId}">Edition</label>
		<input id="download-edition-{downloadId}" class="input" bind:value={edition} maxlength="80" required />
	</div>
	<label class="checkline small"><input type="checkbox" bind:checked={checked} /> I checked this download and confirm the edition above</label>
	{#if error}<p class="form-error" role="alert">{plainReason(error).text}</p>{/if}
	<div><Button variant="primary" type="submit" disabled={!edition.trim() || !checked} {busy}>Save edition and try import again</Button></div>
</form>

<style>
	.edition-decision {
		display: grid;
		gap: 8px;
		margin-top: 12px;
		padding: 16px;
		border: 0;
		border-radius: var(--r-card);
		background: var(--surface-2);
	}
	.decision-title .t { margin: 0; font: var(--t-headline); }
	.note { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.form-error { color: var(--fail); font: var(--t-callout); }
	.edition-decision p { margin: 0; }
	.decision-title {
		display: flex;
		align-items: center;
		gap: var(--s-2);
	}
	.decision-title > .ic {
		display: grid;
		place-items: center;
		width: 28px;
		height: 28px;
		border-radius: 50%;
		background: var(--surface-1);
		color: var(--text-muted);
	}
	@media (max-width: 600px) {
		.edition-decision :global(.ui-btn) {
			width: 100%;
		}
	}

</style>
