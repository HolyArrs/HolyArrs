<script lang="ts">
	import { ReleaseList } from '#lib/mediafiles/index.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
</script>

<svelte:head><title>{t('library.held.title')} · HolyArrs</title></svelte:head>

<ReleaseList list="held" />
