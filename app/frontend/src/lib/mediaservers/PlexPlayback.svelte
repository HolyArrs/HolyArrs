<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { dateTime, duration, percent } from '#lib/core/format.ts';
	import { lang, t } from '#lib/core/i18n/i18n.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { plainMessage } from '#lib/core/settings.ts';
	import Button from '#lib/ui/Button.svelte';
	import Field from '#lib/ui/Field.svelte';
	import { ask } from '#lib/ui/ask.svelte.ts';

	type Play = {
		id: number;
		title_id?: number;
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
	type Playback = {
		enabled: boolean;
		stale: boolean;
		last_seen_at: string | null;
		active: Play[] | null;
		history: Play[] | null;
		people: { account: string; name: string }[] | null;
		plays: number;
		watch_ms: number;
	};

	let { server }: { server: number } = $props();
	const uid = $props.id();
	const pageSize = 50;
	const endpoint = $derived(`/media-servers/${server}/playback`);
	let view = $state<Playback | null>(null);
	let error = $state('');
	let clearing = $state(false);
	let offset = $state(0);
	let fields = $state({ title: '', account: '', from: '', through: '' });
	let filters = $state({ ...fields });
	let requestId = 0;

	async function refresh() {
		const id = ++requestId;
		const query = new URLSearchParams({ ...filters, offset: String(offset) });
		try {
			const next = await api.get<Playback>(`${endpoint}?${query}`);
			if (id === requestId) {
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

	function movePage(direction: number) {
		offset += direction * pageSize;
		void refresh();
	}

	async function clear() {
		const confirmed = await ask(t('settings.mediaServers.playback.clearConfirm'), {
			yes: t('settings.mediaServers.playback.clear'), danger: true
		});
		if (!confirmed) return;
		clearing = true;
		try {
			await api.del(endpoint);
			offset = 0;
			await refresh();
		} catch (err) {
			error = plainMessage(err);
		} finally {
			clearing = false;
		}
	}

	onMount(() => {
		void refresh();
		const timer = window.setInterval(() => {
			if (view?.enabled && !clearing) void refresh();
		}, 5000);
		return () => window.clearInterval(timer);
	});
</script>

{#snippet playTitle(play: Play)}
	{#if play.title_id}
		<a href={to(`/library/${play.title_id}`)}>{play.title || t('mediafiles.copies.unknown')}</a>
	{:else}
		{play.title || t('mediafiles.copies.unknown')}
	{/if}
{/snippet}

<section class="playback" aria-label={t('settings.mediaServers.playback.title')}>
	<h3>{t('settings.mediaServers.playback.now')}</h3>
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
					<li class="stream">
						<div class="stream-heading">
							<strong>{@render playTitle(play)}</strong>
							<span class="state">{play.state === 'paused' ? t('downloads.status.paused') : play.state === 'unknown' ? t('mediafiles.copies.unknown') : t('settings.mediaServers.playback.state.' + play.state)}</span>
						</div>
						<p class="muted">{[play.person || t('mediafiles.copies.unknown'), play.device, play.method === 'unknown' ? t('mediafiles.copies.unknown') : t('settings.mediaServers.playback.method.' + play.method)].filter(Boolean).join(' · ')}</p>
						{#if play.duration_ms > 0}
							<progress aria-label={t('downloads.col.progress')} max="100" value={percent(play.position_ms / play.duration_ms)}></progress>
							<p class="muted">{duration(play.position_ms / 1000)} / {duration(play.duration_ms / 1000)}</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}

		<div class="history-heading">
			<h3>{t('settings.mediaServers.playback.history')}</h3>
			<Button size="sm" type="button" variant="destructive" busy={clearing} onclick={clear}>{t('settings.mediaServers.playback.clear')}</Button>
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

		<p class="muted">{t(view.plays === 1 ? 'library.stats.unit.play.one' : 'library.stats.unit.play.other', { n: view.plays.toLocaleString(lang.value) })} · {t('settings.mediaServers.playback.watchTime')} {duration(view.watch_ms / 1000)}</p>
		{#if !view.history?.length}
			<p class="muted">{t('library.copies.nothingMatches')}</p>
		{:else}
			<ul class="history">
				{#each view.history as play (play.id)}
					<li class="history-row">
						<div>
							<strong>{@render playTitle(play)}</strong>
							<p class="muted">{[play.person || t('mediafiles.copies.unknown'), play.device, play.method === 'unknown' ? t('mediafiles.copies.unknown') : t('settings.mediaServers.playback.method.' + play.method)].filter(Boolean).join(' · ')}</p>
						</div>
						<div class="details">
							<time datetime={play.started_at}>{dateTime(play.started_at)}</time>
							<span>{duration(play.watch_ms / 1000)}{play.incomplete ? ` · ${t('settings.mediaServers.playback.gap')}` : ''}</span>
						</div>
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
	.playback { border-top: 1px solid var(--line); margin-top: var(--s-4); padding-top: var(--s-4); display: grid; gap: var(--s-3); min-width: 0; }
	h3 { font: var(--t-headline); }
	.streams, .history { list-style: none; padding: 0; margin: 0; }
	.streams { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr)); gap: var(--s-3); }
	.stream { background: var(--surface-2); border-radius: var(--r-panel); padding: var(--s-3); min-width: 0; }
	.stream-heading, .history-heading, .history-row { display: flex; justify-content: space-between; gap: var(--s-3); align-items: baseline; }
	.stream-heading, .history-row { overflow-wrap: anywhere; }
	.history-heading { margin-top: var(--s-3); flex-wrap: wrap; align-items: center; }
	.state { color: var(--accent-text); font: var(--t-footnote); }
	progress { width: 100%; height: 8px; margin-top: var(--s-3); accent-color: var(--accent-fill); }
	.filters { display: grid; grid-template-columns: minmax(0, 1fr) repeat(3, minmax(0, 140px)) auto; gap: var(--s-3); align-items: end; }
	.history-row { padding-block: var(--s-3); border-bottom: 1px solid var(--line); }
	.details { text-align: end; color: var(--text-muted); font: var(--t-footnote); display: grid; flex: none; }
	.pagination { display: flex; gap: var(--s-2); }
	@media (max-width: 940px) { .filters { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@media (max-width: 540px) { .history-row { display: grid; } .details { text-align: start; display: flex; gap: var(--s-2); } }
</style>
