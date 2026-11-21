<script lang="ts">
	// A small strip with the video downloading now and how many wait behind it; gone when nothing is moving.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import type { YtVideo } from './view.ts';

	let { videos }: { videos: YtVideo[] } = $props();
	const id = $props.id();

	const now = $derived(videos.find((v) => v.state === 'downloading'));
	const waiting = $derived(videos.filter((v) => v.state === 'queued').length);
</script>

{#if now || waiting > 0}
	<section class="strip" aria-labelledby="{id}-t" data-testid="youtube-downloading">
		<h2 class="small" id="{id}-t">{t('youtube.strip.title')}</h2>
		{#if now}
			<div class="now">
				<span class="ellipsis small">{now.title || now.video_id}</span>
				<ProgressBar size="sm" value={now.percent ?? 0} showValue label={t('youtube.state.downloading', { percent: now.percent ?? 0 })} />
			</div>
		{/if}
		{#if waiting > 0}<span class="small muted">{t('youtube.strip.waiting', { n: waiting })}</span>{/if}
	</section>
{/if}

<style>
	.strip {
		display: flex; align-items: center; gap: var(--s-3); flex-wrap: wrap;
		padding: var(--s-2) var(--s-3); border: 1px solid var(--line); border-radius: var(--r-card); background: var(--surface-1);
	}
	.strip h2 { margin: 0; font-weight: 600; }
	.now { flex: 1; min-width: 12em; display: grid; gap: 2px; }
</style>
