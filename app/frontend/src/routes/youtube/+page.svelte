<script lang="ts">
	// The person's own YouTube videos, shaped like the Library: a toolbar with search, grid or list, sort and filter and Add; then what is downloading and the videos.
	import { tick, untrack } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { message } from '#lib/core/api.ts';
	import { bytes } from '#lib/core/format.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { dlTop, searchShortcut } from '#lib/downloads/index.ts';
	import { ask } from '#lib/ui/ask.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import {
		AddLink,
		ConfirmCard,
		DownloadStrip,
		FollowPanel,
		NeedsPrograms,
		VideoCard,
		VideoList,
		VideoSortFilter,
		VideoToolbar,
		afterUnfollow,
		applyLive,
		channelsOf,
		filterForFollow,
		filterYtVideos,
		loadYtView,
		noYtFilter,
		saveYtView,
		showsFollow,
		shownVideos,
		sortYtVideos,
		ytFiltersOn,
		ytFollows,
		ytNextSort,
		ytRemove,
		ytRetry,
		ytStateCounts,
		ytStatus,
		ytVideos,
		type YtFilter,
		type YtFollow,
		type YtLive,
		type YtProbe,
		type YtSortDir,
		type YtSortKey,
		type YtStatus,
		type YtVideo,
		type YtView
	} from '#lib/youtube/index.ts';

	// The layout sends anyone else away; nothing of the page draws for them meanwhile.
	const mayUse = $derived(session.can('youtube') && !session.kid && !session.held('youtube'));

	let status = $state<YtStatus | null>(null);
	let videos = $state<YtVideo[]>([]);
	let follows = $state<YtFollow[]>([]);
	let loaded = $state(false);
	let error = $state('');

	let probe = $state<{ probe: YtProbe; url: string } | null>(null);
	let manageOpen = $state(false);
	let focus = $state<number | null>(null);

	const saved = loadYtView(typeof localStorage === 'undefined' ? undefined : localStorage);
	let view = $state<YtView>(saved.view);
	let sort = $state(saved.sort);
	let filter = $state<YtFilter>({ ...noYtFilter });
	let q = $state('');
	let sortOpen = $state(false);
	let addOpen = $state(false);
	let findOpen = $state(false);
	let width = $state(1000);
	const phone = $derived(width < 640);

	const shown = $derived(shownVideos(videos));
	const channels = $derived(channelsOf(shown));
	const playlists = $derived(follows.filter((f) => f.kind === 'playlist'));
	const counts = $derived(ytStateCounts(shown));
	const filtersOn = $derived(ytFiltersOn(filter));
	const listed = $derived(sortYtVideos(filterYtVideos(shown, filter, q), sort));
	const countText = $derived(t('youtube.confirm.videos', { n: listed.length }));
	const toolsMissing = $derived(!!status && status.missing.length > 0);

	function keep() {
		saveYtView(typeof localStorage === 'undefined' ? undefined : localStorage, view, sort);
	}
	function setView(v: YtView) {
		view = v;
		keep();
	}
	function setSort(key: YtSortKey, dir?: YtSortDir) {
		sort = ytNextSort(sort, key, dir);
		keep();
	}
	function setFilter(patch: Partial<YtFilter>) {
		filter = { ...filter, ...patch };
	}
	function clearFilters() {
		filter = { ...noYtFilter };
		q = '';
	}

	async function load() {
		try {
			const [v, f] = await Promise.all([ytVideos(), ytFollows()]);
			videos = v;
			follows = f;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
		try {
			status = await ytStatus();
		} catch {
			// The notes above the videos wait for the next load.
		}
	}

	// A video this page does not hold yet (a check queued it), or one just finished, comes in with a fresh list.
	let soon: ReturnType<typeof setTimeout> | undefined;
	function reloadSoon() {
		clearTimeout(soon);
		soon = setTimeout(() => void load(), 800);
	}

	// Starts once the session says the person may use the page.
	$effect(() => {
		if (!mayUse) return;
		untrack(() => void load());
		const offs = [
			live.on('youtube', (data) => {
				const ev = data as YtLive;
				if (typeof ev?.id !== 'number') return;
				const known = videos.some((v) => v.id === ev.id);
				if (known) videos = applyLive(videos, ev);
				if (!known || ev.state === 'done') reloadSoon();
			}),
			live.on('resync', () => reloadSoon())
		];
		return () => {
			offs.forEach((off) => off());
			clearTimeout(soon);
		};
	});

	// Ctrl K and "/" go to the page's own search, as in the Library: there is no top bar here.
	function findKey(e: KeyboardEvent) {
		const el = e.target as HTMLElement | null;
		const inField = !!el && (el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName));
		const inDialog = !!el?.closest?.('[role="dialog"], dialog, [popover]:popover-open') || !!document.querySelector('dialog[open]');
		if (!mayUse || !searchShortcut(e, inField, inDialog)) return;
		e.preventDefault();
		e.stopImmediatePropagation();
		if (phone) {
			findOpen = true;
			void tick().then(() => document.getElementById('find')?.focus());
		} else {
			const f = document.querySelector<HTMLInputElement>('.youtube [data-dl-search]');
			f?.focus();
			f?.select();
		}
	}
	$effect(() => {
		window.addEventListener('keydown', findKey, true);
		return () => window.removeEventListener('keydown', findKey, true);
	});

	function probed(p: YtProbe, url: string) {
		probe = { probe: p, url };
		addOpen = false;
	}

	function added(v: YtVideo) {
		videos = [v, ...videos.filter((x) => x.id !== v.id)];
		probe = null;
		toasts.push(t('youtube.added'), 'ok');
	}

	function followed(f: YtFollow) {
		follows = [...follows.filter((x) => x.id !== f.id), f];
		probe = null;
		toasts.push(t('youtube.followed', { name: f.title }), 'ok');
		reloadSoon();
	}

	function showFollow(id: number) {
		focus = id;
		manageOpen = true;
	}

	function showVideosOf(id: number) {
		const f = follows.find((x) => x.id === id);
		if (f) filter = filterForFollow(f);
		q = '';
		manageOpen = false;
	}

	function changed(f: YtFollow) {
		const before = follows.find((x) => x.id === f.id);
		follows = follows.map((x) => (x.id === f.id ? f : x));
		// A playlist just mirrored is checked at once: its videos change.
		if (before && !before.mirror && f.mirror) reloadSoon();
	}

	function unfollowed(id: number, files: 'keep' | 'delete') {
		const f = follows.find((x) => x.id === id);
		if (f && showsFollow(filter, f)) filter = { ...noYtFilter };
		follows = follows.filter((x) => x.id !== id);
		videos = afterUnfollow(videos, id, files);
		reloadSoon();
	}

	async function retry(v: YtVideo) {
		try {
			const fresh = await ytRetry(v.id);
			videos = videos.map((x) => (x.id === fresh.id ? fresh : x));
		} catch (e) {
			toasts.push(t('youtube.retryFailed'), 'fail', { body: message(e) });
		}
	}

	async function remove(v: YtVideo) {
		if (!(await ask(t('youtube.card.removeAsk', { title: v.title || v.video_id }), { yes: t('youtube.card.remove'), danger: true }))) return;
		try {
			await ytRemove(v.id);
			videos = videos.map((x) => (x.id === v.id ? { ...x, state: 'removed' } : x));
			toasts.push(t('youtube.card.removed'), 'ok');
		} catch (e) {
			toasts.push(t('youtube.card.removeFailed'), 'fail', { body: message(e) });
		}
	}

	const stateWords = (s: YtFilter['state']) =>
		s === 'done' ? t('youtube.state.done') : s === 'downloading' ? t('youtube.strip.title') : s === 'queued' ? t('youtube.state.queued') : t('youtube.state.failed');
