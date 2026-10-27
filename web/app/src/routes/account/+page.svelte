<script lang="ts">
	import Account from '#lib/Account.svelte';
	import VariantBar from '#lib/ui/VariantBar.svelte';
	import AccountVariants from '#lib/account/AccountVariants.svelte';

	// ?view=1…5 shows one of the layouts Luis is comparing; no parameter keeps today's page.
	let view = $state(0);
	const VIEWS = [
		{ value: 0, label: 'Now' },
		{ value: 1, label: '1 Apple ID' },
		{ value: 2, label: '2 Netflix' },
		{ value: 3, label: '3 Profile' },
		{ value: 4, label: '4 Security' },
		{ value: 5, label: '5 Tabs' }
	];
</script>

<svelte:head><title>My account · HolyArrs</title></svelte:head>

{#if view === 0}
	<div class="account-view"><Account /></div>
{:else}
	<AccountVariants {view} />
{/if}
<VariantBar options={VIEWS} bind:value={view} storageKey="holyarrs.account.view" label="Account layouts" />

<style>
	.account-view {
		width: min(900px, 100%);
		margin: 0 auto;
	}
</style>
