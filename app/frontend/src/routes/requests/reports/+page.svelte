<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { ReportsList } from '#lib/requests/index.ts';

	// Reports: the owner and admins see and answer every report; everyone else sees the ones they made. The list carries its own toolbar (name, tabs, the way back to Requests), like Requests does.
</script>

<svelte:head><title>{t('requests.reports.pageTitle')}</title></svelte:head>

<ReportsList />
