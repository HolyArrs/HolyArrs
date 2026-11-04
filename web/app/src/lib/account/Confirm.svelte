<script lang="ts">
	import type { Snippet } from 'svelte';
	import Button from '#lib/ui/Button.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { needsConfirm, type Sensitive } from './sections.ts';

	// Shows the sensitive control once the local sign-in is fresh; otherwise
	// asks for the HolyArrs password (and a code) right here, inline, and then
	// shows it. Nothing waits in a permanent card.
	let { action, onCancel, children }: { action: Sensitive; onCancel?: () => void; children: Snippet } = $props();
	const id = $props.id();
	let typed = $state('');
	let code = $state('');
	let error = $state('');
	let busy = $state(false);
	const need = $derived(needsConfirm(action, { localRecent: session.s.localRecent, via: session.s.via, hasPassword: session.s.hasPassword, mustTwoFactor: session.s.mustTwoFactor }));

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			await session.reauth(typed, code.trim() || undefined);
			toasts.push('Sign-in confirmed.', 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			typed = '';
			code = '';
			busy = false;
		}
	}
</script>

{#if need}
	<form class="confirm" onsubmit={submit} novalidate data-testid="confirm-sign-in">
		<p class="hint">Confirm it's you first.</p>
		<input type="text" name="username" autocomplete="username" value={session.s.name} hidden readonly />
		<div class="field">
			<label for="{id}-pw">HolyArrs password</label>
			<input id="{id}-pw" class="input" type="password" bind:value={typed} autocomplete="current-password" required />
		</div>
		{#if session.s.twoFactor}
			<div class="field">
				<label for="{id}-code">Two-factor code</label>
				<input id="{id}-code" class="input mono" bind:value={code} autocomplete="one-time-code" maxlength="11" required />
			</div>
		{/if}
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<div class="row acts">
			<Button size="sm" variant="primary" type="submit" disabled={busy || !typed || (session.s.twoFactor && !code.trim())} busy={busy}>Confirm</Button>
			{#if onCancel}<Button size="sm" variant="plain" type="button" onclick={onCancel}>Cancel</Button>{/if}
		</div>
	</form>
{:else}
	{@render children()}
{/if}

<style>
	.confirm { display: flex; flex-direction: column; gap: 10px; width: min(100%, 420px); }
	.acts { gap: 8px; }
</style>
