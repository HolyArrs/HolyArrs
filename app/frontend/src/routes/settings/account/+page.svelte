<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { Account } from '#lib/accounts/index.ts';
</script>

<svelte:head><title>{t('settings.account.title')} · HolyArrs</title></svelte:head>

<div class="account-view"><Account /></div>

<style>
	.account-view {
		width: 100%;
	}
</style>
