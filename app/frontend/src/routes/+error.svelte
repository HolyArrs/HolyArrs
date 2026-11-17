<!-- Page for a missing address or a failed load: a clear sentence and a way back home. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { page } from '$app/state';
	import { to } from '#lib/core/paths.ts';
	import EmptyState from '#lib/ui/EmptyState.svelte';

	const gone = $derived(page.status === 404);
</script>

<div class="error-page" data-testid="error-page">
	<EmptyState
		icon={gone ? 'search' : 'info'}
		title={gone ? t('discover.titlePage.notFound') : t('core.api.wentWrong')}
		text={gone ? t('frame.error.pageGone') : page.error?.message}
		action={{ label: t('accounts.invite.goHome'), href: to('/') }}
	/>
</div>

<style>
	.error-page { min-height: 60vh; display: grid; place-items: center; }
</style>
