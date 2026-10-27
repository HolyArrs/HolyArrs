<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import type { AccountModel } from '../account.svelte.ts';

	// Where Alerts reach this person: email, Discord, new-episode notices.
	let { m, ondone }: { m: AccountModel; ondone?: () => void } = $props();
	const id = $props.id();
	async function submit(e: SubmitEvent) {
		if (await m.saveTargets(e)) ondone?.();
	}
</script>

<form class="pw" onsubmit={submit} novalidate>
	<div class="field">
		<label for="{id}-email">Email address</label>
		<input id="{id}-email" class="input" type="email" bind:value={m.targets.email} autocomplete="email" />
	</div>
	<div class="field">
		<label for="{id}-discord">Discord user id</label>
		<input id="{id}-discord" class="input" inputmode="numeric" bind:value={m.targets.discord_id} autocomplete="off" />
		<span class="hint">A long number: in Discord turn on Developer Mode, then right-click your name and Copy User ID. Discord Alerts then name you.</span>
	</div>
	<div class="field">
		<label class="box"><input type="checkbox" bind:checked={m.targets.new_episodes} data-testid="new-episodes" /> New episodes of shows I asked for</label>
		<span class="hint">One notice per show, at most once an hour ("3 new episodes of …").</span>
	</div>
	{#if m.tgError}<p class="form-error" role="alert">{m.tgError}</p>{/if}
	<Button variant="primary" type="submit" disabled={m.tgBusy} aria-busy={m.tgBusy}>Save</Button>
</form>

<style>
	.pw { width: min(100%, 420px); }
	.pw :global(button) { margin-top: var(--s-4); }
</style>
