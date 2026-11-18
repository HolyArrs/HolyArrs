<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';

	// An owner with no password is asked to make one (the boxes), or, when the check needs a fresh sign-in, only to sign in again.
	let { flow, submit, oneCol = false }: { flow: any; submit: string; oneCol?: boolean } = $props();
</script>

{#if flow.pwStale}
	<p class="form-error" role="alert">{t('settings.move.signInAgainNote')}</p>
	<div><Button variant="primary" onclick={flow.signInAgain}>{t('settings.move.signInAgain')}</Button></div>
{:else}
	<form class="setpw" class:one-col={oneCol} onsubmit={flow.setFirstPw}>
		<input class="input" type="password" bind:value={flow.pwA} placeholder={t('settings.move.passwordLabel')} aria-label={t('settings.move.passwordLabel')} autocomplete="new-password" />
		<input class="input" type="password" bind:value={flow.pwB} placeholder={t('settings.move.confirmPasswordLabel')} aria-label={t('settings.move.confirmPasswordLabel')} autocomplete="new-password" />
		{#if flow.pwErr}<p class="form-error" role="alert">{flow.pwErr}</p>{/if}
		<Button variant="primary" type="submit" busy={flow.pwBusy} disabled={flow.pwBusy}>{submit}</Button>
	</form>
{/if}

<style>
	.setpw { display: grid; grid-template-columns: 1fr 1fr auto; gap: 8px; }
	.setpw.one-col { grid-template-columns: 1fr; }
	@media (max-width: 600px) { .setpw { grid-template-columns: 1fr; } }
</style>
