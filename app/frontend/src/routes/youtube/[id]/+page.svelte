<script lang="ts">
	// One of the person's YouTube videos: plays it from where they stopped and saves it to the device.
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { message } from '#lib/core/api.ts';
	import { date } from '#lib/core/format.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import { VideoPlayer, applyLive, makeSaver, resumeAt, ytFile, ytRetry, ytSaveLink, ytSavePosition, ytThumb, ytVideo, type YtLive, type YtVideo } from '#lib/youtube/index.ts';

	// The layout sends anyone else away; nothing of the page draws for them meanwhile.
	const mayUse = $derived(session.can('youtube') && !session.kid && !session.held('youtube'));
	const id = $derived(Number(page.params.id));

	let video = $state<YtVideo | null>(null);
	let start = $state(0);
	let loaded = $state(false);
	let error = $state('');
	let retrying = $state(false);

	async function load(which: number) {
		try {
			const v = await ytVideo(which);
			if (which !== id) return;
			video = v;
			start = resumeAt(v);
			if (saverOf !== v.id) {
				saver = saverFor(v.id);
				saverOf = v.id;
			}
			error = '';
		} catch (e) {
			if (which === id) error = message(e);
		} finally {
			if (which === id) loaded = true;
		}
	}

	// Saves go out with keepalive, so the last one still lands when the person leaves the page.
	function saverFor(which: number) {
		return makeSaver((seconds, duration) => {
			ytSavePosition(which, seconds, duration).then(
				(r) => {
					if (video?.id !== which) return;
					video.position_seconds = seconds;
					if (r?.watched && !video.watched_at) video.watched_at = new Date().toISOString();
				},
				() => {
					// The next save carries a newer position.
				}
			);
		});
	}
	// Belongs to the video on screen, not the address: moving to another video unmounts the old player, whose last save must still go to the old video.
	let saver: ReturnType<typeof makeSaver> | null = null;
	let saverOf = 0;

	function position(seconds: number, duration: number, final: boolean) {
		if (final) saver?.leave(seconds, duration);
		else saver?.tick(seconds, duration);
	}

	$effect(() => {
		if (!mayUse || !Number.isInteger(id) || id <= 0) return;
		const which = id;
		untrack(() => {
			video = null;
			loaded = false;
			void load(which);
		});
		const offs = [
			live.on('youtube', (data) => {
				const ev = data as YtLive;
				if (ev?.id !== which || !video) return;
				if (ev.state === 'done') void load(which);
				else video = applyLive([video], ev)[0];
			}),
			live.on('resync', () => void load(which))
		];
		return () => offs.forEach((off) => off());
	});

	async function retry() {
		if (!video) return;
		retrying = true;
		try {
			video = await ytRetry(video.id);
		} catch (e) {
			toasts.push(t('youtube.retryFailed'), 'fail', { body: message(e) });
		} finally {
			retrying = false;
		}
	}

	const uploaded = $derived(video?.uploaded_at ? date(video.uploaded_at) : '');
</script>

<svelte:head><title>{video?.title || t('youtube.title')} · HolyArrs</title></svelte:head>

{#if mayUse}
	<div class="page">
		<a class="back small" href={to('/youtube')}><Icon name="chevronLeft" size={16} /><span>{t('youtube.video.back')}</span></a>

		<PageLoader loading={!loaded}>
			{#if !video}
				<LoadError hasData={false} message={error || t('youtube.video.gone')} onretry={() => void load(id)} />
			{:else}
				{#if video.state === 'done'}
					{#key video.id}
						<VideoPlayer src={ytFile(video.id)} poster={ytThumb(video.id)} startAt={start} label={video.title} onposition={position} />
					{/key}
				{:else}
					<div class="wait" role="status" data-testid="youtube-not-ready" data-state={video.state}>
						{#if video.state === 'queued'}
							<p class="lead">{t('youtube.state.queued')}</p>
							<p class="small muted">{t('youtube.video.notReady')}</p>
						{:else if video.state === 'downloading'}
							<p class="lead">{t('youtube.state.downloading', { percent: Math.round(video.percent ?? 0) })}</p>
							<ProgressBar value={video.percent ?? 0} label={t('youtube.strip.title')} />
							<p class="small muted">{t('youtube.video.notReady')}</p>
						{:else if video.state === 'failed'}
							<p class="lead">{t('youtube.state.failed')}</p>
							{#if video.error}<p class="small muted">{video.error}</p>{/if}
							<div><Button size="sm" busy={retrying} onclick={retry}>{t('youtube.retry')}</Button></div>
						{:else}
							<p class="lead">{t('youtube.video.gone')}</p>
						{/if}
					</div>
				{/if}

				<div class="about">
					<div class="titles">
						<h1 class="title">{video.title || video.video_id}</h1>
						<p class="small muted">
							{#if video.watched_at}<span class="dot" role="img" aria-label={t('youtube.watched')} title={t('youtube.watched')}></span>{/if}
							<span>{[video.channel_name, uploaded && t('youtube.video.uploaded', { date: uploaded })].filter(Boolean).join(' · ')}</span>
						</p>
					</div>
					{#if video.state === 'done'}
						<Button icon="download" href={ytSaveLink(video.id)} rel="external" data-testid="youtube-save">{t('youtube.card.save')}</Button>
					{/if}
				</div>
			{/if}
		</PageLoader>
	</div>
{/if}

<style>
	.page { display: grid; gap: var(--s-4); max-width: 1200px; }
	.back { justify-self: start; display: inline-flex; align-items: center; gap: 4px; color: var(--text-muted); text-decoration: none; }
	.back:hover { color: var(--text); }
	.wait { display: grid; gap: var(--s-2); align-content: center; justify-items: start; aspect-ratio: 16 / 9; max-height: 60vh; padding: var(--s-5); border-radius: var(--r-card); background: var(--surface-1); }
	.wait p { margin: 0; }
	.wait .lead { font-weight: 600; }
	.about { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: var(--s-3); }
	.titles { min-width: 0; display: grid; gap: var(--s-1); }
	.title { margin: 0; font: var(--t-title2); letter-spacing: var(--t-title2-track); text-wrap: balance; overflow-wrap: anywhere; }
	.titles p { margin: 0; display: flex; align-items: center; gap: var(--s-2); }
	.dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
</style>
