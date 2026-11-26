<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { dateTime, duration, percent } from '#lib/core/format.ts';
	import { lang, t } from '#lib/core/i18n/i18n.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { plainMessage } from '#lib/core/settings.ts';
	import Button from '#lib/ui/Button.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import StreamDetails from './StreamDetails.svelte';
	import type { ComponentProps } from 'svelte';

	type Play = {
		id: number;
		title_id?: number;
		version_id?: number;
		version?: string;
		poster?: string;
		details?: ComponentProps<typeof StreamDetails>['details'];
		title: string;
		person: string;
		device: string;
		method: 'direct_play' | 'direct_stream' | 'transcode' | 'unknown';
		state: 'playing' | 'paused' | 'buffering' | 'unknown';
		position_ms: number;
		duration_ms: number;
		watch_ms: number;
		started_at: string;
		incomplete: boolean;
	};
	type Rank = { key: string; name: string; plays: number; watch_ms: number };
	type Playback = {
		enabled: boolean;
		stale: boolean;
		last_seen_at: string | null;
		active: Play[] | null;
		history: Play[] | null;
		people: { account: string; name: string }[] | null;
		plays: number;
		watch_ms: number;
		viewers: number;
		methods: { method: Play['method']; plays: number }[] | null;
		top_titles: Rank[] | null;
		top_viewers: Rank[] | null;
	};

	let { server }: { server: number } = $props();
	const uid = $props.id();
	const pageSize = 50;
	const pollInterval = 5000;
	const endpoint = $derived(`/media-servers/${server}/playback`);
	let view = $state<Playback | null>(null);
	let error = $state('');
	let offset = $state(0);
	let fields = $state({ title: '', account: '', from: '', through: '' });
	let filters = $state({ ...fields });
	let requestId = 0;
	let clockNow = $state(0);
	let sampleReceivedAt = $state(0);
	const estimateMs = $derived(view?.last_seen_at && !view.stale && !error
		? Math.min(pollInterval * 2, Math.floor(Math.max(0, clockNow - sampleReceivedAt) / 1000) * 1000)
		: 0);

	async function refresh() {
		const id = ++requestId;
		const query = new URLSearchParams({ ...filters, offset: String(offset) });
		try {
			const next = await api.get<Playback>(`${endpoint}?${query}`);
			if (id === requestId) {
				clockNow = performance.now();
				// A refresh of the same sample does not restart its clock.
				if (next.last_seen_at !== view?.last_seen_at) sampleReceivedAt = clockNow;
				view = next;
				error = '';
			}
		} catch (err) {
			if (id === requestId) error = plainMessage(err);
		}
	}

	function filter(event: SubmitEvent) {
		event.preventDefault();
		filters = { ...fields };
		offset = 0;
		void refresh();
	}

	function setDays(days: number) {
		const now = new Date();
		fields.through = now.toISOString().slice(0, 10);
		now.setUTCDate(now.getUTCDate() - days + 1);
		fields.from = now.toISOString().slice(0, 10);
		filters = { ...fields };
		offset = 0;
		void refresh();
	}

	function filterRank(key: 'title' | 'account', value: string) {
		fields[key] = value;
		filters = { ...fields };
		offset = 0;
		void refresh();
	}

	function movePage(direction: number) {
		offset += direction * pageSize;
		void refresh();
	}

	function methodLabel(method: Play['method']): string {
		return method === 'unknown' ? t('mediafiles.copies.unknown') : t('settings.mediaServers.playback.method.' + method);
	}

	function streamSummary(play: Play): string {
		const source = play.details?.source_resolution?.toUpperCase() ?? '';
		const output = play.details?.output_height ? play.details.output_height + 'p' : '';
		const quality = [source, output].filter(Boolean).join(' → ');
		return [methodLabel(play.method), quality, play.details?.hardware ? 'HW' : ''].filter(Boolean).join(' · ');
	}

	function positionOf(play: Play): number {
		const position = play.position_ms + (play.state === 'playing' ? estimateMs : 0);
		return play.duration_ms > 0 ? Math.min(position, play.duration_ms) : position;
	}

	function timecode(ms: number): string {
		const seconds = Math.floor(ms / 1000);
		const hours = Math.floor(seconds / 3600);
		const minutes = Math.floor(seconds / 60) % 60;
		const ss = String(seconds % 60).padStart(2, '0');
		return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${ss}` : `${minutes}:${ss}`;
	}

	const number = (n: number) => n.toLocaleString(lang.value);
	const playCount = (n: number) => t(n === 1 ? 'library.stats.unit.play.one' : 'library.stats.unit.play.other', { n: number(n) });

	onMount(() => {
		void refresh();
		const poll = window.setInterval(() => {
			if (!view || view.enabled) void refresh();
		}, pollInterval);
		// A single clock for all cards. Sampling performance.now also keeps
		// the clock from lagging if the browser throttles the timer.
		const tick = window.setInterval(() => {
			if (view?.active?.some((play) => play.state === 'playing')) clockNow = performance.now();
		}, 250);
		return () => {
			window.clearInterval(poll);
			window.clearInterval(tick);
		};
	});
</script>

{#snippet playTitle(play: Play)}
	{#if play.title_id}
		<a href={to(`/library/${play.title_id}`)}>{play.title || t('mediafiles.copies.unknown')}</a>
	{:else}
		{play.title || t('mediafiles.copies.unknown')}
	{/if}
{/snippet}

{#snippet playedVersion(play: Play)}
	{#if play.title_id && play.version_id && play.version}
		<a class="version-link" href={to(`/library/${play.title_id}?version=${play.version_id}#version-${play.version_id}`)}>
			{t('requests.report.version', { name: play.version })}<Icon name="chevronRight" size={12} />
		</a>
	{/if}
{/snippet}

{#snippet ranking(title: string, rows: Rank[], kind: 'title' | 'account')}
	<section class="ranking tp-sheet">
		<h3>{title}</h3>
		<ul>
			{#each rows as row (row.key)}
				<li>
					<button type="button" onclick={() => filterRank(kind, kind === 'title' ? row.name : row.key)}>{row.name || t('mediafiles.copies.unknown')}</button>
					<span>{duration(row.watch_ms / 1000)} · {playCount(row.plays)}</span>
				</li>
			{/each}
		</ul>
	</section>
{/snippet}

<section class="playback" aria-label={t('settings.mediaServers.playback.title')}>
	<h2>{t('settings.mediaServers.playback.now')}</h2>
	{#if error}<p class="bad-text" role="alert">{error}</p>{/if}
	{#if !view}
		{#if error}<Button size="sm" type="button" onclick={refresh}>{t('ui.loadError.tryAgain')}</Button>
		{:else}<p class="muted" role="status">{t('ui.pageLoader.loading')}</p>{/if}
	{:else}
		{#if !view.enabled}
			<p class="muted">{t('settings.mediaServers.playback.monitor')} · {t('settings.mediaServers.off')}</p>
		{:else if view.stale}
			<p class="muted" role="status">{t('settings.mediaServers.playback.stale')}</p>
		{:else if !view.last_seen_at}
			<p class="muted" role="status">{t('ui.pageLoader.loading')}</p>
		{:else if !view.active?.length}
			<p class="muted">{t('settings.mediaServers.playback.idle')}</p>
		{/if}

		{#if view.active?.length && view.enabled}
			<ul class="streams" aria-label={t('settings.mediaServers.playback.now')}>
				{#each view.active as play (play.id)}
					{@const position = positionOf(play)}
					<li class="stream tp-sheet">
						{#if play.title_id}
							<a class="stream-cover" href={to(`/library/${play.title_id}`)} aria-label={`${t('downloads.act.library')} · ${play.title}`} title={t('downloads.act.library')}>
								<Poster src={play.poster} name={play.title} />
							</a>
						{:else}
							<div class="stream-cover"><Poster src={play.poster} name={play.title} /></div>
						{/if}
						<div class="stream-body">
							<div class="stream-heading">
								<span class="state">{play.state === 'paused' ? t('downloads.status.paused') : play.state === 'unknown' ? t('mediafiles.copies.unknown') : t('settings.mediaServers.playback.state.' + play.state)}</span>
							</div>
							<h3>{@render playTitle(play)}</h3>
							<p class="muted">{[play.person || t('mediafiles.copies.unknown'), play.device].filter(Boolean).join(' · ')}</p>
							{@render playedVersion(play)}
							{#if play.duration_ms > 0}
								<progress aria-label={t('downloads.col.progress')} max="100" value={percent(position / play.duration_ms)}></progress>
							{/if}
							<div class="stream-meta">
								<span>{streamSummary(play)}</span>
								<span class="muted stream-clock">
									<span data-testid="playback-position">{timecode(position)}</span>{#if play.duration_ms > 0}{` / ${timecode(play.duration_ms)}`}{/if}
								</span>
							</div>
						</div>
						{#if play.details}<div class="stream-details"><StreamDetails details={play.details} /></div>{/if}
					</li>
				{/each}
			</ul>
		{/if}

		<div class="history-title">
			<h2>{t('settings.mediaServers.playback.history')}</h2>
			<div class="pagination">
				<Button size="sm" variant="secondary" type="button" onclick={() => setDays(7)}>{t('downloads.hs.days', { n: 7 })}</Button>
				<Button size="sm" variant="secondary" type="button" onclick={() => setDays(30)}>{t('settings.appearance.last30')}</Button>
			</div>
		</div>
		<form class="filters" onsubmit={filter}>
			<Field id={uid + '-title'} label={t('mediafiles.list.title')}>
				<input id={uid + '-title'} class="input" type="search" bind:value={fields.title} maxlength="500" />
			</Field>
			<Field id={uid + '-person'} label={t('discover.people.person')}>
				<select id={uid + '-person'} class="input" bind:value={fields.account}>
					<option value="">{t('discover.hidden.everyone')}</option>
					{#each view.people ?? [] as person (person.account)}
						<option value={person.account}>{person.name || t('mediafiles.copies.unknown')}</option>
					{/each}
				</select>
			</Field>
			<Field id={uid + '-from'} label={t('downloads.hd.from') + ' (UTC)'}>
				<input id={uid + '-from'} class="input" type="date" bind:value={fields.from} max={fields.through || undefined} />
			</Field>
			<Field id={uid + '-through'} label={t('downloads.hd.to') + ' (UTC)'}>
				<input id={uid + '-through'} class="input" type="date" bind:value={fields.through} min={fields.from || undefined} />
			</Field>
			<Button size="sm" type="submit">{t('mediafiles.sortFilter.filter')}</Button>
		</form>

		<div class="metrics" data-testid="playback-metrics">
			<div class="tp-sheet metric"><Icon name="play" size={18} /><b>{playCount(view.plays)}</b></div>
			<div class="tp-sheet metric"><span>{t('settings.mediaServers.playback.watchTime')}</span><b>{duration(view.watch_ms / 1000)}</b></div>
			<div class="tp-sheet metric"><span>{t('discover.search.people')}</span><b>{number(view.viewers)}</b></div>
		</div>
		{#if view.methods?.length}
			<ul class="methods tp-sheet">
				{#each view.methods as bucket (bucket.method)}
					<li>
						<span>{methodLabel(bucket.method)}</span>
						<meter min="0" max={view.plays} value={bucket.plays} aria-label={methodLabel(bucket.method)}></meter>
						<strong>{playCount(bucket.plays)}</strong>
					</li>
				{/each}
			</ul>
		{/if}
		{#if view.top_titles?.length && view.top_viewers?.length}
			<div class="rankings">
				{@render ranking(t('library.stats.titles'), view.top_titles, 'title')}
				{@render ranking(t('discover.search.people'), view.top_viewers, 'account')}
			</div>
		{/if}
		{#if !view.history?.length}
			<p class="muted">{t('library.copies.nothingMatches')}</p>
		{:else}
			<ul class="history tp-sheet">
				{#each view.history as play (play.id)}
					<li class="history-row">
						<div>
							<strong>{@render playTitle(play)}</strong>
							{@render playedVersion(play)}
							<p class="muted">{[play.person || t('mediafiles.copies.unknown'), play.device, methodLabel(play.method)].filter(Boolean).join(' · ')}</p>
						</div>
						<div class="details">
							<time datetime={play.started_at}>{dateTime(play.started_at)}</time>
							<span>{duration(play.watch_ms / 1000)}{play.incomplete ? ` · ${t('settings.mediaServers.playback.gap')}` : ''}</span>
						</div>
						{#if play.details}<div class="row-details"><StreamDetails details={play.details} /></div>{/if}
					</li>
				{/each}
			</ul>
		{/if}
		{#if offset > 0 || offset + pageSize < view.plays}
			<div class="pagination">
				<Button size="sm" type="button" disabled={offset === 0} onclick={() => movePage(-1)}>{t('library.copies.previous')}</Button>
				<Button size="sm" type="button" disabled={offset + pageSize >= view.plays} onclick={() => movePage(1)}>{t('library.copies.next')}</Button>
			</div>
		{/if}
	{/if}
</section>

<style>
	.playback { display: grid; gap: var(--s-4); min-width: 0; }
	h2, h3 { font: var(--t-headline); }
	.streams, .history, .methods { list-style: none; padding: 0; margin: 0; }
	.streams, .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr)); gap: var(--s-3); }
	.stream, .metric, .methods, .history, .ranking { padding: var(--s-4); min-width: 0; background: var(--surface-1); border: 1px solid var(--line); border-radius: var(--r-panel); }
	.stream, .metric { display: grid; gap: var(--s-2); overflow-wrap: anywhere; }
	.stream-heading, .stream-meta, .history-row { display: flex; justify-content: space-between; gap: var(--s-3); align-items: baseline; }
	.stream-heading { align-items: center; margin-bottom: var(--s-1); }
	.stream { grid-template-columns: minmax(65px, 24%) minmax(0, 1fr); column-gap: var(--s-4); align-content: start; }
	.stream-cover { --poster-ratio: 2 / 3; }
	a.stream-cover { display: block; border-radius: 6px; }
	a.stream-cover:hover { box-shadow: 0 0 0 2px var(--accent-text); }
	.stream-body { display: grid; gap: var(--s-1); min-width: 0; min-height: 0; align-content: center; }
	.stream-details { grid-column: 1 / -1; }
	.version-link { display: inline-flex; width: fit-content; align-items: center; gap: var(--s-1); color: var(--accent-text); font: var(--t-footnote); margin-inline-start: var(--s-1); text-decoration: underline; text-underline-offset: .2em; }
	.stream-body .version-link { margin-inline-start: 0; }
	.state { color: var(--accent-text); background: var(--surface-2); border-radius: var(--r-panel); padding: var(--s-1) var(--s-2); }
	.state, .stream-meta, .metric span { font: var(--t-footnote); }
	.stream-meta { flex-wrap: wrap; }
	.stream-clock { white-space: nowrap; font-variant-numeric: tabular-nums; }
	.metric b { font: var(--t-title2); }
	.history-title { margin-top: var(--s-4); display: flex; gap: var(--s-2); align-items: center; justify-content: space-between; flex-wrap: wrap; }
	.rankings { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 290px), 1fr)); gap: var(--s-3); }
	.ranking ul { list-style: none; margin: var(--s-2) 0 0; padding: 0; }
	.ranking li { display: flex; align-items: baseline; justify-content: space-between; gap: var(--s-3); padding-block: var(--s-1); }
	.ranking li > span { color: var(--text-muted); font: var(--t-footnote); flex: 0 0 auto; }
	.ranking button { padding: var(--s-1) 0; text-align: start; text-decoration: underline; text-underline-offset: .2em; cursor: pointer; overflow-wrap: anywhere; }
	.row-details { width: 100%; }

	.methods { display: grid; gap: var(--s-2); }
	.methods li { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 2fr) auto; align-items: center; gap: var(--s-3); }
	progress, meter { width: 100%; height: 10px; accent-color: var(--accent-fill); }
	progress { margin-top: var(--s-2); }
	.filters { display: grid; grid-template-columns: minmax(0, 1fr) repeat(3, minmax(0, 140px)) auto; gap: var(--s-3); align-items: end; }
	.history-row { flex-wrap: wrap; padding-block: var(--s-3); overflow-wrap: anywhere; }
	.history-row + .history-row { border-top: 1px solid var(--line); }
	.details { text-align: end; color: var(--text-muted); font: var(--t-footnote); display: grid; flex: none; }
	.pagination { display: flex; gap: var(--s-2); }
	@media (max-width: 940px) { .filters { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@media (max-width: 540px) { .history-row { display: grid; } .details { text-align: start; display: flex; gap: var(--s-2); } }
</style>
