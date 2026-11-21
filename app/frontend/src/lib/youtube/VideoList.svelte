<script lang="ts">
	// The person's videos as a list, in the Library's ledger: a small thumbnail, the title, channel, playlists, upload date, length, size and where it stands.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { bytes, date } from '#lib/core/format.ts';
	import { to } from '#lib/core/paths.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { ytSaveLink, ytThumb } from './api.ts';
	import { playlistsOf, type YtSort, type YtSortKey } from './list.ts';
	import { clock, type YtFollow, type YtVideo } from './view.ts';

	type Props = {
		videos: YtVideo[];
		follows: YtFollow[];
		sort: YtSort;
		phone: boolean;
		setSort: (key: YtSortKey) => void;
		onretry: (v: YtVideo) => Promise<void>;
		onremove: (v: YtVideo) => void;
	};
	let { videos, follows, sort, phone, setSort, onretry, onremove }: Props = $props();

	let retrying = $state<number | null>(null);
	let noThumb = $state<Record<number, boolean>>({});

	const heads = $derived<{ key?: YtSortKey; label: string; num?: boolean }[]>([
		{ key: 'title', label: t('mediafiles.list.title') },
		{ key: 'channel', label: t('youtube.follows.channel') },
		{ label: t('youtube.filter.playlists') },
		{ key: 'uploaded', label: t('youtube.sort.uploaded') },
		{ key: 'duration', label: t('youtube.sort.duration'), num: true },
		{ key: 'size', label: t('mediafiles.col.size'), num: true },
		{ label: t('mediafiles.col.status') }
	]);

	const itemsOf = (v: YtVideo): MenuItem[] => [
		...(v.state === 'done' ? [{ label: t('youtube.card.save'), icon: 'download', href: ytSaveLink(v.id) } as MenuItem] : []),
		{ label: t('youtube.card.remove'), icon: 'trash', destructive: true, onselect: () => onremove(v) }
	];

	function stateOf(v: YtVideo): { tone: string; words: string } {
		if (v.state === 'done') return v.watched_at ? { tone: 'idle', words: t('youtube.watched') } : { tone: 'ok', words: t('youtube.state.done') };
		if (v.state === 'downloading') return { tone: 'info', words: t('youtube.state.downloading', { percent: v.percent ?? 0 }) };
		if (v.state === 'failed') return { tone: 'fail', words: t('youtube.state.failed') };
		return { tone: 'wait', words: t('youtube.state.queued') };
	}

	async function retry(v: YtVideo) {
		retrying = v.id;
		try {
			await onretry(v);
		} finally {
			retrying = null;
		}
	}
</script>

