<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { session } from '#lib/session.svelte.ts';
	import type { AccountModel } from '../account.svelte.ts';

	// Confirm local sign-in: the HolyArrs password (and a code when two-factor is on).
	let { m, intro = true, ondone }: { m: AccountModel; intro?: boolean; ondone?: () => void } = $props();
	const id = $props.id();
	async function submit(e: SubmitEvent) {
		if (await m.reauthenticate(e)) ondone?.();
	}
</script>

{#if intro}
	<p class="small muted lead">
		{#if session.s.localRecent}
			Your local sign-in was confirmed recently. Confirm it again here if a sensitive action says that confirmation expired.
		{:else}
			Confirm your HolyArrs password before linking a sign-in method or making sensitive account changes.
		{/if}
		This replaces the current session with a freshly confirmed one.
	</p>
{/if}
<form class="pw" onsubmit={submit} novalidate>
	<input type="text" name="username" autocomplete="username" value={session.s.name} hidden readonly />
	<div class="field">
		<label for="{id}-pw">HolyArrs password</label>
		<input id="{id}-pw" class="input" type="password" bind:value={m.reauthPassword} autocomplete="current-password" required />
	</div>
	{#if session.s.twoFactor}
		<div class="field">
			<label for="{id}-code">Two-factor code</label>
			<input id="{id}-code" class="input code" bind:value={m.reauthCode} autocomplete="one-time-code" maxlength="11" required />
		</div>
	{/if}
	{#if m.reauthError}<p class="form-error" role="alert">{m.reauthError}</p>{/if}
	<Button variant="primary" type="submit" disabled={m.reauthBusy || !m.reauthPassword || (session.s.twoFactor && !m.reauthCode.trim())} aria-busy={m.reauthBusy}>
		Confirm local sign-in
	</Button>
</form>

<style>
	.lead { margin: 0 0 var(--s-3); }
	.pw { width: min(100%, 420px); }
	.pw :global(button) { margin-top: var(--s-4); }
	.code { font-family: var(--font-mono); letter-spacing: 0.08em; }
</style>
