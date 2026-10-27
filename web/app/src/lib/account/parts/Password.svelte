<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { session } from '#lib/session.svelte.ts';
	import type { AccountModel } from '../account.svelte.ts';

	// Change the HolyArrs password (needs a recent local sign-in).
	let {
		m,
		ondone,
		confirmHint = 'Confirm your local sign-in first.'
	}: { m: AccountModel; ondone?: () => void; confirmHint?: string } = $props();
	const id = $props.id();
	async function submit(e: SubmitEvent) {
		if (await m.changePassword(e)) ondone?.();
	}
</script>

<form class="pw" onsubmit={submit} novalidate>
	<input type="text" name="username" autocomplete="username" value={session.s.name} hidden readonly />
	<div class="field">
		<label for="{id}-cur">Current password</label>
		<input id="{id}-cur" class="input" type="password" bind:value={m.current} autocomplete="current-password" />
		{#if !session.s.localRecent}<span class="hint">{confirmHint}</span>{/if}
	</div>
	<div class="field">
		<label for="{id}-new">New password</label>
		<input id="{id}-new" class="input" type="password" bind:value={m.next} autocomplete="new-password" />
		<span class="hint">At least {m.minLength} characters.</span>
	</div>
	<div class="field">
		<label for="{id}-again">New password again</label>
		<input id="{id}-again" class="input" type="password" bind:value={m.again} autocomplete="new-password" />
	</div>
	{#if m.pwError}<p class="form-error" role="alert">{m.pwError}</p>{/if}
	<Button variant="primary" type="submit" disabled={m.pwBusy || !m.next || !session.s.localRecent} aria-busy={m.pwBusy}>Change password</Button>
</form>
{#if session.s.role === 'owner'}
	<p class="small muted owner">
		Locked out? Run <code>holyarrs reset-owner</code> on the machine that runs HolyArrs. It sets a new name and password, turns off two-factor sign-in, and
		signs out every device and API key.
	</p>
{/if}

<style>
	.pw { width: min(100%, 420px); }
	.pw :global(button) { margin-top: var(--s-4); }
	.owner { margin-top: var(--s-4); }
	code { font-family: var(--font-mono); color: var(--text); }
</style>