<div class="rows" class:ph={phone} role="table" aria-label={t('youtube.title')} data-testid="youtube-list">
	{#if !phone}
		<div class="lh" role="row">
			<span role="columnheader"></span>
			{#each heads as h (h.label)}
				{#if h.key}
					{@const k = h.key}
					<span role="columnheader" class:num={h.num} aria-sort={sort.key === k ? (sort.dir === 'asc' ? 'ascending' : 'descending') : undefined}>
						<button type="button" class="h sort" class:num={h.num} aria-label={sort.key === k ? t('mediafiles.list.sorted', { what: h.label, dir: t(sort.dir === 'asc' ? 'mediafiles.list.lowHigh' : 'mediafiles.list.highLow') }) : h.label} aria-pressed={sort.key === k} onclick={() => setSort(k)} data-testid="youtube-sort-{k}">{h.label}{#if sort.key === k}<span class="dir">{sort.dir === 'asc' ? '↑' : '↓'}</span>{/if}</button>
					</span>
				{:else}
					<span role="columnheader" class="h">{h.label}</span>
				{/if}
			{/each}
			<span role="columnheader"></span>
		</div>
	{/if}
	{#each videos as v (v.id)}
		{@const done = v.state === 'done'}
		{@const href = done ? to(`/youtube/${v.id}`) : undefined}
		{@const st = stateOf(v)}
		{@const lists = playlistsOf(v, follows).join(', ')}
		{@const length = clock(v.duration_seconds)}
		<div class="lr entry" role="row" data-testid="youtube-row" data-state={v.state}>
			<span role="cell" class="lp">
				<svelte:element this={href ? 'a' : 'span'} class="thumb" {href} tabindex={href ? -1 : undefined} aria-hidden="true">
					{#if done && !noThumb[v.id]}
						<img src={ytThumb(v.id)} alt="" loading="lazy" decoding="async" onerror={() => (noThumb[v.id] = true)} />
					{:else}
						<Icon name="play" size={16} />
					{/if}
				</svelte:element>
			</span>
			<span role="cell" class="lt">
				{#if href}<a {href}>{v.title || v.video_id}</a>{:else}<span class="name">{v.title || v.video_id}</span>{/if}
				{#if phone}<span class="lsub">{[v.channel_name, length, st.words].filter(Boolean).join(' · ')}</span>{/if}
			</span>
			{#if !phone}
				<span role="cell" class="lc" title={v.channel_name}>{v.channel_name}</span>
				<span role="cell" class="lc" title={lists} data-testid="youtube-row-playlists">{lists}</span>
				<span role="cell" class="lc">{v.uploaded_at ? date(v.uploaded_at) : ''}</span>
				<span role="cell" class="lc num">{length}</span>
				<span role="cell" class="lc num">{v.size_bytes > 0 ? bytes(v.size_bytes) : ''}</span>
				<span role="cell" class="ls {st.tone}" title={v.state === 'failed' ? v.error : undefined}>
					<span class="dot" aria-hidden="true"></span><span class="ellipsis">{st.words}</span>
					{#if v.state === 'failed'}<Button size="sm" icon="refresh" busy={retrying === v.id} disabled={retrying === v.id} onclick={() => retry(v)} data-testid="youtube-retry">{t('youtube.retry')}</Button>{/if}
				</span>
			{/if}
			<span role="cell" class="la"><Menu label={t('youtube.card.menu')} items={itemsOf(v)} align="end" /></span>
		</div>
	{/each}
</div>

<style>
	/* The Library's ledger: one sheet, a quiet heading row, a drawn line between rows. */
	.rows { position: relative; isolation: isolate; min-width: 0; border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b) 420px); box-shadow: var(--edge), var(--drop); overflow: clip; }
	.lh, .lr { display: grid; grid-template-columns: 84px minmax(160px, 2.4fr) minmax(90px, 1fr) minmax(90px, 1fr) 104px 64px 78px minmax(150px, 190px) 44px; align-items: center; gap: 0 12px; padding-inline-end: 6px; }
	.lh { position: relative; height: 40px; background: color-mix(in srgb, var(--text) 3%, transparent); border-bottom: 1px solid var(--line); }
	.lr { position: relative; min-height: 54px; padding-block: 6px; }
	.entry::before { content: ''; position: absolute; top: 0; left: 0; right: 0; height: 1px; background: var(--line); pointer-events: none; }
	.lh + .entry::before, .entry:first-child::before { display: none; }
	.h { color: color-mix(in srgb, var(--text) 78%, transparent); font: 600 12px/1 var(--font-ui); text-align: left; white-space: nowrap; }
	.num { text-align: right; justify-self: end; }
	.sort { display: inline-flex; align-items: center; gap: 4px; padding: 6px 0; border: 0; background: none; cursor: pointer; }
	.sort[aria-pressed='true'], .sort:hover { color: var(--text); }
	.sort:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.dir { color: var(--accent); }
	.lp { padding-inline-start: 10px; }
	.thumb { display: grid; place-items: center; width: 64px; aspect-ratio: 16 / 9; overflow: hidden; border-radius: 6px; background: var(--surface-2); color: var(--text-muted); box-shadow: 0 0 0 1px var(--line); }
	.thumb img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.lt { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.lt a, .lt .name { min-width: 0; color: var(--text); font: 600 14px/1.25 var(--font-ui); text-decoration: none; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow-wrap: anywhere; }
	.lt a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.lsub { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
	.lc { min-width: 0; color: var(--text-muted); font: var(--t-callout); font-variant-numeric: tabular-nums; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ls { display: inline-flex; align-items: center; gap: 7px; min-width: 0; color: var(--text); font: var(--t-callout); white-space: nowrap; }
	.ls .dot { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); }
	.ls.ok .dot { background: var(--ok); }
	.ls.info .dot { background: var(--info); }
	.ls.wait .dot { background: var(--wait); }
	.ls.fail .dot { background: var(--fail); }
	.ls.fail { color: var(--fail-text); }
	.ls.idle { color: var(--text-muted); }
	.la { display: grid; place-items: center; }
	/* A phone row: the thumbnail, the title over "channel · length · state", the menu. */
	.rows.ph .lr { grid-template-columns: 84px minmax(0, 1fr) 44px; min-height: 60px; }
</style>
