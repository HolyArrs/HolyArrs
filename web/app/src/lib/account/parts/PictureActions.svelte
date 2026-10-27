<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { session } from '#lib/session.svelte.ts';
	import type { AccountModel } from '../account.svelte.ts';

	// Save / Cancel after a pick, otherwise Change (or Upload) and Remove.
	let { m, size = 'md', ondone }: { m: AccountModel; size?: 'sm' | 'md'; ondone?: () => void } = $props();
	async function save() {
		await m.saveAvatar();
		if (!m.picked) ondone?.();
	}
	async function remove() {
		await m.removeAvatar();
		ondone?.();
	}
</script>

<div class="acts">
	{#if m.picked}
		<Button {size} variant="primary" type="button" onclick={save} busy={m.avatarBusy}>Save picture</Button>
		<Button {size} variant="plain" type="button" onclick={m.clearPick} disabled={m.avatarBusy}>Cancel</Button>
	{:else}
		<Button {size} variant="secondary" type="button" icon="upload" onclick={m.choosePicture} disabled={m.avatarBusy}
			>{session.s.avatar ? 'Change picture' : 'Upload picture'}</Button>
		{#if session.s.avatar}<Button {size} variant="plain" type="button" onclick={remove} disabled={m.avatarBusy}>Remove</Button>{/if}
	{/if}
</div>

<style>
	.acts { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
</style>
