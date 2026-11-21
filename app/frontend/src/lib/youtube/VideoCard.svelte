<script lang="ts">
	// One of the person's videos: thumbnail, title, channel · length · age, where it stands, a watched dot and how far in they are.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { ago } from '#lib/core/format.ts';
	import { to } from '#lib/core/paths.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import { ytSaveLink, ytThumb } from './api.ts';
	import { clock, progressOf, type YtVideo } from './view.ts';

	let { video, onretry, onremove }: { video: YtVideo; onretry: (v: YtVideo) => Promise<void>; onremove: (v: YtVideo) => void } = $props();
	const id = $props.id();

	let retrying = $state(false);
	let thumbFailed = $state(false);

	const done = $derived(video.state === 'done');
	const href = $derived(done ? to(`/youtube/${video.id}`) : undefined);
	const watched = $derived(!!video.watched_at);
	const seen = $derived(progressOf(video));
	const length = $derived(clock(video.duration_seconds));
	const age = $derived(ago(video.uploaded_at ?? video.added_at));
	const items = $derived<MenuItem[]>([
		...(done ? [{ label: t('youtube.card.save'), icon: 'download', href: ytSaveLink(video.id) } as MenuItem] : []),
		{ label: t('youtube.card.remove'), icon: 'trash', destructive: true, onselect: () => onremove(video) }
	]);

	async function retry() {
		retrying = true;
		try {
			await onretry(video);
		} finally {
			retrying = false;
		}
	}
</script>

<article class="video" aria-labelledby="{id}-t" data-testid="youtube-video" data-state={video.state}>
	<svelte:element this={href ? 'a' : 'div'} class="thumb" {href} tabindex={href ? -1 : undefined} aria-hidden="true">
		{#if done && !thumbFailed}
			<img src={ytThumb(video.id)} alt="" loading="lazy" decoding="async" onerror={() => (thumbFailed = true)} />
		{:else}
			<span class="blank"><Icon name="play" size={28} /></span>
		{/if}
		{#if length}<span class="len">{length}</span>{/if}
		{#if seen > 0 && !watched}<span class="seen" style="--seen: {seen * 100}%"></span>{/if}
	</svelte:element>
	<div class="body">
		<div class="top">
			<h3 class="title" id="{id}-t">
				{#if href}<a {href}>{video.title || video.video_id}</a>{:else}{video.title || video.video_id}{/if}
			</h3>
			<Menu label={t('youtube.card.menu')} {items} />
		</div>
		<p class="meta small muted">
			{#if watched}<span class="dot" role="img" aria-label={t('youtube.watched')} title={t('youtube.watched')}></span>{/if}
			<span class="ellipsis">{[video.channel_name, length, age].filter(Boolean).join(' · ')}</span>
		</p>
		{#if video.state === 'queued'}
			<span class="chip wait">{t('youtube.state.queued')}</span>
		{:else if video.state === 'downloading'}
			<ProgressBar size="sm" value={video.percent ?? 0} label={t('youtube.state.downloading', { percent: video.percent ?? 0 })} />
			<span class="small muted">{t('youtube.state.downloading', { percent: video.percent ?? 0 })}</span>
		{:else if video.state === 'failed'}
			<div class="failed">
				<span class="chip fail">{t('youtube.state.failed')}</span>
				<Button size="sm" icon="refresh" busy={retrying} disabled={retrying} onclick={retry} data-testid="youtube-retry">{t('youtube.retry')}</Button>
			</div>
			{#if video.error}<p class="err small">{video.error}</p>{/if}
		{/if}
	</div>
</article>

<style>
	.video { display: grid; gap: var(--s-2); align-content: start; min-width: 0; }
	.thumb {
		position: relative; display: block; aspect-ratio: 16 / 9; overflow: hidden;
		border-radius: var(--r-card); background: var(--surface-2); color: var(--text-muted);
	}
	.thumb img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.blank { position: absolute; inset: 0; display: grid; place-items: center; }
	.len {
		position: absolute; inset-inline-end: 6px; bottom: 8px; padding: 1px 5px; border-radius: 4px;
		background: rgb(0 0 0 / 75%); color: #fff; font: var(--t-footnote); font-variant-numeric: tabular-nums;
	}
	.seen { position: absolute; inset-inline: 0; bottom: 0; height: 3px; background: rgb(255 255 255 / 35%); }
	.seen::after { content: ''; position: absolute; inset-block: 0; inset-inline-start: 0; width: var(--seen); background: var(--accent); }
	.body { display: grid; gap: var(--s-1); min-width: 0; }
	.top { display: flex; align-items: flex-start; gap: var(--s-1); }
	.title {
		flex: 1; min-width: 0; margin: 0; font: var(--t-headline); font-size: 15px;
		display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
	}
	.title a { color: inherit; text-decoration: none; }
	.title a:hover { text-decoration: underline; }
	.meta { display: flex; align-items: center; gap: 6px; margin: 0; min-width: 0; }
	.dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
	.failed { display: flex; align-items: center; gap: var(--s-2); flex-wrap: wrap; }
	.err { margin: 0; color: var(--fail-text); overflow-wrap: anywhere; }
	.chip { justify-self: start; }
</style>
