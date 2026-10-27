<script lang="ts">
	import { onMount } from 'svelte';
	import { AccountModel } from './account.svelte.ts';
	import AppleId from './AppleId.svelte';
	import NetflixAccount from './NetflixAccount.svelte';
	import ProfileTabs from './ProfileTabs.svelte';
	import SecurityFirst from './SecurityFirst.svelte';
	import SideTabs from './SideTabs.svelte';

	// The five My account layouts share one model, so switching keeps what was typed.
	let { view }: { view: number } = $props();
	const m = new AccountModel();
	onMount(() => m.start());
	$effect(() => {
		if (view === 3 && m.asks === null) void m.loadAsks();
	});
</script>

<!-- One picture chooser for every layout; each layout's Change picture opens it. -->
<input
	class="sr-only"
	type="file"
	accept="image/jpeg,image/png,image/webp"
	bind:this={m.avatarInput}
	onchange={m.pickAvatar}
	disabled={m.avatarBusy}
	aria-label="Choose a profile picture"
	tabindex="-1"
/>

<div class="variant" data-view={view}>
	{#if view === 1}
		<AppleId {m} />
	{:else if view === 2}
		<NetflixAccount {m} />
	{:else if view === 3}
		<ProfileTabs {m} />
	{:else if view === 4}
		<SecurityFirst {m} />
	{:else}
		<SideTabs {m} />
	{/if}
</div>

<style>
	.variant { min-width: 0; }
</style>
