<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import Button from '#lib/ui/Button.svelte';
	import type { TrackView } from './types.ts';
	import { albumCover, albumStateOf, extraKinds, mainKinds, newAlbumChoices, pressingStatuses, toggle as toggleKind } from './labels.ts';
	import { ReleaseSheet, DeleteSheet, splitSteps, type RenameStep, RenamePreview } from '#lib/mediafiles/index.ts';
	import { HistoryLine as CompactRow, isRoutine, groupDownloads } from '#lib/downloads/index.ts';
	import { goto } from '$app/navigation';
	import { SvelteSet } from 'svelte/reactivity';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ago, bytes } from '#lib/core/format.ts';
	import type { AlbumSummary, EventRecord, LibraryFolder, PickRulesView, TitleView } from '#lib/core/types.ts';
	import { to } from '#lib/core/paths.ts';

	// This sits on the artist page's dark panel in both themes, so status text uses the *-on-art colours.

	// An artist's Library tab: what you have, the albums grouped by kind (closed until opened, with their tracks and a link to the album page), keep up to date, search and pick per album and track, activity, and settings (album kinds, quality, folder, delete).
	let { titleId, manage = false, stamp = 0, ondeleted }: { titleId: number; manage?: boolean; stamp?: number; ondeleted?: () => void } = $props();

	let title = $state<TitleView | null>(null);
	let albums = $state<AlbumSummary[]>([]);
	let events = $state<EventRecord[]>([]);
	// The same compact rows as a movie's Library tab, with routine work left out.
	const eventRows = $derived(groupDownloads(events.filter((e) => !isRoutine(e.Kind))));
	const eventTitles = $derived(new Map(title ? [[title.id, title]] : []));
	let busy = $state('');
	let show = $state<'included' | 'left_out' | 'all'>('included');
	let allEvents = $state(false);
	const open = new SvelteSet<number>();
	let tracks = $state<Record<number, TrackView[]>>({});

	async function load() {
		try {
			const [t, a, e] = await Promise.all([
				api.get<TitleView>(`/titles/${titleId}`),
				api.get<{ albums: AlbumSummary[] }>(`/titles/${titleId}/albums`),
				api.get<{ events?: typeof events }>(`/events?title=${titleId}&limit=20`).catch(() => ({ events: [] }))
			]);
			title = t;
			albums = a.albums ?? [];
			events = e.events ?? [];
		} catch (err) {
			toasts.push(t('music.couldnTLoadArtist'), 'fail', { body: message(err) });
		}
	}
	$effect(() => {
		void titleId;
		void stamp;
		void load();
	});

	async function loadTracks(id: number) {
		try {
			const r = await api.get<{ tracks: TrackView[] }>(`/albums/${id}/tracks`);
			tracks = { ...tracks, [id]: r.tracks ?? [] };
		} catch {
			tracks = { ...tracks, [id]: [] };
		}
	}
	function toggle(id: number) {
		if (open.has(id)) return void open.delete(id);
		open.add(id);
		if (!tracks[id]) void loadTracks(id);
	}

	// Groups in this order; anything else falls under Other.
	const KINDS = (): [string, string][] => [['album', t('music.albums')], ['ep', t('music.eps')], ['single', t('music.singles')], ['live', t('music.live')], ['compilation', t('music.compilations')], ['other', t('music.other')]];
	const kindOf = (a: AlbumSummary) => (a.secondary_types?.includes('live') ? 'live' : a.secondary_types?.includes('compilation') ? 'compilation' : ['album', 'ep', 'single'].includes(a.type) ? a.type : 'other');
	const kept = $derived(albums.filter((a) => a.included));
	const leftOut = $derived(albums.length - kept.length);
	const shown = $derived(show === 'all' ? albums : show === 'included' ? kept : albums.filter((a) => !a.included));
	const groups = $derived(
		KINDS().map(([k, label]) => ({ k, label, list: shown.filter((a) => kindOf(a) === k).sort((x, y) => (y.release_date ?? '').localeCompare(x.release_date ?? '')) })).filter((g) => g.list.length)
	);
	const have = $derived(kept.reduce((n, a) => n + a.in_library, 0));
	const total = $derived(kept.reduce((n, a) => n + a.tracks, 0));
	const missing = $derived(kept.reduce((n, a) => n + a.missing, 0));
	const fullAlbums = $derived(kept.filter((a) => a.tracks > 0 && a.in_library >= a.tracks).length);
	const facts = $derived.by(() => {
		const ti = title;
		if (!ti) return [];
		const out: { k: string; icon: 'folder' | 'archive' | 'star' | 'check' | 'x' | 'music'; v: string; title?: string }[] = [];
		const folder = String(ti.folder_path ?? '').split(/[\\/]/).filter(Boolean).pop();
		if (folder) out.push({ k: 'folder', icon: 'folder', v: folder, title: ti.folder_path ?? '' });
		if (ti.size_bytes) out.push({ k: 'size', icon: 'archive', v: bytes(ti.size_bytes) });
		if (ti.pick_rules_name) out.push({ k: 'rules', icon: 'star', v: ti.pick_rules_name });
		out.push({ k: 'keep', icon: ti.keep_up_to_date ? 'check' : 'x', v: ti.keep_up_to_date ? t('music.keptUpToDate') : t('music.notKeptUpTo') });
		out.push({ k: 'albums', icon: 'music', v: t('music.ofReleasesKept', { kept: kept.length, total: albums.length }) });
		return out;
	});

	const year = (iso?: string) => (iso ? iso.slice(0, 4) : '');
	const len = (ms?: number) => {
		if (!ms) return '';
		const sec = Math.round(ms / 1000);
		return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`;
	};
	const tone = (st = '') => (['have', 'imported', 'in_library', 'kept'].includes(st) ? 'have' : ['downloading', 'importing', 'searching'].includes(st) ? 'downloading' : st === 'failed' ? 'failed' : 'missing');

	async function run(key: string, fn: () => Promise<unknown>, done?: string) {
		busy = key;
		try {
			await fn();
			if (done) toasts.push(done, 'ok');
		} catch (e) {
			toasts.push(t('music.couldnTSave'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}
	const search = (body: Record<string, unknown>, key: string, done: string) => run(key, () => api.post(`/titles/${titleId}/search`, body), done);
	const keepAlbum = (a: AlbumSummary) =>
		run(
			`k${a.id}`,
			async () => {
				await api.put(`/albums/${a.id}`, { keep_up_to_date: !a.keep_up_to_date });
				await load();
			},
			a.keep_up_to_date ? t('music.stoppedUpdating', { name: a.name }) : t('music.keepingUpToDate', { name: a.name })
		);
	const keepTitle = () =>
		run(
			'keep',
			async () => {
				if (!title) return;
				await api.put(`/titles/${titleId}`, { keep_up_to_date: !title.keep_up_to_date });
				await load();
			},
			title?.keep_up_to_date ? t('music.stoppedUpdatingArtist') : t('music.keepingArtistUpTo')
		);
	const refresh = () => run('refresh', () => api.post(`/titles/${titleId}/refresh`), t('music.refreshingArtist'));
	const albumPage = (a: AlbumSummary) => to(`/library/${titleId}/albums/${a.id}`);

	// Album kinds: what counts for this artist, and what happens to albums MusicBrainz lists later. Saving brings the newly included albums in.
	let kindsOpen = $state(false);
	let eNew = $state('all');
	let eTypes = $state<string[]>([]);
	let eExtras = $state<string[]>([]);
	let eStatuses = $state<string[]>([]);
	let kindsError = $state('');
	let saving = $state(false);
	type KindPick = 'albums' | 'eps' | 'singles';
	const kindSets: Record<KindPick, string[]> = { albums: ['album'], eps: ['album', 'ep'], singles: ['album', 'ep', 'single'] };
	const kindWords = (): Record<KindPick | 'custom', string> => ({ albums: t('music.albums'), eps: t('music.albumsAndEps'), singles: t('music.albumsEpsAndSingles'), custom: t('music.yourOwnMix') });
	const same = (a: string[] = [], b: string[] = []) => a.length === b.length && a.every((x) => b.includes(x));
	const kindPick = $derived.by((): KindPick | 'custom' => {
		const ar = title?.artist;
		if (!ar || !same(ar.secondary_types, ['studio']) || !same(ar.release_statuses, ['official'])) return 'custom';
		return (Object.keys(kindSets) as KindPick[]).find((k) => same(ar.album_types, kindSets[k])) ?? 'custom';
	});
	const newWords = (): Record<string, string> => ({ all: t('music.newOnesKept'), new: t('music.onlyNewerOnesKept'), none: t('music.newOnesLeftOff') });
	const kindHint = $derived(`${kindWords()[kindPick]} · ${newWords()[title?.artist?.new_albums ?? 'all'] ?? ''}`);
	const setKinds = (k: KindPick) => {
		if (k === kindPick) return;
		return run('kinds', async () => {
			await api.put(`/titles/${titleId}/artist`, { new_albums: title?.artist?.new_albums ?? 'all', album_types: kindSets[k], secondary_types: ['studio'], release_statuses: ['official'] });
			await load();
		}, t('music.albumKindsSaved'));
	};
	const kindMenu = $derived<MenuItem[]>([{ label: t('music.chooseKinds'), icon: 'tools', onselect: openKinds }]);
	let moreKinds = $state(false);
	function openKinds() {
		moreKinds = false;
		const ar = title?.artist;
		kindsError = '';
		eNew = ar?.new_albums ?? 'all';
		eTypes = [...(ar?.album_types ?? [])];
		eExtras = [...(ar?.secondary_types ?? [])];
		eStatuses = [...(ar?.release_statuses ?? [])];
		kindsOpen = true;
	}
	async function saveKinds(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		kindsError = '';
		try {
			await api.put(`/titles/${titleId}/artist`, { new_albums: eNew, album_types: eTypes, secondary_types: eExtras, release_statuses: eStatuses });
			kindsOpen = false;
			toasts.push(t('music.albumKindsSaved'), 'ok', { body: t('music.refreshToAddAlbums') });
			await load();
		} catch (err) {
			kindsError = message(err);
		} finally {
			saving = false;
		}
	}

	let pickOpen = $state(false);
	let pickScope = $state<{ label: string; query: string; body: Record<string, unknown> } | null>(null);
	const pick = (s: typeof pickScope) => ((pickScope = s), (pickOpen = true));
	let deleteOpen = $state(false);
	let renameOpen = $state(false);

	// Settings like a movie's: choices on the row, a row only when it can do something.
	let rules = $state<PickRulesView[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	let renameCount = $state(0);
	$effect(() => {
		if (!manage) return;
		void api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules.filter((x) => x.kinds.includes('artist') || x.kinds.includes('music'))), () => {});
		void api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders.filter((f) => f.kind === 'music')), () => {});
	});
	$effect(() => {
		if (!manage || renameOpen) return;
		void title?.id;
		void api.get<{ steps: RenameStep[] }>(`/titles/${titleId}/rename`).then((r) => (renameCount = splitSteps(r.steps).change.length), () => (renameCount = 0));
	});
	// The three ready-made music sets, low to high, with short names on the control.
	const qOrder = ['Any music', 'Lossy 320 or V0', 'Lossless first'];
	// Keyed by the ready-made sets' names as the server sends them; the values are the words shown.
	const qShort = (): Record<string, string> => ({ 'Any music': t('music.any'), 'Lossy 320 or V0': t('music.lossy'), 'Lossless first': t('music.lossless') });
	const qHints = (): Record<string, string> => ({ 'Any music': t('music.anyQualityUpTo'), 'Lossy 320 or V0': t('music.mp3320OrV0'), 'Lossless first': t('music.flacMp3320Until') });
	const qPresets = $derived(rules.filter((r) => r.preset && qOrder.includes(r.name)).sort((a, b) => qOrder.indexOf(a.name) - qOrder.indexOf(b.name)));
	const qOthers = $derived(rules.filter((r) => !qPresets.includes(r)));
	const qCustom = $derived(qOthers.find((r) => r.id === title?.pick_rules_id));
	const qHint = $derived(qHints()[title?.pick_rules_name ?? ''] ?? (qCustom ? t('music.yourSet') : ''));
	const setRules = (id: number) => {
		if (id === title?.pick_rules_id) return;
		return run('rules', async () => {
			await api.put(`/titles/${titleId}`, { pick_rules_id: id });
			await load();
		}, '');
	};
	const qMenu = $derived<MenuItem[]>([
		...(qOthers.length ? [{ kind: 'heading' as const, label: t('music.moreSets') }, ...qOthers.map((r) => ({ label: r.name, checked: r.id === title?.pick_rules_id, onselect: () => void setRules(r.id) })), { kind: 'separator' as const }] : []),
		{ label: t('music.newSet'), icon: 'plus', href: to('/settings/pick-rules/new') },
		{ label: t('music.editSets'), icon: 'tools', href: to(qCustom && !qCustom.preset ? `/settings/pick-rules/${qCustom.id}` : '/settings/pick-rules') }
	]);
	// Only with another music folder to move to; moving asks first.
	let moveTo = $state<LibraryFolder | null>(null);
	const folderMenu = $derived<MenuItem[]>(folders.map((f) => ({ label: f.label, checked: f.id === title?.library_folder_id, onselect: () => { if (f.id !== title?.library_folder_id) moveTo = f; } })));
	const curFolder = $derived(folders.find((f) => f.id === title?.library_folder_id));
	async function moveFolder() {
		const f = moveTo;
		if (!f) return;
		await run('move', async () => {
			await api.post(`/titles/${titleId}/move`, { library_folder_id: f.id, apply: true });
			moveTo = null;
			await load();
		}, t('music.movingFilesTo', { name: f.label }));
	}
</script>

{#if title}
	{@const ti = title}
	<div class="al" data-testid="artist-library">
		<div class="al-main">
			<section class="al-status">
				<div class="al-st-text">
					<p class="al-st-title"><span class="dot {missing ? 'part' : 'have'}" aria-hidden="true"></span>{t('music.ofTracks', { have, total })}</p>
					<p class="al-st-sub">{fullAlbums === 1 ? t('music.fullRelease', { n: fullAlbums }) : t('music.fullReleases', { n: fullAlbums })}</p>
					<!-- Why nothing is searched yet (gate, kinds, keep off), as on movie and show pages. -->
					{#if ti.gate_reason}<p class="al-why" data-testid="gate-reason">{ti.gate_reason}</p>{/if}
					<p class="al-facts">{#each facts as f (f.k)}<span class="al-fact" title={f.title ?? f.v}><Icon name={f.icon} size={13} />{f.v}</span>{/each}</p>
				</div>
				{#if manage}
					{#if missing}<button type="button" class="al-btn primary" disabled={busy === 'all'} onclick={() => search({}, 'all', t('music.searchingForAllMissing'))}><Icon name="search" size={16} />{t('music.getAllMissing')}</button>{/if}
				{/if}
			</section>

			<div class="al-head">
				<h3 class="al-h">{t('music.releases')}</h3>
				<span class="al-head-acts">
					{#if leftOut}
						<span class="al-head-acts" role="group" aria-label={t('music.releasesShown')}>
							<button type="button" class="al-btn sm" class:on={show === 'included'} aria-pressed={show === 'included'} onclick={() => (show = 'included')}>{t('music.included')}</button>
							<button type="button" class="al-btn sm" class:on={show === 'left_out'} aria-pressed={show === 'left_out'} onclick={() => (show = 'left_out')}>{t('music.notIncluded2', { n: leftOut })}</button>
							<button type="button" class="al-btn sm" class:on={show === 'all'} aria-pressed={show === 'all'} onclick={() => (show = 'all')}>{t('music.all', { n: albums.length })}</button>
						</span>
					{/if}
					<button type="button" class="al-btn sm" onclick={() => (open.size ? open.clear() : shown.forEach((a) => !open.has(a.id) && toggle(a.id)))}>{open.size ? t('music.collapseAll') : t('music.expandAll')}</button>
				</span>
			</div>

			{#each groups as g (g.k)}
				<p class="al-group">{g.label} <span>{g.list.length}</span></p>
				<div class="al-list">
					{#each g.list as a (a.id)}
						{@const isOpen = open.has(a.id)}
						{@const full = a.tracks > 0 && a.in_library >= a.tracks}
						<section class="al-row" class:open={isOpen} class:off={!a.included} data-testid="album">
							<div class="al-row-head">
								{#if manage}<button type="button" class="al-keep" class:on={a.keep_up_to_date} aria-pressed={a.keep_up_to_date} aria-label={t('music.keepUpToDate', { name: a.name })} title={!a.included ? t('music.notIncludedByThe') : a.keep_up_to_date ? t('music.keptUpToDate2') : t('music.notKeptUpTo2')} disabled={busy === `k${a.id}` || !a.included} onclick={() => keepAlbum(a)}><Icon name="check" size={13} /></button>{/if}
								<button type="button" class="al-row-main" aria-expanded={isOpen} onclick={() => toggle(a.id)}>
									<img class="al-cover" src={albumCover(a.id)} alt="" loading="lazy" decoding="async" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = 'hidden')} />
									<span class="al-name"><b>{a.name}</b><small>{[year(a.release_date), a.tracks ? t('music.tracks', { n: a.tracks }) : '', a.not_out_yet && albumStateOf(a).label !== t('music.notOutYet') ? t('music.notOutYet') : '', a.dropped ? t('music.noLongerListed') : ''].filter(Boolean).join(' · ')} · <span data-testid="album-state">{albumStateOf(a).label}</span></small></span>
									{#if a.included}<span class="al-count" class:full class:part={!full && a.in_library > 0} class:none={!a.in_library && a.missing > 0} data-testid="album-count">{a.in_library} / {a.tracks}</span>{/if}
									<span class="al-chev" aria-hidden="true"><Icon name="chevronDown" size={16} /></span>
								</button>
								{#if manage}
									<span class="al-acts">
										<button type="button" class="al-ico" title={t('music.getTheMissingTracks')} aria-label={t('music.getTheMissingTracks2', { name: a.name })} disabled={!a.missing || busy === `a${a.id}`} onclick={() => search({ album_id: a.id }, `a${a.id}`, t('music.searchingFor', { name: a.name }))}><Icon name="search" size={15} /></button>
										<button type="button" class="al-ico" title={t('music.chooseARelease')} aria-label={t('music.chooseAReleaseFor', { name: a.name })} onclick={() => pick({ label: a.name, query: `album_id=${a.id}`, body: { album_id: a.id } })}><Icon name="grid" size={15} /></button>
									</span>
								{/if}
							</div>
							{#if isOpen}
								<ol class="al-tracks">
									{#each tracks[a.id] ?? [] as tr (tr.id)}
										{@const st = tr.slots[0]}
										{@const tn = tone(st?.state)}
										<li class="al-tr {tn}">
											<span class="al-tr-n">{tr.disc > 1 ? `${tr.disc}-` : ''}{tr.number}</span>
											<span class="al-tr-name">{tr.name ?? t('music.track2', { n: tr.number })}</span>
											<span class="al-tr-len">{len(tr.length_ms)}</span>
											<span class="al-tr-st {tn}"><span class="dot {tn}" aria-hidden="true"></span>{serverWords(st?.state_label) ?? t('music.missing')}</span>
											{#if manage && st}<button type="button" class="al-ico" title={t('music.lookForItNow')} aria-label={t('music.lookFor', { name: tr.name ?? '' })} disabled={tn === 'have' || tn === 'downloading' || busy === `t${tr.id}`} onclick={() => search({ slot_id: st.id }, `t${tr.id}`, tr.name ? t('music.searchingFor', { name: tr.name }) : t('music.searchingForTrack'))}><Icon name="search" size={14} /></button>{/if}
										</li>
									{:else}
										{#each [0, 1, 2] as i (i)}<li class="al-tr skel"></li>{/each}
									{/each}
								</ol>
								<a class="al-open" href={albumPage(a)} aria-label={t('music.openTheAlbumPage2', { name: a.name })}>{t('music.openTheAlbumPage')}<Icon name="chevronRight" size={15} /></a>
							{/if}
						</section>
					{/each}
				</div>
			{:else}
				<p class="muted">{show === 'left_out' ? t('music.nothingHere') : t('music.noReleasesIncludedYet')}</p>
			{/each}
		</div>

		<aside class="al-side">
			<h3 class="al-h">{t('music.history')}</h3>
			<div class="al-hist">
				{#each allEvents ? eventRows : eventRows.slice(0, 5) as r (r.key)}
					<CompactRow row={r} ctx={{ titles: eventTitles }} titleById={eventTitles} onclick={() => goto(to(`/downloads/history?title=${titleId}`))} />
				{:else}<p class="muted">{t('music.nothingHasHappenedYet')}</p>{/each}
			</div>
			{#if eventRows.length > 5}<button type="button" class="al-link" onclick={() => (allEvents = !allEvents)}>{allEvents ? t('music.showLess') : t('music.showAll', { n: eventRows.length })}</button>{/if}

			{#if manage}
				<h3 class="al-h">{t('music.settings')}</h3>
				<div class="al-rows">
					<button type="button" class="al-rowbtn" role="switch" aria-checked={ti.keep_up_to_date} disabled={busy === 'keep'} onclick={keepTitle}><span><b>{t('music.keepUpToDate2')}</b><small>{t('music.newAndBetterCopies')}</small></span><span class="al-switch" class:on={ti.keep_up_to_date} aria-hidden="true"></span></button>
					<div class="al-rowbtn static q-row" data-testid="quality-row">
						<span><b>{t('music.quality')}</b>{#if qHint}<small>{qHint}</small>{/if}</span>
						<div class="q-seg" role="radiogroup" aria-label={t('music.quality')}>
							{#each qPresets as r (r.id)}
								<button type="button" role="radio" aria-checked={r.id === ti.pick_rules_id} class:on={r.id === ti.pick_rules_id} title={r.name} disabled={busy === 'rules'} onclick={() => setRules(r.id)}>{qShort()[r.name] ?? r.name}</button>
							{/each}
							<div class="q-more" class:on={!!qCustom}>
								<Menu label={qCustom ? t('music.qualityOtherSets', { name: qCustom.name }) : t('music.customQualitySets')} variant="button" width={230} items={qMenu}>{#snippet trigger()}<i>{qCustom?.name ?? t('music.custom')}</i><Icon name="chevronDown" size={13} />{/snippet}</Menu>
							</div>
						</div>
					</div>
					<div class="al-rowbtn static q-row" data-testid="kinds-row">
						<span><b>{t('music.releases')}</b><small>{kindHint}</small></span>
						<div class="q-seg" role="radiogroup" aria-label={t('music.releases')}>
							{#each [['albums', t('music.albums')], ['eps', t('music.eps2')], ['singles', t('music.singles2')]] as [k, l] (k)}
								<button type="button" role="radio" aria-checked={kindPick === k} class:on={kindPick === k} disabled={busy === 'kinds'} onclick={() => setKinds(k as KindPick)}>{l}</button>
							{/each}
							<div class="q-more" class:on={kindPick === 'custom'}>
								<Menu label={t('music.releaseKinds')} variant="button" width={200} items={kindMenu}>{#snippet trigger()}<i>{t('music.custom')}</i><Icon name="chevronDown" size={13} />{/snippet}</Menu>
							</div>
						</div>
					</div>
					{#if folders.length > 1}
						<div class="al-rowbtn static" data-testid="folder-row">
							<span><b>{t('music.folder')}</b></span>
							<div class="al-pick"><Menu label={t('music.folder')} variant="button" items={folderMenu} width={260}>{#snippet trigger()}<i>{curFolder?.label ?? t('music.folder')}</i><Icon name="chevronDown" size={14} />{/snippet}</Menu></div>
						</div>
					{/if}
					{#if renameCount}
						<button type="button" class="al-rowbtn" onclick={() => (renameOpen = true)}><span><b>{t('music.renameFiles')}</b><small>{renameCount === 1 ? t('music.n1FileGetsA') : t('music.filesGetANew', { n: renameCount })}</small></span><Icon name="chevronRight" size={16} /></button>
					{/if}
					<button type="button" class="al-rowbtn" disabled={busy === 'refresh'} onclick={refresh}><span><b>{t('music.refreshDetails')}</b><small>{t('music.newAlbumsAndTracks')}{ti.refreshed_at ? ` · ${ago(ti.refreshed_at)}` : ''}</small></span><Icon name="refresh" size={16} /></button>
				</div>
				<div class="al-rows danger">
					<button type="button" class="al-rowbtn" onclick={() => (deleteOpen = true)}><span><b>{t('music.deleteArtist')}</b></span><Icon name="trash" size={16} /></button>
				</div>
			{/if}
		</aside>
	</div>

	{#if manage}
		<ReleaseSheet bind:open={pickOpen} {titleId} name={ti.name} scope={pickScope} onchosen={load} />
		<Sheet open={!!moveTo} onclose={() => (moveTo = null)} title={t('music.moveTo', { name: moveTo?.label ?? '' })} description={t('music.itsFilesMoveTo')} size="sm">
			<p class="al-move-path">{moveTo?.path}</p>
			<div class="al-move-actions">
				<button type="button" class="al-btn" onclick={() => (moveTo = null)}>{t('music.cancel')}</button>
				<button type="button" class="al-btn primary" disabled={busy === 'move'} onclick={moveFolder}>{t('music.moveFiles')}</button>
			</div>
		</Sheet>
		<DeleteSheet bind:open={deleteOpen} {titleId} name={ti.name} kind="artist" ondone={() => ondeleted?.()} />
		<Sheet bind:open={renameOpen} title={t('music.renameFiles')} description={ti.name} size="md">
			{#if renameOpen}<RenamePreview {titleId} onDone={() => { renameOpen = false; void load(); }} />{/if}
		</Sheet>
		<Sheet bind:open={kindsOpen} title={t('music.albumKinds')} description={ti.name} size="md">
			<form class="al-kinds" onsubmit={saveKinds}>
				<div class="field">
					<label for="al-new">{t('music.newAlbums')}</label>
					<select id="al-new" class="input" bind:value={eNew}>
						{#each newAlbumChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</div>
				<fieldset class="al-kind-set">
					<legend><b>{t('music.albumKinds')}</b></legend>
					<div class="al-boxes">
						{#each mainKinds as [v, l] (v)}<label class="al-box"><input type="checkbox" checked={eTypes.includes(v)} onchange={(e) => (eTypes = toggleKind(eTypes, v, e.currentTarget.checked, mainKinds))} /> {l}</label>{/each}
					</div>
				</fieldset>
				<button type="button" class="al-more" aria-expanded={moreKinds} onclick={() => (moreKinds = !moreKinds)}>{moreKinds ? t('music.fewerKinds') : t('music.liveCompilationsAndMore')}<Icon name="chevronDown" size={14} /></button>
				{#if moreKinds}
				<fieldset class="al-kind-set">
					<legend><b>{t('music.extraKinds')}</b></legend>
					<div class="al-boxes">
						{#each extraKinds as [v, l] (v)}<label class="al-box"><input type="checkbox" checked={eExtras.includes(v)} onchange={(e) => (eExtras = toggleKind(eExtras, v, e.currentTarget.checked, extraKinds))} /> {l}</label>{/each}
					</div>
				</fieldset>
				<fieldset class="al-kind-set">
					<legend><b>{t('music.pressingsThatCount')}</b></legend>
					<div class="al-boxes">
						{#each pressingStatuses as [v, l] (v)}<label class="al-box"><input type="checkbox" checked={eStatuses.includes(v)} onchange={(e) => (eStatuses = toggleKind(eStatuses, v, e.currentTarget.checked, pressingStatuses))} /> {l}</label>{/each}
					</div>
				</fieldset>
				{/if}
				{#if kindsError}<p class="form-error" role="alert">{kindsError}</p>{/if}
				<div class="al-kinds-acts">
					<Button variant="plain" onclick={() => (kindsOpen = false)}>{t('music.cancel')}</Button>
					<Button variant="primary" type="submit" disabled={saving || !eTypes.length || !eExtras.length || !eStatuses.length} busy={saving}>{t('music.save')}</Button>
				</div>
			</form>
		</Sheet>
	{/if}
{:else}
	<div class="al-skel" aria-busy="true"></div>
{/if}

<style>
	.al { display: grid; gap: 0 40px; grid-template-columns: minmax(0, 1fr); margin-top: 8px; }
	.al-main, .al-side { display: grid; align-content: start; min-width: 0; }
	@media (min-width: 1100px) {
		.al { grid-template-columns: minmax(0, 1fr) minmax(300px, 360px); }
		.al-side { position: sticky; top: calc(var(--topbar, 56px) + 16px); }
		.al-side > .al-h:first-child { margin-top: 0; }
	}
	.al-skel { height: 220px; border-radius: 18px; background: var(--surface-2); }
	.al-h { margin: 28px 0 12px; font: 700 20px/1.2 var(--font-ui); letter-spacing: -.01em; }
	.al-status { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 12px; padding: 20px 22px; border-radius: 18px; background: var(--surface-2); box-shadow: inset 0 0 0 .5px color-mix(in srgb, var(--text) 10%, transparent); }
	.al-st-text { flex: 1 1 320px; min-width: 0; }
	.al-st-title { display: flex; align-items: center; gap: 10px; margin: 0; font: 700 22px/1.2 var(--font-ui); letter-spacing: -.01em; }
	.al-why { margin: 10px 0 0 20px; max-width: 70ch; color: var(--text); font: 500 14px/1.4 var(--font-ui); }
	.al-st-sub { margin: 6px 0 0 20px; color: var(--text-muted); font: 500 14px/1.4 var(--font-ui); }
	.al-facts { display: flex; flex-wrap: wrap; gap: 6px; margin: 12px 0 0 20px; }
	.al-fact { display: inline-flex; align-items: center; gap: 6px; max-width: 260px; height: 26px; padding: 0 10px; border-radius: 8px; background: var(--surface-1); color: var(--text-muted); font: 600 12.5px/1 var(--font-ui); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.dot { flex: none; width: 9px; height: 9px; border-radius: 99px; background: var(--text-muted); }
	.dot.have { background: var(--ok); }
	.dot.part { background: var(--wait); }
	.dot.downloading { background: var(--info); }
	.dot.failed { background: var(--fail); }
	.al-btn { display: inline-flex; align-items: center; gap: 8px; height: 40px; padding: 0 18px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 14px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.al-btn.primary { background: var(--text); color: var(--bg); }
	.al-btn.sm { height: 34px; padding: 0 14px; font-size: 13px; }
	.al-btn.on { background: var(--text); color: var(--bg); }
	.al-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 0 12px; }
	.al-head .al-btn.sm { margin-top: 16px; }
	.al-head-acts { display: flex; flex-wrap: wrap; gap: 0 8px; }
	.al-open { display: flex; align-items: center; justify-content: flex-end; gap: 4px; min-height: var(--tap); padding: 0 16px; border-top: .5px solid color-mix(in srgb, var(--text) 8%, transparent); color: var(--accent-text); font: 600 14px/1 var(--font-ui); text-decoration: none; }
	.al-links { display: flex; flex-wrap: wrap; gap: 16px; margin: 0; }
	.al-links .al-link { text-decoration: none; }
	.al-kinds { display: grid; gap: var(--s-4); }
	.al-kind-set { margin: 0; padding: 0; border: 0; }
	.al-kind-set legend { margin-bottom: var(--s-2); font-size: var(--text-sm); }
	.al-boxes { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: var(--s-2); }
	.al-box { display: flex; align-items: center; gap: var(--s-2); min-height: var(--tap); font-size: var(--text-sm); }
	.al-kinds-acts { position: sticky; bottom: 0; display: flex; justify-content: flex-end; gap: var(--s-2); padding: 12px 0 4px; margin-bottom: -4px; background: inherit; }
	.al-more { justify-self: start; display: inline-flex; align-items: center; gap: 6px; padding: 0; border: 0; background: none; color: var(--text-muted); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.al-more[aria-expanded='true'] :global(svg) { transform: rotate(180deg); }
	.al-group { margin: 14px 0 8px; color: var(--text-muted); font: 700 12px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.al-group span { margin-left: 4px; opacity: .7; }
	.al-list { display: grid; gap: 8px; }
	.al-row { border-radius: 14px; background: var(--surface-2); overflow: hidden; }
	.al-row.off { opacity: .6; }
	.al-row-head { display: flex; align-items: center; gap: 8px; padding: 0 10px 0 12px; min-height: 64px; }
	.al-row-main { flex: 1; min-width: 0; display: flex; align-items: center; gap: 12px; height: 64px; padding: 0 4px; border: 0; background: none; color: var(--text); font: inherit; text-align: left; cursor: pointer; }
	.al-cover { flex: none; width: 46px; height: 46px; border-radius: 6px; object-fit: cover; background: color-mix(in srgb, var(--text) 8%, transparent); }
	.al-name { display: grid; gap: 3px; min-width: 0; }
	.al-name b { font: 650 15px/1.2 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.al-name small { color: var(--text-muted); font: 500 12.5px/1.2 var(--font-ui); }
	.al-count { flex: none; display: inline-flex; align-items: center; height: 22px; padding: 0 8px; border-radius: 99px; background: color-mix(in srgb, var(--text) 9%, transparent); color: var(--text); font: 700 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.al-count.full { background: color-mix(in srgb, var(--ok) 16%, transparent); color: var(--ok-on-art); }
	.al-count.part { background: color-mix(in srgb, var(--wait) 16%, transparent); color: var(--wait-on-art); }
	.al-count.none { background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--text); }
	.al-chev { margin-left: auto; display: grid; color: var(--text-muted); transition: transform var(--dur-fast) var(--ease-out); }
	.al-row.open .al-chev { transform: rotate(180deg); }
	.al-acts { display: flex; gap: 2px; }
	.al-ico { display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 99px; background: none; color: var(--text-muted); cursor: pointer; }
	.al-ico:hover:not(:disabled) { background: color-mix(in srgb, var(--text) 9%, transparent); color: var(--text); }
	.al-ico:disabled { opacity: .3; cursor: default; }
	/* The box is small; its hit area reaches 44px. */
	.al-keep::after { content: ''; position: absolute; inset: -11px; }
	.al-keep:disabled { opacity: .35; cursor: default; }
	.al-keep { position: relative; flex: none; display: grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 1.5px solid color-mix(in srgb, var(--text) 30%, transparent); border-radius: 6px; background: none; color: transparent; cursor: pointer; }
	.al-keep.on { border-color: var(--ok); background: var(--ok); color: #fff; }
	.al-tracks { display: grid; margin: 0; padding: 0 8px 8px; list-style: none; border-top: .5px solid color-mix(in srgb, var(--text) 8%, transparent); }
	.al-tr { display: grid; grid-template-columns: 34px minmax(0, 1fr) auto auto 32px; align-items: center; gap: 12px; min-height: 42px; padding: 0 8px; border-radius: 8px; font: 500 14px/1.2 var(--font-ui); }
	.al-tr:hover { background: color-mix(in srgb, var(--text) 5%, transparent); }
	.al-tr.skel { height: 42px; background: color-mix(in srgb, var(--text) 5%, transparent); margin-top: 4px; }
	.al-tr-n { color: var(--text-muted); font-variant-numeric: tabular-nums; text-align: right; }
	.al-tr-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.al-tr-len { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.al-tr-st { display: inline-flex; align-items: center; gap: 7px; height: 24px; padding: 0 9px; border-radius: 99px; background: color-mix(in srgb, var(--text) 7%, transparent); color: var(--text); font: 600 12px/1 var(--font-ui); white-space: nowrap; }
	.al-tr-st.have { color: var(--ok-on-art); background: color-mix(in srgb, var(--ok) 12%, transparent); }
	.al-tr-st.downloading { color: var(--info-on-art); background: color-mix(in srgb, var(--info) 12%, transparent); }
	.al-tr-st.failed { color: var(--fail-on-art); background: color-mix(in srgb, var(--fail) 12%, transparent); }
	.al-link { justify-self: start; margin-top: 8px; padding: 0; border: 0; background: none; color: inherit; font: 600 14px/1 var(--font-ui); text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
	.al-rowbtn.static { cursor: default; }
	.al-rowbtn.static:hover { background: none; }
	.al-rowbtn.static + .al-rowbtn, .al-rowbtn + .al-rowbtn.static { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.al-rowbtn.q-row { display: grid; gap: 10px; justify-content: stretch; }
	.q-seg { display: flex; gap: 2px; padding: 3px; border-radius: 10px; background: color-mix(in srgb, var(--text) 7%, transparent); min-width: 0; }
	.q-seg > button, .q-more :global(.trigger), .al-pick :global(.trigger) { flex: 1 1 auto; min-width: 0; display: inline-flex; align-items: center; justify-content: center; gap: 3px; height: 32px; padding: 0 8px; border: 0; border-radius: 8px; background: none; color: color-mix(in srgb, var(--text) 82%, transparent); font: 600 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.q-seg > button.on, .q-more.on :global(.trigger) { background: color-mix(in srgb, var(--text) 16%, transparent); color: var(--text); }
	.q-seg > button:hover:not(.on), .q-more:not(.on) :global(.trigger:hover) { color: var(--text); }
	.q-more { flex: 0 0 auto; display: flex; }
	/* The shared menu button has a taller minimum; match the other segments so Custom sits on the same line. */
	.q-more :global(.ui-pop) { align-self: stretch; }
	.q-more :global(.trigger) { min-height: 0; line-height: 1; }
	.q-more :global(.ui-pop) { display: flex; flex: 1 1 auto; }
	.q-seg i, .al-pick i { font-style: normal; overflow: hidden; text-overflow: ellipsis; }
	.al-pick { flex: none; max-width: 55%; min-width: 0; }
	.al-pick :global(.ui-pop) { display: flex; max-width: 100%; }
	.al-pick :global(.trigger) { font-weight: 500; font-size: 14px; }
	.al-move-path { margin: 0 0 16px; color: var(--text-muted); font: 500 13px/1.4 var(--font-mono, ui-monospace, monospace); overflow-wrap: anywhere; }
	.al-move-actions { display: flex; justify-content: flex-end; gap: 8px; }
	.al-hist { display: grid; gap: 2px; margin: 0 -12px; }
	.al-rows { display: grid; grid-template-columns: minmax(0, 1fr); border-radius: 16px; background: var(--surface-2); overflow: hidden; }
	.al-rows.danger { margin-top: 16px; }
	.al-rows.danger b, .al-rows.danger :global(svg) { color: var(--fail-text); }
	.al-rowbtn { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; padding: 14px 18px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; font: inherit; }
	.al-rowbtn:not(.static):hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.al-rowbtn:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	@media (pointer: coarse) { .al-more { min-height: 44px; } .q-seg > button, .q-more :global(.trigger), .al-pick :global(.trigger) { height: 40px; } }
	.al-rowbtn + .al-rowbtn { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.al-rowbtn span { display: grid; gap: 3px; min-width: 0; }
	.al-rowbtn b { font: 600 15px/1.2 var(--font-ui); }
	.al-rowbtn small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.al-rowbtn :global(svg) { flex: none; color: var(--text-muted); }
	.al-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); }
	.al-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; box-shadow: 0 1px 3px rgb(0 0 0 / 25%); transition: transform var(--dur-fast) var(--ease-out); }
	.al-switch.on { background: var(--ok); }
	.al-switch.on::after { transform: translateX(18px); }
	@media (max-width: 600px) {
		.al-status { padding: 18px; }
		.al-btn:not(.sm) { flex: 1; justify-content: center; }
		.al-btn.sm { height: var(--tap); }
		.al-links .al-link { display: inline-flex; align-items: center; min-height: var(--tap); margin-top: 0; }
		.al-tr { grid-template-columns: 26px minmax(0, 1fr) auto 32px; }
		.al-tr-len { display: none; }
		.al-row-main { gap: 10px; }
		.al-cover { width: 40px; height: 40px; }
		.al-chev { display: none; }
		.al-row-head { padding: 0 6px 0 10px; gap: 6px; }
		.al-acts .al-ico { width: 28px; }
	}
</style>
