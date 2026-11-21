<script lang="ts">
	// Says what a pasted link is and asks once: Download a video, Download all and follow a playlist (mirrored or not), Follow a channel with its choices.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { ApiError, message } from '#lib/core/api.ts';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { ytAddFollow, ytAddVideo } from './api.ts';
	import { backfillOf, clock, type BackfillChoice, type YtFollow, type YtProbe, type YtVideo } from './view.ts';

	let {
		probe,
		url,
		onvideo,
		onfollow,
		onshowfollow,
		oncancel
	}: {
		probe: YtProbe;
		url: string;
		onvideo: (v: YtVideo) => void;
		onfollow: (f: YtFollow) => void;
		onshowfollow: (id: number) => void;
		oncancel: () => void;
	} = $props();
	const id = $props.id();

	let take = $state<BackfillChoice>('newest20');
	let newest = $state(50);
	let keep = $state(false);
	let keepN = $state(20);
	let shorts = $state(false);
	let mirror = $state(false);
	let busy = $state(false);
	let error = $state('');
	let existing = $state<number | null>(null);

	const length = $derived(clock(probe.duration_seconds));

	async function confirm() {
		if (busy) return;
		busy = true;
		error = '';
		existing = null;
		try {
			if (probe.kind === 'video') onvideo(await ytAddVideo(url));
			else if (probe.kind === 'playlist') onfollow(await ytAddFollow({ url, mirror }));
			else
				onfollow(
					await ytAddFollow({ url, backfill: backfillOf(take, newest), keep_last: keep ? Math.max(1, Math.round(keepN || 1)) : null, include_shorts: shorts })
				);
		} catch (e) {
			// A duplicate follow answers with the one the person already has.
			if (e instanceof ApiError && e.status === 409 && typeof e.body.existing_id === 'number') existing = e.body.existing_id;
			else error = message(e);
		} finally {
			busy = false;
		}
	}
</script>

<section class="card confirm" aria-labelledby="{id}-t" data-testid="youtube-confirm">
	<div class="what">
		<h2 class="h3" id="{id}-t">{probe.title}</h2>
		<p class="muted small">
			{#if probe.kind === 'video'}
				{probe.channel_name}{#if length}<span aria-hidden="true"> · </span>{length}{/if}
			{:else if probe.kind === 'playlist'}
				{t('youtube.follows.playlist')}<span aria-hidden="true"> · </span>{probe.count > 0 ? t('youtube.confirm.videos', { n: probe.count }) : t('youtube.confirm.countUnknown')}
			{:else}
				{t('youtube.follows.channel')}<span aria-hidden="true"> · </span>{probe.count > 0 ? t('youtube.confirm.videos', { n: probe.count }) : t('youtube.confirm.countUnknown')}
			{/if}
		</p>
	</div>

	{#if probe.kind === 'channel'}
		<div class="choices">
			<SegmentedControl
				label={t('youtube.confirm.take')}
				bind:value={take}
				options={[
					{ value: 'newest20', label: t('youtube.confirm.newest20') },
					{ value: 'newest', label: t('youtube.confirm.newestSome') },
					{ value: 'all', label: t('youtube.confirm.all') }
				]}
				testid="youtube-take"
			/>
			{#if take === 'newest'}
				<label class="num">
					<span class="small">{t('youtube.confirm.howMany')}</span>
					<input class="input" type="number" min="1" step="1" bind:value={newest} data-testid="youtube-take-n" />
				</label>
			{/if}
			<label class="checkline">
				<input type="checkbox" bind:checked={keep} data-testid="youtube-keep" />
				<span>{t('youtube.confirm.keepNewest')}<br /><span class="hint">{t('youtube.confirm.keepNewestHint')}</span></span>
			</label>
			{#if keep}
				<label class="num">
					<span class="small">{t('youtube.confirm.howMany')}</span>
					<input class="input" type="number" min="1" step="1" bind:value={keepN} data-testid="youtube-keep-n" />
				</label>
			{/if}
			<label class="checkline">
				<input type="checkbox" bind:checked={shorts} data-testid="youtube-shorts" />
				<span>{t('youtube.confirm.shorts')}</span>
			</label>
		</div>
	{/if}

	{#if probe.kind === 'playlist'}
		<label class="checkline">
			<input type="checkbox" role="switch" bind:checked={mirror} data-testid="youtube-mirror" />
			<span>{t('youtube.confirm.mirror')}<br /><span class="hint">{t('youtube.confirm.mirrorHint')}</span></span>
		</label>
	{/if}

	{#if existing !== null}
		<p class="notice warn" role="status" data-testid="youtube-duplicate">
			<span class="grow">{t('youtube.confirm.duplicate')}</span>
			<Button size="sm" variant="plain" onclick={() => existing !== null && onshowfollow(existing)}>{t('youtube.confirm.showFollow')}</Button>
		</p>
	{/if}
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}

	<div class="row actions">
		<Button variant="primary" icon={probe.kind === 'video' ? 'download' : 'plus'} {busy} disabled={busy} onclick={confirm} data-testid="youtube-confirm-go">
			{probe.kind === 'video' ? t('youtube.confirm.download') : probe.kind === 'playlist' ? t('youtube.confirm.downloadAllFollow') : t('youtube.confirm.follow')}
		</Button>
		<Button variant="plain" onclick={oncancel}>{t('youtube.confirm.cancel')}</Button>
	</div>
</section>

<style>
	.confirm { display: grid; gap: var(--s-4); }
	.what h2 { margin: 0 0 var(--s-1); }
	.what p { margin: 0; }
	.choices { display: grid; gap: var(--s-2); justify-items: start; }
	.num { display: flex; align-items: center; gap: var(--s-2); }
	.num .input { width: 7em; }
	.notice { display: flex; align-items: center; gap: var(--s-3); margin: 0; }
	.actions { justify-content: flex-start; }
</style>