</script>

<svelte:head><title>{t('youtube.title')} · HolyArrs</title></svelte:head>

{#if mayUse}
	<div class="youtube" bind:clientWidth={width}>
		<h1 class="sr-only">{t('youtube.title')}</h1>
		<VideoToolbar {phone} {loaded} {countText} shown={listed.length} total={shown.length} {view} {setView} {filtersOn} bind:sortOpen bind:addOpen bind:findOpen bind:q searchKey={dlTop.searchKey} sidebarCollapsed={dlTop.collapsed} onsidebar={() => dlTop.toggle()} {sortFilter} add={addBox} />
		{#if phone}
			<Sheet bind:open={sortOpen} title={t('mediafiles.toolbar.sortAndFilter')} placement="bottom">{@render sortFilter()}</Sheet>
			<Sheet bind:open={addOpen} title={t('youtube.add.label')} placement="bottom">{@render addBox()}</Sheet>
		{/if}

		{#if toolsMissing && status}
			<NeedsPrograms missing={status.missing} />
		{:else if status && !status.folder_set && session.s.role === 'owner'}
			<div class="notice warn" role="status" data-testid="youtube-no-folder">
				<Icon name="folder" size={18} />
				<div class="grow">
					<p class="lead">{t('youtube.folder.title')}</p>
					<p class="small">{t('youtube.folder.text')}</p>
				</div>
				<Button size="sm" href={to('/settings/youtube')}>{t('youtube.folder.open')}</Button>
			</div>
		{/if}
		{#if status?.low_space}
			<p class="notice warn" role="status" data-testid="youtube-low-space">
				<Icon name="alert" size={18} /><span>{t('youtube.lowSpace', { floor: bytes(status.floor_bytes) })}</span>
			</p>
		{/if}

		{#if probe}
			{#key probe}
				<ConfirmCard probe={probe.probe} url={probe.url} onvideo={added} onfollow={followed} onshowfollow={showFollow} oncancel={() => (probe = null)} />
			{/key}
		{/if}

		{#if follows.length > 0}
			<div class="following" data-testid="youtube-following">
				<span class="small muted">{t('youtube.following')}</span>
				{#each follows as f, i (f.id)}
					<button type="button" class="link small" class:paused={!f.enabled} aria-pressed={showsFollow(filter, f)} onclick={() => (filter = showsFollow(filter, f) ? { ...noYtFilter } : filterForFollow(f))}>{f.title}</button>{#if i < follows.length - 1}<span class="small muted" aria-hidden="true">·</span>{/if}
				{/each}
				<Button size="sm" variant="plain" onclick={() => { focus = null; manageOpen = true; }} data-testid="youtube-manage">{t('youtube.manage')}</Button>
			</div>
		{/if}

		<DownloadStrip videos={shown} />

		{#if filtersOn || q}
			<div class="chips" role="group" aria-label={t('library.page.activeFilters')} data-testid="youtube-active-filters">
				{#if filter.state !== 'any'}<Chip label={stateWords(filter.state)} selected onclick={() => setFilter({ state: 'any' })} onremove={() => setFilter({ state: 'any' })} />{/if}
				{#if filter.watched !== 'any'}<Chip label={filter.watched === 'watched' ? t('youtube.watched') : t('youtube.filter.unwatched')} selected onclick={() => setFilter({ watched: 'any' })} onremove={() => setFilter({ watched: 'any' })} />{/if}
				{#if filter.playlist}<Chip label={playlists.find((p) => p.id === filter.playlist)?.title ?? t('youtube.follows.playlist')} selected onclick={() => setFilter({ playlist: 0 })} onremove={() => setFilter({ playlist: 0 })} />{/if}
				{#if filter.channel}<Chip label={channels.find((c) => c.id === filter.channel)?.name ?? t('youtube.follows.channel')} selected onclick={() => setFilter({ channel: '' })} onremove={() => setFilter({ channel: '' })} />{/if}
				{#if q}<Chip label={t('library.page.searchChip', { q })} selected onclick={() => (q = '')} onremove={() => (q = '')} />{/if}
				<Button variant="plain" size="md" onclick={clearFilters}>{t('library.page.clear')}</Button>
			</div>
		{/if}

		<PageLoader loading={!loaded}>
			{#if error && videos.length === 0}
				<LoadError hasData={false} message={error} onretry={load} />
			{:else}
				{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
				{#if shown.length === 0}
					<EmptyState icon="play" title={t('youtube.empty.title')} text={t('youtube.empty.text')} />
				{:else if listed.length === 0}
					<EmptyState compact icon="search" title={t('youtube.empty.filtered')} />
				{:else if view === 'list'}
					<VideoList videos={listed} {follows} {sort} {phone} setSort={(k) => setSort(k)} onretry={retry} onremove={remove} />
				{:else}
					<div class="grid" data-testid="youtube-grid">
						{#each listed as v (v.id)}
							<VideoCard video={v} onretry={retry} onremove={remove} />
						{/each}
					</div>
				{/if}
			{/if}
		</PageLoader>
	</div>

	<FollowPanel bind:open={manageOpen} {follows} {focus} onchanged={changed} onunfollowed={unfollowed} onshow={showVideosOf} />
{/if}

{#snippet sortFilter()}
	<VideoSortFilter {sort} {filter} {channels} {playlists} {counts} {loaded} {setSort} {setFilter} />
{/snippet}

{#snippet addBox()}
	<div class="add-box"><AddLink disabled={toolsMissing} onprobed={probed} /></div>
{/snippet}

<style>
	/* The Library's page frame: a column under the toolbar, and the sheet the list is made of. */
	.youtube { display: flex; flex-direction: column; gap: 14px; padding-bottom: 48px; min-width: 0;
		--sheet-a: light-dark(#ffffff, #212126); --sheet-b: light-dark(#f6f7fa, #17171a);
		--edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 6%));
		--drop: 0 1px 2px light-dark(rgb(20 24 40 / 5%), rgb(0 0 0 / 40%)), 0 18px 32px -20px light-dark(rgb(20 24 40 / 30%), rgb(0 0 0 / 85%)); }
	.notice p { margin: 0; }
	.notice .lead { font-weight: 600; }
	.add-box { display: grid; gap: var(--s-2); padding: 6px; }
	.following { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s-1) var(--s-2); }
	.link { border: 0; background: none; padding: 0; color: var(--text); cursor: pointer; font: inherit; }
	.link:hover { text-decoration: underline; }
	.link[aria-pressed='true'] { color: var(--accent); font-weight: 600; }
	.link.paused { color: var(--text-muted); }
	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
	.grid { display: grid; gap: var(--s-5) var(--s-4); grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); }
</style>
