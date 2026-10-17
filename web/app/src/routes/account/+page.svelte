<script lang="ts">
	import Account from '#lib/Account.svelte';
</script>

<svelte:head><title>My account · HolyArrs</title></svelte:head>

<div class="account-view"><Account /></div>

<style>
	.account-view {
		width: min(900px, 100%);
		margin: 0 auto;
	}
</style>
