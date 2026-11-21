<script lang="ts">
	// Said when the server lacks yt-dlp or ffmpeg: nothing can be looked up or downloaded until they are there.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';

	let { missing }: { missing: string[] } = $props();
	const docs = 'https://docs.holyarrs.com/youtube/';
</script>

<div class="notice warn" role="status" data-testid="youtube-needs-programs">
	<Icon name="alert" size={18} />
	<div>
		<p class="lead">{t('youtube.needs.title')}</p>
		<p class="small">{t('youtube.needs.missing', { names: missing.join(', ') })}</p>
		<p class="small"><a href={docs} target="_blank" rel="noopener noreferrer">{t('youtube.needs.docs')}</a></p>
	</div>
</div>

<style>
	p { margin: 0; }
	.lead { font-weight: 600; }
</style>
