<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import type { OwnedTitle } from './title-owned.svelte.ts';
	import { dayWords, epFile, epQuality, epTone } from './title-library-words.ts';
	import { ShowVersions, SeasonToolsSheet, EpisodeSheet, type SheetEpisode } from '#lib/shows/index.ts';
	import { HistoryLine as CompactRow, isRoutine, groupDownloads, downloadTone } from '#lib/downloads/index.ts';
	import { qualityBadges, ShowDeleteDialog, splitSteps, type RenameStep } from '#lib/mediafiles/index.ts';
	import { MovieVersions } from '#lib/movies/index.ts';
	import { SubtitlePanel } from '#lib/subtitles/index.ts';
	import type { TitleSubtitles } from '#lib/subtitles/index.ts';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { SvelteSet } from 'svelte/reactivity';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { LibraryFolder, PickRulesView, TitleView } from '#lib/core/types.ts';
	import { bytes, ago } from '#lib/core/format.ts';
	import { date } from '#lib/core/format.ts';

	let { own, folders, rules }: { own: OwnedTitle; folders: LibraryFolder[]; rules: PickRulesView[] } = $props();
	const manage = $derived(own.manage);
	const owned = $derived(own.owned);
	const ownTab = $derived(own.ownTab);
	const ownedEvents = $derived(own.ownedEvents);
	const busyAction = $derived(own.busyAction);
	const ownState = $derived(own.ownState);
	const ownFiles = $derived(own.ownFiles);
	const ownBadges = $derived(own.ownBadges);
	const ownDownload = $derived(own.ownDownload);

	// Routine work stays out of the title page; History shows it all.
	const ownedRows = $derived(groupDownloads(ownedEvents.filter((e) => !isRoutine(e.Kind))));

	const ownedTitles = $derived(new Map(owned ? [[owned.id, owned]] : []));

	let epOpen = $state(false);

	let epPicked = $state<SheetEpisode | null>(null);

	// Seasons start closed; each one opened loads its episodes (with files) once.
	const openSeasons = new SvelteSet<number>();

	const seasonFailed = new SvelteSet<number>();

	let seasonEps = $state<Record<number, SheetEpisode[]>>({});

	async function loadSeason(id: number, n: number) {
		try {
			const r = await api.get<{ episodes: SheetEpisode[] }>(`/titles/${id}/episodes?season=${n}&files=1`);
			seasonEps = { ...seasonEps, [n]: r.episodes ?? [] };
			seasonFailed.delete(n);
		} catch {
			// Left unset, so opening it again (or Retry) asks once more.
			seasonFailed.add(n);
		}
	}

	function toggleSeason(id: number, n: number) {
		if (openSeasons.has(n)) return void openSeasons.delete(n);
		openSeasons.add(n);
		if (!seasonEps[n]) void loadSeason(id, n);
	}

	function toggleAll(o: TitleView) {
		if (openSeasons.size) return openSeasons.clear();
		for (const se of o.show?.seasons ?? []) toggleSeason(o.id, se.number);
	}

	async function keepSeason(id: number, n: number, keep: boolean) {
		own.busyAction = `k${n}`;
		try {
			await api.put(`/titles/${id}/seasons/${n}`, { keep_up_to_date: keep });
			await own.loadOwned(id);
			if (seasonEps[n]) await loadSeason(id, n);
		} catch (e) {
			toasts.push(t('discover.titleLibrary.couldntUpdateSeason'), 'fail', { body: message(e) });
		} finally {
			own.busyAction = '';
		}
	}

	async function keepEpisode(e: SheetEpisode, n: number) {
		const keep = e.keep_up_to_date === false;
		try {
			await api.put(`/episodes/${e.id}`, { keep_up_to_date: keep });
			seasonEps = { ...seasonEps, [n]: (seasonEps[n] ?? []).map((x) => (x.id === e.id ? { ...x, keep_up_to_date: keep } : x)) };
		} catch (err) {
			toasts.push(t('discover.titleLibrary.couldntUpdateEpisode'), 'fail', { body: message(err) });
		}
	}

	// One season's files, rename and history, and the title's Edit panel.
	let toolsOpen = $state(false);

	let toolsSeason = $state(0);

	let showDeleter: ReturnType<typeof ShowDeleteDialog> | undefined = $state();

	let toolsMode = $state<'files' | 'rename' | 'history'>('files');

	function openTools(id: number, n: number) {
		toolsSeason = n;
		toolsMode = 'files';
		toolsOpen = true;
		if (!seasonEps[n]) void loadSeason(id, n);
	}

	// Look for one episode, or a whole season's missing ones.
	async function epSearch(body: Record<string, unknown>, key: string, done: string) {
		const o = owned;
		if (!o) return;
		own.busyAction = key;
		try {
			await api.post(`/titles/${o.id}/search`, body);
			toasts.push(done, 'info');
		} catch (e) {
			toasts.push(t('discover.titleLibrary.couldntStartSearch'), 'fail', { body: message(e) });
		} finally {
			own.busyAction = '';
		}
	}

	// Ready-made quality sets sit in the control (4K stays for second versions); every other set is in the Custom menu.
	const qOrder = ['Lite', 'Standard', 'Best'];

	const qHints = (): Record<string, string> => ({ Lite: t('discover.titleLibrary.hintLite'), Standard: t('discover.titleLibrary.hintStandard'), Best: t('discover.titleLibrary.hintBest'), '4K': t('discover.titleLibrary.hint4k') });

	const qKind = $derived(rules.filter((r) => r.kinds.includes(owned?.kind ?? '')));

	const qPresets = $derived(qKind.filter((r) => r.preset && r.name !== '4K').sort((a, b) => (qOrder.indexOf(a.name) + 1 || 99) - (qOrder.indexOf(b.name) + 1 || 99)));

	const qOthers = $derived(qKind.filter((r) => !qPresets.includes(r)));

	const qCustom = $derived(qOthers.find((r) => r.id === owned?.pick_rules_id));

	const qHint = $derived(qHints()[owned?.pick_rules_name ?? ''] ?? (qCustom ? (qCustom.preset ? '' : t('discover.titleLibrary.yourSet')) : ''));

	const qMenu = $derived<MenuItem[]>([
		...(qOthers.length ? [{ kind: 'heading' as const, label: t('discover.titleLibrary.moreSets') }, ...qOthers.map((r) => ({ label: r.name, checked: r.id === owned?.pick_rules_id, onselect: () => void setRules(r.id) }))] : []),
		...(qOthers.length ? [{ kind: 'separator' as const }] : []),
		{ label: t('discover.titleLibrary.newSet'), icon: 'plus', href: to('/settings/pick-rules/new') },
		{ label: t('discover.titleLibrary.editSets'), icon: 'tools', href: to(qCustom && !qCustom.preset ? `/settings/pick-rules/${qCustom.id}` : '/settings/pick-rules') }
	]);

	async function setRules(id: number) {
		const o = owned;
		if (!o || id === o.pick_rules_id) return;
		own.busyAction = 'rules';
		try {
			const r = await api.put<{ title: TitleView }>(`/titles/${o.id}`, { pick_rules_id: id });
			own.owned = r.title;
		} catch (e) {
			toasts.push(t('discover.titleLibrary.couldntChangePickRules'), 'fail', { body: message(e) });
		} finally {
			own.busyAction = '';
		}
	}

	// Settings rows that save on the spot: folder (after a short check, files move), and for shows which seasons to keep, new seasons and episode order.
	async function putTitle(body: Record<string, unknown>, busy: string, done?: string) {
		const o = owned;
		if (!o) return;
		own.busyAction = busy;
		try {
			const r = await api.put<{ title: TitleView }>(`/titles/${o.id}`, body);
			own.owned = r.title;
			if (done) toasts.push(done, 'ok');
		} catch (e) {
			toasts.push(t('discover.titleLibrary.couldntSave'), 'fail', { body: message(e) });
		} finally {
			own.busyAction = '';
		}
	}

	const kindFolders = $derived(folders.filter((f) => f.kind === owned?.kind));

	const curFolder = $derived(folders.find((f) => f.id === owned?.library_folder_id));

	let moveTo = $state<LibraryFolder | null>(null);

	async function moveFolder() {
		const o = owned, f = moveTo;
		if (!o || !f) return;
		own.busyAction = 'move';
		try {
			await api.post(`/titles/${o.id}/move`, { library_folder_id: f.id, apply: true });
			toasts.push(t('discover.titleLibrary.movingFilesTo', { label: f.label }), 'info');
			moveTo = null;
			void own.loadOwned(o.id);
		} catch (e) {
			toasts.push(t('discover.titleLibrary.couldntMoveFiles'), 'fail', { body: message(e) });
		} finally {
			own.busyAction = '';
		}
	}

	const pathEnd = (p: string) => p.split(/[\\/]+/).filter(Boolean).slice(-2).join('/');

	const folderName = (f: LibraryFolder) => (kindFolders.filter((x) => x.label === f.label).length > 1 ? pathEnd(f.path) : f.label);

	const folderMenu = $derived<MenuItem[]>(kindFolders.map((f) => ({ label: folderName(f), checked: f.id === owned?.library_folder_id, onselect: () => { if (f.id !== owned?.library_folder_id) moveTo = f; } })));

	// One choice sets every season and new seasons together; a mix made season by season reads Custom.
	type SeasonPick = 'all' | 'latest' | 'none';

	const realSeasons = $derived((owned?.show?.seasons ?? []).filter((x) => x.number > 0));

	const latestSeason = $derived(Math.max(0, ...realSeasons.map((x) => x.number)));

	const seasonPick = $derived.by((): SeasonPick | 'custom' => {
		const kept = realSeasons.filter((x) => x.keep_up_to_date);
		const newOn = owned?.show?.new_seasons !== 'none';
		if (kept.length === realSeasons.length && newOn) return 'all';
		if (kept.length === 0 && !newOn) return 'none';
		if (kept.length === 1 && kept[0].number === latestSeason && newOn) return 'latest';
		return 'custom';
	});

	const seasonPickWords = $derived<Record<SeasonPick | 'custom', string>>({ all: t('discover.titleLibrary.pickAll'), latest: t('discover.titleLibrary.pickLatest'), none: t('discover.titleLibrary.pickNone'), custom: t('discover.titleLibrary.pickCustom') });

	async function setSeasons(pick: SeasonPick) {
		const o = owned;
		if (!o) return;
		own.busyAction = 'seasons';
		try {
			for (const x of realSeasons) {
				const want = pick === 'all' || (pick === 'latest' && x.number === latestSeason);
				if (want !== x.keep_up_to_date) await api.put(`/titles/${o.id}/seasons/${x.number}`, { keep_up_to_date: want });
			}
			const newSeasons = pick === 'none' ? 'none' : 'all';
			if (o.show?.new_seasons !== newSeasons) await api.put(`/titles/${o.id}`, { new_seasons: newSeasons });
			seasonEps = {};
			openSeasons.clear();
			await own.loadOwned(o.id);
		} catch (e) {
			toasts.push(t('discover.titleLibrary.couldntSaveSeasons'), 'fail', { body: message(e) });
		} finally {
			own.busyAction = '';
		}
	}

	const seasonMenu = $derived<MenuItem[]>(([['all', t('discover.titleLibrary.allSeasons')], ['latest', t('discover.titleLibrary.latestSeason')], ['none', t('discover.titleLibrary.pickNone')]] as [SeasonPick, string][]).map(([v, l]) => ({ label: l, checked: seasonPick === v, onselect: () => void setSeasons(v) })));

	const orders = $derived<[string, string][]>([['aired', t('discover.titleLibrary.orderAired')], ['daily', t('discover.titleLibrary.orderDaily')], ['absolute', t('discover.titleLibrary.orderAbsolute')]]);

	const orderMenu = $derived<MenuItem[]>(orders.map(([v, l]) => ({ label: l, checked: owned?.show?.episode_order === v, onselect: () => void putTitle({ episode_order: v }, 'order') })));

	// An active download can be marked failed from the Library tab.
	let failing = $state(0);

	async function markFailed(downloadID: number) {
		if (!confirm(t('discover.titleLibrary.confirmMarkFailed'))) return;
		failing = downloadID;
		try {
			await api.post(`/downloads/${downloadID}/mark-failed`);
			toasts.push(t('discover.titleLibrary.markedFailed'), 'ok', { body: t('discover.titleLibrary.searchingNextRelease') });
			if (owned) await own.loadOwned(owned.id);
		} catch (e) {
			toasts.push(t('discover.titleLibrary.couldntMarkFailed'), 'fail', { body: message(e) });
		} finally {
			failing = 0;
		}
	}

	const activeDownloads = $derived((owned?.slots ?? []).filter((x) => x.download?.active));

	// One status card, the files as cards, recent activity, then settings with delete last.
	const libHead = $derived.by(() => {
		const o = owned;
		if (!o) return null;
		const k = ownState.kind;
		const pct = !o.show && ownDownload?.download ? Math.round(Math.min(Math.max(ownDownload.download.progress, 0), 1) * 100) : null;
		const main = ownFiles[0]?.file;
		const sub =
			k === 'have' && o.show ? [t('discover.titleLibrary.fullSeasons', { n: o.show.seasons.filter((x) => x.number > 0 && x.episodes && x.in_library >= x.episodes).length }), o.size_bytes ? bytes(o.size_bytes) : ''].filter(Boolean).join(' · ')
			: k === 'have' ? [ownBadges.join(' · '), main?.Size ? bytes(main.Size) : '', o.added_at ? t('discover.titleLibrary.addedAgo', { when: ago(o.added_at) }) : ''].filter(Boolean).join(' · ')
			: k === 'downloading' ? t('discover.titleLibrary.onItsWay')
			: k === 'failed' ? t('discover.titleLibrary.lastTryFailed')
			: o.keep_up_to_date ? t('discover.titleLibrary.downloadsWhenGood') : t('discover.titleLibrary.notKeptNothing');
		const action = !manage ? null : k === 'failed' ? { label: t('discover.titleLibrary.tryAgain'), icon: 'refresh' as const } : k === 'missing' ? { label: t('discover.titleLibrary.searchNow'), icon: 'search' as const } : null;
		return { kind: k, title: k === 'have' ? (o.show ? ownState.label : t('discover.titleLibrary.inYourLibrary')) : ownState.label, sub, pct, action };
	});

	// Folder, size, quality, keep, status, network and language.
	const libFacts = $derived.by(() => {
		const o = owned;
		if (!o) return [];
		const folderName = String(o.folder_path ?? '').split(/[\\/]/).filter(Boolean).pop() ?? '';
		const lang = o.original_language ? new Intl.DisplayNames(undefined, { type: 'language' }).of(o.original_language) : '';
		const out: { k: string; icon: 'folder' | 'archive' | 'star' | 'check' | 'x' | 'play' | 'tv' | 'sound'; v: string; title?: string }[] = [];
		if (folderName) out.push({ k: 'folder', icon: 'folder', v: folderName, title: o.folder_path ?? '' });
		if (o.size_bytes) out.push({ k: 'size', icon: 'archive', v: bytes(o.size_bytes) });
		if (o.pick_rules_name) out.push({ k: 'rules', icon: 'star', v: o.pick_rules_name });
		out.push({ k: 'keep', icon: o.keep_up_to_date ? 'check' : 'x', v: o.keep_up_to_date ? t('discover.titleLibrary.keptUpToDate') : t('discover.titleLibrary.notKeptUpToDate') });
		if (o.show?.status) out.push({ k: 'status', icon: 'play', v: o.show.status.replace(/^./, (c) => c.toUpperCase()) });
		if (o.show?.network) out.push({ k: 'net', icon: 'tv', v: o.show.network });
		if (lang) out.push({ k: 'lang', icon: 'sound', v: lang });
		return out;
	});

	const libFiles = $derived((owned?.kind === 'show' ? ownFiles : (owned?.slots ?? [])).map((x) => {
		const f = x.file as Record<string, any>;
		const m = (f?.MediaInfo ?? {}) as Record<string, any>;
		const langs = (v: unknown) => (Array.isArray(v) ? v : typeof v === 'string' && v ? v.split(/[\/,]/) : []).map((s) => String(s).trim().toUpperCase()).filter(Boolean).slice(0, 6);
		const name = String(f?.RelativePath ?? '').split(/[\\/]/).pop() ?? '';
		const addedOn = (iso?: string) => { if (!iso) return ''; const d = new Date(iso); return d.toLocaleDateString(appLocale(), { month: 'short', day: 'numeric', ...(d.getFullYear() !== new Date().getFullYear() ? { year: 'numeric' } : {}) }); };
		return { res: String(m.resolution ?? (f?.Quality?.resolution ? `${f.Quality.resolution}p` : '')), addedOn: addedOn(f?.AddedAt), id: x.id, versionId: x.version_id, has: !!x.file, state: x.state_label, label: x.version && x.version !== 'Main' ? x.version : t('discover.titleLibrary.mainCopy'), badges: qualityBadges([f]), codec: [m.video_codec, m.audio_codec].filter(Boolean).map((c) => String(c).toUpperCase()).join(' · '), size: f?.Size ? bytes(f.Size) : '', name, added: f?.AddedAt ? ago(f.AddedAt) : '', audio: langs(m.audio_languages), subs: langs(m.subtitle_languages ?? m.subtitles) };
	}));

	let libAll = $state(false);

	let subsOpen = $state(false);

	// Subtitles work like Quality: Default (the default languages), Off, or the title's own languages under Custom, whose menu also opens the full window and starts a search.
	let subsData = $state<TitleSubtitles | null>(null);

	async function loadSubs(id: number) {
		try {
			subsData = await api.get<TitleSubtitles>(`/titles/${id}/subtitles`);
		} catch {
			subsData = null;
		}
	}

	$effect(() => {
		const id = owned?.id;
		if (id && manage && ownTab === 'library' && !subsOpen) void loadSubs(id);
	});

	const subsOwn = $derived(subsData?.wishes.find((w) => !w.item_id && !w.version_id));

	const subsMode = $derived<'default' | 'off' | 'custom'>(!subsOwn ? 'default' : subsOwn.languages.length ? 'custom' : 'off');

	const subsCodes = (ws: { language: string }[]) => [...new Set(ws.map((w) => w.language.toUpperCase()))].join(', ');

	const subsHint = $derived.by(() => {
		if (subsMode === 'off') return t('discover.titleLibrary.noSubtitles');
		if (subsMode === 'custom') return subsCodes(subsOwn?.languages ?? []);
		const d = subsData?.files[0]?.wish?.languages ?? [];
		return d.length ? t('discover.titleLibrary.defaultCodes', { codes: subsCodes(d) }) : t('discover.titleLibrary.defaultNone');
	});

	async function setSubs(mode: 'default' | 'off') {
		const o = owned;
		if (!o || mode === subsMode) return;
		own.busyAction = 'subs';
		try {
			await api.put(`/titles/${o.id}/subtitle-wish`, mode === 'default' ? { inherit: true } : { languages: [] });
			await loadSubs(o.id);
		} catch (e) {
			toasts.push(t('discover.titleLibrary.couldntSaveSubtitles'), 'fail', { body: message(e) });
		} finally {
			own.busyAction = '';
		}
	}

	async function searchSubs() {
		const o = owned;
		if (!o) return;
		try {
			const r = await api.post<{ queued: number }>('/subtitles/search', { title_id: o.id });
			toasts.push(r.queued ? t('discover.titleLibrary.searchingSubtitles') : t('discover.titleLibrary.noFileToSearch'), r.queued ? 'info' : 'ok');
		} catch (e) {
			toasts.push(t('discover.titleLibrary.couldntSearchSubtitles'), 'fail', { body: message(e) });
		}
	}

	const subsMenu = $derived<MenuItem[]>([
		{ label: t('discover.titleLibrary.chooseLanguages'), icon: 'tools', onselect: () => (subsOpen = true) },
		{ label: t('discover.titleLibrary.lookForSubtitles'), icon: 'search', onselect: () => void searchSubs() },
		...(owned?.kind !== 'show' ? [{ label: t('discover.titleLibrary.subtitleFiles'), icon: 'copy' as const, onselect: () => (subsOpen = true) }] : [])
	]);

	// The Rename row says up front how many files would get a new name, from the same read the preview uses.
	let renameCount = $state<number | null>(null);

	$effect(() => {
		const id = owned?.id, files = owned?.slots?.length ?? 0;
		if (!id || !manage || ownTab !== 'library' || own.renameOpen) return;
		void files;
		void api.get<{ steps: RenameStep[] }>(`/titles/${id}/rename`).then((r) => (renameCount = splitSteps(r.steps).change.length), () => (renameCount = null));
	});
</script>

{#if ownTab !== 'overview'}
	<section class="own-pane" data-testid="own-pane">
	{#if owned && libHead}
		{@const o = owned}
		<div class="lib" data-testid="lib-pane">
			<div class="lib-main">
			<section class="lib-status {libHead.kind}">
				<div class="lib-st-text">
					<p class="lib-st-title"><span class="own-dot {libHead.kind}" aria-hidden="true"></span>{libHead.title}</p>
					{#if libHead.sub}<p class="lib-st-sub">{libHead.sub}</p>{/if}
						<p class="lib-facts">{#each libFacts as f (f.k)}<span class="lib-fact" title={f.title ?? f.v}><Icon name={f.icon} size={13} />{f.v}</span>{/each}</p>
					{#if libHead.pct != null}<div class="lib-bar" role="progressbar" aria-label={t('discover.titleLibrary.downloadProgress')} aria-valuenow={libHead.pct} aria-valuemin="0" aria-valuemax="100"><span style:transform="translateX({libHead.pct - 100}%)"></span></div>{/if}
				</div>
				{#if libHead.action}<button type="button" class="lib-btn primary" disabled={busyAction === 'search'} onclick={() => own.ownRun('search')}><Icon name={libHead.action.icon} size={16} />{libHead.action.label}</button>{/if}
				{#if manage}<button type="button" class="lib-btn" onclick={() => own.openPick(null)}><Icon name="grid" size={16} />{t('discover.titleLibrary.chooseARelease')}</button>{/if}
			</section>

			{#if o.show}
				<div class="lib-eps-head">
					<h3 class="lib-h">{t('discover.titleLibrary.seasons')}</h3>
					<span class="lib-head-acts">
						{#if manage && o.show.in_library < o.show.episodes - (o.show.not_out_yet ?? 0)}<button type="button" class="lib-btn sm" disabled={busyAction === 'all'} onclick={() => epSearch({}, 'all', t('discover.titleLibrary.searchingMissing'))}><Icon name="search" size={15} />{t('discover.titleLibrary.getAllMissing')}</button>{/if}
						<button type="button" class="lib-btn sm" onclick={() => toggleAll(o)}>{openSeasons.size ? t('discover.titleLibrary.collapseAll') : t('discover.titleLibrary.expandAll')}</button>
					</span>
				</div>
				<div class="lib-sl">
					{#each [...o.show.seasons].sort((x, y) => y.number - x.number) as se (se.number)}
						{@const isOpen = openSeasons.has(se.number)}
						{@const full = se.episodes > 0 && se.in_library >= se.episodes}
						<section class="lib-sr" class:open={isOpen}>
							<div class="lib-sr-head">
								{#if manage}<button type="button" class="lib-keep" class:on={se.keep_up_to_date} aria-pressed={se.keep_up_to_date} title={se.keep_up_to_date ? t('discover.titleLibrary.keptUpToDate') : t('discover.titleLibrary.notKeptUpToDate')} aria-label={t('discover.titleLibrary.keepSeasonUpTo', { number: se.number })} disabled={busyAction === `k${se.number}`} onclick={() => keepSeason(o.id, se.number, !se.keep_up_to_date)}><Icon name="check" size={13} /></button>{/if}
								<button type="button" class="lib-sr-main" aria-expanded={isOpen} onclick={() => toggleSeason(o.id, se.number)}>
									<b>{se.number === 0 ? t('discover.titleLibrary.specials') : t('discover.titleLibrary.seasonN', { n: se.number })}</b>
									<span class="lib-count" class:full class:part={!full && se.in_library > 0} class:none={!se.in_library && se.missing > 0}>{se.in_library} / {se.episodes}</span>
									{#if se.downloading}<span class="lib-sr-note dl">{t('discover.titleLibrary.downloading', { downloading: se.downloading })}</span>{/if}
									{#if se.failed}<span class="lib-sr-note fail">{t('discover.titleLibrary.failed', { failed: se.failed })}</span>{/if}
									{#if se.next_air_date}<span class="lib-sr-note">{t('discover.titleLibrary.next', { v: dayWords(se.next_air_date) })}</span>{/if}
									<span class="lib-chev" aria-hidden="true"><Icon name="chevronDown" size={16} /></span>
								</button>
								{#if manage}
									<span class="lib-sr-acts">
										<button type="button" class="lib-ico" title={t('discover.titleLibrary.getTheMissingEpisodes')} aria-label={t('discover.titleLibrary.getTheMissingEpisodes2', { number: se.number })} disabled={!se.missing || busyAction === `s${se.number}`} onclick={() => epSearch({ season: se.number }, `s${se.number}`, t('discover.titleLibrary.searchingSeason', { n: se.number }))}><Icon name="search" size={15} /></button>
										<button type="button" class="lib-ico" title={t('discover.titleLibrary.chooseAReleaseFor')} aria-label={t('discover.titleLibrary.chooseAReleaseFor2', { number: se.number })} onclick={() => own.openPick({ label: se.number === 0 ? t('discover.titleLibrary.specials') : t('discover.titleLibrary.seasonN', { n: se.number }), query: `season=${se.number}`, body: { season: se.number } })}><Icon name="grid" size={15} /></button>
										<button type="button" class="lib-ico" title={t('discover.titleLibrary.seasonTools')} aria-label={t('discover.titleLibrary.seasonTools2', { number: se.number })} onclick={() => openTools(o.id, se.number)}><Icon name="more" size={15} /></button>
									</span>
								{/if}
							</div>
							{#if isOpen}
								<div class="lib-eps">
									{#if seasonFailed.has(se.number) && !seasonEps[se.number]}<p class="form-error" role="alert">{t('discover.titleLibrary.couldnTLoadThis')} <button type="button" class="lib-link" onclick={() => loadSeason(o.id, se.number)}>{t('discover.titleLibrary.retry')}</button></p>{/if}
									{#each seasonEps[se.number] ?? [] as e (e.id)}
										{@const st = e.slots[0]}
										{@const tone = epTone(st?.state ?? '')}
										{@const aired = !e.air_date || Date.parse(e.air_date) <= Date.now()}
										<!-- The row is not a button itself, since it holds buttons: the title button opens the episode and stretches over the row. -->
										<div class="lib-ep {tone}" class:future={!aired}>
											{#if manage}<button type="button" class="lib-keep sm" class:on={e.keep_up_to_date !== false} aria-pressed={e.keep_up_to_date !== false} title={e.keep_up_to_date !== false ? t('discover.titleLibrary.keptUpToDate') : t('discover.titleLibrary.notKeptUpToDate')} aria-label={t('discover.titleLibrary.keepEpisodeUpTo', { episode: e.episode })} onclick={(ev) => { ev.stopPropagation(); void keepEpisode(e, se.number); }}><Icon name="check" size={11} /></button>{/if}
											<span class="lib-ep-still">{#if e.still}<img src={e.still.replace('/w780/', '/w300/')} alt="" loading="lazy" decoding="async" />{/if}<span class="lib-ep-n">{e.episode}</span></span>
											<button type="button" class="lib-ep-text" aria-label={t('discover.titleLibrary.episodeNamed', { episode: e.episode, name: e.name ?? '' })} onclick={() => { epPicked = e; epOpen = true; }}>
												<b>{e.name ?? t('discover.titleLibrary.episodeN', { episode: e.episode })}{#if e.episode === (seasonEps[se.number]?.length ?? 0) && se.episodes > 1}<span class="lib-tag">{t('discover.titleLibrary.finale')}</span>{/if}</b>
												<small>{[e.air_date ? dayWords(e.air_date) : '', e.runtime_minutes ? t('discover.titleLibrary.runtimeMin', { n: e.runtime_minutes }) : '', epFile(e)].filter(Boolean).join(' · ')}</small>
											</button>
											<span class="lib-ep-st {aired ? tone : 'future'}"><span class="own-dot {tone}" aria-hidden="true"></span>{aired ? epQuality(e) || serverWords(st?.state_label) || t('discover.titleLibrary.missing') : t('discover.titleLibrary.notOutYet')}</span>
											{#if manage && st}
												<span class="lib-ep-acts">
													<button type="button" class="lib-ico" title={t('discover.titleLibrary.lookForItNow')} aria-label={t('discover.titleLibrary.lookForEpisode', { episode: e.episode })} disabled={!aired || tone === 'downloading' || busyAction === `e${e.id}`} onclick={(ev) => { ev.stopPropagation(); void epSearch({ slot_id: st.id }, `e${e.id}`, t('discover.titleLibrary.searchingEpisode', { episode: e.episode })); }}><Icon name="search" size={14} /></button>
													<button type="button" class="lib-ico" title={t('discover.titleLibrary.chooseAReleaseFor3')} aria-label={t('discover.titleLibrary.chooseAReleaseFor4', { episode: e.episode })} disabled={!aired} onclick={(ev) => { ev.stopPropagation(); own.openPick({ label: `S${se.number} E${e.episode}`, query: `item_id=${e.id}`, body: { slot_id: st.id } }); }}><Icon name="grid" size={14} /></button>
												</span>
											{/if}
										</div>
									{:else}
										{#each [0, 1, 2] as i (i)}<div class="lib-ep skel"></div>{/each}
									{/each}
								</div>
							{/if}
						</section>
					{/each}
				</div>
			{:else}
				<div class="lib-eps-head">
					<h3 class="lib-h">{libFiles.length > 1 ? t('discover.titleLibrary.versions') : t('discover.titleLibrary.file')}</h3>
					{#if manage}<span class="lib-head-acts"><button type="button" class="lib-btn sm" onclick={() => void own.addVersion()}><Icon name="plus" size={15} />{t('discover.titleLibrary.addAVersion')}</button></span>{/if}
				</div>
				<div class="lib-files">
					{#each libFiles as f (f.id)}
							{@const v = o.versions.find((x) => x.id === f.versionId)}
						<article class="lib-file" data-testid="version-card">
							<p class="lib-file-top"><b>{f.label}</b>{#each f.badges as b (b)}<span class="lib-badge">{b}</span>{/each}{#if manage && v && !v.main && o.kind !== 'show'}<span class="lib-file-menu"><Menu label={t('discover.titleLibrary.versionActionsFor', { label: v.label })} items={[{ label: t('discover.titleLibrary.removeLabel', { label: v.label }), icon: 'trash', destructive: true, onselect: () => void own.movieVersions?.remove(v) }]} width={220} /></span>{/if}</p>
							{#if f.has}
								<div class="lib-tiles"><div><span>{t('discover.titleLibrary.quality')}</span><b>{f.res || '—'}</b></div><div><span>{t('discover.titleLibrary.size')}</span><b>{f.size || '—'}</b></div><div><span>{t('discover.titleLibrary.added')}</span><b>{f.addedOn || '—'}</b></div></div>
							{:else}<p class="lib-file-meta">{t('discover.titleLibrary.noFileYet', { state: f.state })}</p>{/if}
							{#if f.audio.length || f.subs.length}
								<p class="lib-langs">{#if f.audio.length}<span class="muted">{t('discover.titleLibrary.audio')}</span>{#each f.audio as l (l)}<span class="lib-lang">{l}</span>{/each}{/if}{#if f.subs.length}<span class="muted">{t('discover.titleLibrary.subtitles')}</span>{#each f.subs as l (l)}<span class="lib-lang">{l}</span>{/each}{/if}</p>
							{/if}
							{#if f.name}<p class="lib-file-name" title={f.name}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4"/></svg><span>{f.name}</span></p>{/if}
						</article>
					{:else}
						<p class="muted">{t('discover.titleLibrary.noFileYet2')}</p>
					{/each}
				</div>
			{/if}
			</div>
			<aside class="lib-side">
			<h3 class="lib-h">{t('discover.titleLibrary.history')}</h3>
			<div class="lib-hist">
				{#each libAll ? ownedRows : ownedRows.slice(0, 5) as r (r.key)}
					<CompactRow row={r} ctx={{ titles: ownedTitles }} titleById={ownedTitles} onclick={() => goto(to(`/downloads/history?title=${o.id}`))} />
				{:else}<p class="muted">{t('discover.titleLibrary.nothingHasHappenedYet')}</p>{/each}
			</div>
			{#if ownedRows.length > 5}<button type="button" class="lib-link" onclick={() => (libAll = !libAll)}>{libAll ? t('discover.titleLibrary.showLess') : t('discover.titleLibrary.showAll', { n: ownedRows.length })}</button>{/if}

			{#if manage}
				<h3 class="lib-h">{t('discover.titleLibrary.settings')}</h3>
				<div class="lib-rows">
					<button type="button" class="lib-row" role="switch" aria-checked={o.keep_up_to_date} disabled={busyAction === 'keep'} onclick={() => own.ownRun('keep')}>
						<span><b>{t('discover.titleLibrary.keepUpToDate')}</b><small>{t('discover.titleLibrary.newAndBetterCopies')}</small></span>
						<span class="lib-switch" class:on={o.keep_up_to_date} aria-hidden="true"></span>
					</button>
					{#if (!o.gate_open || !o.keep_up_to_date) && o.gate_reason}
						<div class="lib-row static">
							<span><b>{t('discover.titleLibrary.notLookingYet')}</b><small class="wrap" data-testid="gate-reason">{o.gate_reason}</small></span>
							{#if o.gate_opens_at && !o.gate_open}<span class="lib-val">{date(o.gate_opens_at)}</span>{/if}
						</div>
					{/if}
					<div class="lib-row static q-row" data-testid="quality-row">
						<span><b>{t('discover.titleLibrary.quality')}</b>{#if qHint}<small>{qHint}</small>{/if}</span>
						<div class="q-seg" role="radiogroup" aria-label={t('discover.titleLibrary.quality')}>
							{#each qPresets as r (r.id)}
								<button type="button" role="radio" aria-checked={r.id === o.pick_rules_id} class:on={r.id === o.pick_rules_id} disabled={busyAction === 'rules'} onclick={() => setRules(r.id)}>{r.name}</button>
							{/each}
							<div class="q-more" class:on={!!qCustom}>
								<Menu label={qCustom ? t('discover.titleLibrary.qualityOtherSets', { name: qCustom.name }) : t('discover.titleLibrary.customQualitySets')} variant="button" width={230} items={qMenu}>
									{#snippet trigger()}<i>{qCustom?.name ?? t('discover.titleLibrary.pickCustom')}</i><Icon name="chevronDown" size={13} />{/snippet}
								</Menu>
							</div>
						</div>
					</div>
					{#if o.show}
						<div class="lib-row static" data-testid="seasons-row">
							<span><b>{t('discover.titleLibrary.seasons')}</b></span>
							<div class="lib-pick"><Menu label={t('discover.titleLibrary.seasons')} variant="button" items={seasonMenu} width={200}>{#snippet trigger()}<span>{busyAction === 'seasons' ? t('discover.titleLibrary.saving') : seasonPickWords[seasonPick]}</span><Icon name="chevronDown" size={14} />{/snippet}</Menu></div>
						</div>
						<div class="lib-row static">
							<span><b>{t('discover.titleLibrary.episodeOrder')}</b></span>
							<div class="lib-pick"><Menu label={t('discover.titleLibrary.episodeOrder')} variant="button" items={orderMenu} width={200}>{#snippet trigger()}<span>{orders.find(([v]) => v === o.show?.episode_order)?.[1] ?? t('discover.titleLibrary.orderAired')}</span><Icon name="chevronDown" size={14} />{/snippet}</Menu></div>
						</div>
					{/if}
					<!-- Only when there is another folder of this kind to move to. -->
					{#if kindFolders.length > 1}
						<div class="lib-row static" data-testid="folder-row" title={o.folder_path ?? ''}>
							<span><b>{t('discover.titleLibrary.folder')}</b></span>
							<div class="lib-pick"><Menu label={t('discover.titleLibrary.folder')} variant="button" items={folderMenu} width={340}>{#snippet trigger()}<span>{curFolder ? folderName(curFolder) : t('discover.titleLibrary.folderWord')}</span><Icon name="chevronDown" size={14} />{/snippet}</Menu></div>
						</div>
					{/if}
					<div class="lib-row static q-row" data-testid="subtitles-row">
						<span><b>{t('discover.titleLibrary.subtitles')}</b><small>{subsData ? subsHint : ' '}</small></span>
						<div class="q-seg" role="radiogroup" aria-label={t('discover.titleLibrary.subtitles')}>
							<button type="button" role="radio" aria-checked={subsMode === 'default'} class:on={subsMode === 'default'} disabled={busyAction === 'subs'} onclick={() => setSubs('default')}>{t('discover.titleLibrary.default')}</button>
							<button type="button" role="radio" aria-checked={subsMode === 'off'} class:on={subsMode === 'off'} disabled={busyAction === 'subs'} onclick={() => setSubs('off')}>{t('discover.titleLibrary.off')}</button>
							<div class="q-more" class:on={subsMode === 'custom'}>
								<Menu label={t('discover.titleLibrary.subtitleOptions')} variant="button" width={230} items={subsMenu}>{#snippet trigger()}<i>{subsMode === 'custom' ? subsCodes(subsOwn?.languages ?? []) : t('discover.titleLibrary.pickCustom')}</i><Icon name="chevronDown" size={13} />{/snippet}</Menu>
							</div>
						</div>
					</div>
					{#if renameCount}
						<button type="button" class="lib-row" data-testid="rename-row" onclick={() => (own.renameOpen = true)}><span><b>{t('discover.titleLibrary.renameFiles')}</b><small>{renameCount === 1 ? t('discover.titleLibrary.renameOne') : t('discover.titleLibrary.renameOther', { n: renameCount })}</small></span><Icon name="chevronRight" size={16} /></button>
					{/if}
				</div>
				<div class="lib-rows danger">
					<button type="button" class="lib-row" onclick={() => (own.deleteOpen = true)}><span><b>{o.kind === 'show' ? t('discover.titleLibrary.deleteShow') : t('discover.titleLibrary.deleteMovie')}</b></span><Icon name="trash" size={16} /></button>
				</div>
			{/if}
			</aside>
		</div>
		{#if manage}
			<!-- Active downloads and Versions (subtitles open from Settings). -->
			<div class="lib-more" data-testid="lib-tools">
				{#if activeDownloads.length}
					<TitleSection id="h-downloads" title={t('discover.titleLibrary.downloading2')}>
						<ul class="tp-list">
							{#each activeDownloads as x (x.id)}
								{@const dl = x.download!}
								<li class="tp-row" data-testid="download">
									<div class="tp-row-main"><b>{x.version && o.slots.length > 1 ? `${x.version}: ` : ''}{dl.label}</b><span class="xs muted">{dl.text}</span></div>
									<div class="tp-row-end"><span class="chip {downloadTone(dl.state)}">{Math.floor(Math.min(Math.max(dl.progress, 0), 1) * 100)}%</span><button type="button" class="lib-btn sm" disabled={failing === dl.id} onclick={() => markFailed(dl.id)}>{t('discover.titleLibrary.markFailed')}</button></div>
								</li>
							{/each}
						</ul>
					</TitleSection>
				{/if}
				{#if o.kind === 'show'}<ShowVersions bind:this={own.showVersions} title={o} staff onchange={() => own.loadOwned(o.id)} />
				{:else}<MovieVersions bind:this={own.movieVersions} list={false} title={o} onchange={() => own.loadOwned(o.id)} />{/if}
			</div>
		{/if}
	{/if}
	</section>
{/if}

{#if owned && manage}
	{@const o = owned}
		<Sheet bind:open={subsOpen} title={t('discover.titleLibrary.subtitles')} description={o.name} size="lg">
			{#if subsOpen}<SubtitlePanel titleId={o.id} movie={o.kind !== 'show'} files={o.kind !== 'show'} bare />{/if}
		</Sheet>
		<Sheet open={!!moveTo} onclose={() => (moveTo = null)} title={t('discover.titleLibrary.moveTo', { folder: moveTo ? folderName(moveTo) : '' })} description={t('discover.titleLibrary.itsFilesMoveTo')} size="sm">
			<p class="lib-move-path">{moveTo?.path}</p>
			<div class="lib-move-actions">
				<button type="button" class="lib-btn" onclick={() => (moveTo = null)}>{t('discover.titleLibrary.cancel')}</button>
				<button type="button" class="lib-btn primary" disabled={busyAction === 'move'} onclick={moveFolder}>{t('discover.titleLibrary.moveFiles')}</button>
			</div>
		</Sheet>
		{#if o.show}<SeasonToolsSheet bind:open={toolsOpen} bind:mode={toolsMode} titleId={o.id} season={toolsSeason} episodes={seasonEps[toolsSeason] ?? []} onchange={() => loadSeason(o.id, toolsSeason)} ondelete={() => { toolsOpen = false; showDeleter?.open({ season: toolsSeason, label: toolsSeason === 0 ? t('discover.titleLibrary.specials') : t('discover.titleLibrary.seasonN', { n: toolsSeason }) }); }} />{/if}
		{#if o.show}<EpisodeSheet bind:open={epOpen} ep={epPicked} titleId={o.id} tmdb={o.ids?.tmdb} {manage} show={o} ondelete={() => { const e = epPicked; if (!e) return; epOpen = false; showDeleter?.open({ episodeIds: [e.id], label: `S${String(e.season ?? 0).padStart(2, '0')}E${String(e.episode).padStart(2, '0')}` }); }} />{/if}
		<!-- One season or one episode (and its files) goes through the show's delete dialog. -->
		{#if o.show}<ShowDeleteDialog bind:this={showDeleter} titleId={o.id} name={o.name} ondone={() => { seasonEps = {}; openSeasons.clear(); void own.loadOwned(o.id); }} />{/if}
{/if}

<style>
	.own-pane { display: grid; gap: 16px; padding-top: 4px; }
	.lib-more { display: flex; flex-direction: column; gap: clamp(28px, 4vw, 44px); margin-top: clamp(28px, 4vw, 44px); min-width: 0; }
	.own-dot { width: 10px; height: 10px; border-radius: 50%; background: var(--text-faint); }
	.own-dot.have { background: var(--ok); }
	.own-dot.downloading { background: var(--info); animation: own-pulse 1.4s ease-in-out infinite; }
	.own-dot.failed { background: var(--fail); }
	@keyframes own-pulse { 50% { opacity: .35; } }
	@media (prefers-reduced-motion: reduce) { .own-dot.downloading { animation: none; } }
	.lib { display: grid; gap: 0 40px; grid-template-columns: minmax(0, 1fr); }
	.lib-main, .lib-side { display: grid; align-content: start; min-width: 0; }
	@media (min-width: 1100px) {
		.lib { grid-template-columns: minmax(0, 1fr) minmax(300px, 380px); }
		.lib-side { position: sticky; top: calc(var(--topbar, 56px) + 16px); }
		.lib-side > .lib-h:first-child { margin-top: 0; }
	}
	.lib-h { margin: 28px 0 12px; font: 700 20px/1.2 var(--font-ui); letter-spacing: -.01em; }
	.lib-status { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 16px; padding: 22px 24px; border-radius: 18px; background: var(--surface-2); box-shadow: inset 0 0 0 .5px color-mix(in srgb, var(--text) 10%, transparent); }
	.lib-status.failed { background: color-mix(in srgb, var(--fail) 10%, var(--surface-2)); }
	.lib-st-text { flex: 1 1 320px; min-width: 0; }
	.lib-st-title { display: flex; align-items: center; gap: 10px; margin: 0; font: 700 22px/1.2 var(--font-ui); letter-spacing: -.01em; }
	.lib-facts { display: flex; flex-wrap: wrap; gap: 6px; margin: 12px 0 0 20px; }
	.lib-fact { display: inline-flex; align-items: center; gap: 6px; max-width: 260px; height: 26px; padding: 0 10px; border-radius: 8px; background: var(--surface-1); color: var(--text-muted); font: 600 12.5px/1 var(--font-ui); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.lib-st-sub { margin: 6px 0 0 20px; color: var(--text-muted); font: 500 14px/1.4 var(--font-ui); }
	.lib-bar { height: 6px; margin: 12px 0 0 20px; border-radius: 99px; background: color-mix(in srgb, var(--text) 10%, transparent); overflow: hidden; }
	.lib-bar span { display: block; width: 100%; height: 100%; border-radius: 99px; background: var(--info); transition: transform var(--dur-slow) var(--ease-out); }
	.lib-btn { display: inline-flex; align-items: center; gap: 8px; height: 40px; padding: 0 18px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 14px/1 var(--font-ui); white-space: nowrap; text-decoration: none; cursor: pointer; }
	.lib-btn.primary { background: var(--text); color: var(--bg); }
	.lib-btn:hover { filter: brightness(1.08); }
	.lib-head-acts { display: flex; gap: 8px; }
	.lib-sl { display: grid; gap: 8px; }
	.lib-sr { border-radius: 14px; background: var(--surface-2); overflow: hidden; }
	.lib-sr-head { display: flex; align-items: center; gap: 8px; padding: 0 10px 0 12px; min-height: 52px; }
	.lib-sr-main { flex: 1; min-width: 0; display: flex; align-items: center; gap: 10px; height: 52px; padding: 0 4px; border: 0; background: none; color: var(--text); font: inherit; text-align: left; cursor: pointer; }
	.lib-sr-main b { font: 650 16px/1 var(--font-ui); white-space: nowrap; }
	.lib-count { display: inline-flex; align-items: center; height: 22px; padding: 0 8px; border-radius: 99px; background: color-mix(in srgb, var(--text) 9%, transparent); color: var(--text); font: 700 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.lib-count.full { background: color-mix(in srgb, var(--ok) 16%, transparent); color: var(--ok-text); }
	.lib-count.part { background: color-mix(in srgb, var(--wait) 16%, transparent); color: var(--wait-text); }
	.lib-count.none { background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--fail-text); }
	.lib-sr-note { color: var(--text-muted); font: 500 13px/1 var(--font-ui); white-space: nowrap; }
	.lib-sr-note.dl { color: var(--info-text); }
	.lib-sr-note.fail { color: var(--fail-text); }
	.lib-chev { margin-left: auto; display: grid; color: var(--text-muted); transition: transform var(--dur-fast) var(--ease-out); }
	.lib-sr.open .lib-chev { transform: rotate(180deg); }
	.lib-sr-acts, .lib-ep-acts { display: flex; gap: 2px; }
	.lib-ico { display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 99px; background: none; color: var(--text-muted); cursor: pointer; }
	.lib-ico:hover:not(:disabled) { background: color-mix(in srgb, var(--text) 9%, transparent); color: var(--text); }
	.lib-ico:disabled { opacity: .3; cursor: default; }
	.lib-keep { flex: none; display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 1.5px solid color-mix(in srgb, var(--text) 50%, transparent); border-radius: 6px; background: none; color: transparent; cursor: pointer; }
	.lib-keep.on { border-color: var(--ok); background: var(--ok); color: #fff; }
	.lib-keep.sm { width: 24px; height: 24px; border-radius: 5px; }
	.lib-sr .lib-eps { padding: 0 6px 8px; border-top: .5px solid color-mix(in srgb, var(--text) 8%, transparent); }
	.lib-sr .lib-ep { grid-template-columns: 24px 96px minmax(0, 1fr) auto auto; }
	.lib-sr .lib-ep:hover, .lib-sr .lib-ep:has(.lib-ep-text:focus-visible) { background: color-mix(in srgb, var(--text) 5%, transparent); }
	.lib-ep.future .lib-ep-still img { filter: grayscale(.8) brightness(.55); }
	.lib-ep-st.future { color: var(--text-muted); background: none; box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text) 14%, transparent); }
	.lib-tag { margin-left: 8px; display: inline-flex; align-items: center; height: 18px; padding: 0 6px; border-radius: 5px; background: color-mix(in srgb, var(--info) 10%, transparent); color: var(--text); font: 700 10.5px/1 var(--font-ui); letter-spacing: .04em; text-transform: uppercase; vertical-align: 2px; }
	@media (max-width: 600px) {
		.lib-sr-note { display: none; }
		.lib-sr .lib-ep { grid-template-columns: 24px 84px minmax(0, 1fr) auto; }
		.lib-sr .lib-ep-st { grid-column: 3; grid-row: 2; }
		.lib-sr .lib-ep-still { grid-row: span 2; grid-column: 2; }
		.lib-sr .lib-ep-acts { grid-column: 4; grid-row: 1 / span 2; flex-direction: column; }
		.lib-head-acts .lib-btn.sm { padding: 0 10px; }
	}	.lib-eps-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.lib-eps-head .lib-h { margin-bottom: 14px; }
	.lib-btn.sm { height: 34px; padding: 0 14px; font-size: 13px; margin-top: 22px; }
	.lib-eps { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
	.lib-ep { display: grid; grid-template-columns: 112px minmax(0, 1fr) auto 34px; align-items: center; gap: 14px; padding: 6px; border-radius: 12px; transition: background var(--dur-fast) var(--ease-out); }
	.lib-ep { position: relative; cursor: pointer; }
	.lib-ep:hover, .lib-ep:has(.lib-ep-text:focus-visible) { background: var(--surface-2); }
	/* The title button covers the whole row; the row's own buttons sit above it. */
	button.lib-ep-text { padding: 0; border: 0; background: none; color: inherit; text-align: left; font: inherit; cursor: pointer; }
	button.lib-ep-text::after { content: ''; position: absolute; inset: 0; border-radius: inherit; }
	.lib-ep .lib-keep, .lib-ep-acts { position: relative; z-index: 1; }
	.lib-ep.skel { height: 75px; background: var(--surface-2); animation: pulse 1.2s ease-in-out infinite alternate; }
	@keyframes pulse { to { opacity: .5; } }
	.lib-ep-still { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: 10px; overflow: hidden; background: var(--surface-2); }
	.lib-ep-still img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.lib-ep.missing .lib-ep-still img { filter: grayscale(.7) brightness(.7); }
	.lib-ep-n { position: absolute; left: 6px; bottom: 6px; min-width: 22px; height: 22px; padding: 0 6px; display: grid; place-items: center; border-radius: 6px; background: rgb(0 0 0 / 62%); color: #fff; font: 700 12px/1 var(--font-ui); backdrop-filter: blur(8px); -webkit-backdrop-filter: blur(8px); }
	.lib-ep-text { display: grid; gap: 4px; min-width: 0; }
	.lib-ep-text b { font: 600 15px/1.25 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lib-ep-text small { color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); }
	.lib-ep-st { display: inline-flex; align-items: center; gap: 7px; height: 26px; padding: 0 10px; border-radius: 99px; background: color-mix(in srgb, var(--text) 7%, transparent); color: var(--text); font: 600 12px/1 var(--font-ui); white-space: nowrap; }
	.lib-ep-st.have { color: var(--ok-text); background: color-mix(in srgb, var(--ok) 12%, transparent); }
	.lib-ep-st.downloading { color: var(--info-text); background: color-mix(in srgb, var(--info) 12%, transparent); }
	.lib-ep-st.failed { color: var(--fail-text); background: color-mix(in srgb, var(--fail) 12%, transparent); }
	@media (max-width: 600px) {
					.lib-btn.sm { flex: none; margin-top: 22px; }
		.lib-ep { grid-template-columns: 112px minmax(0, 1fr) 34px; gap: 12px; }
		.lib-ep-st { grid-column: 2; grid-row: 2; justify-self: start; height: 22px; margin-top: -6px; }
		.lib-ep-still { grid-row: span 2; }
	}
	.lib-files { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 12px; }
	.lib-file { display: grid; gap: 8px; align-content: start; padding: 18px 20px; border-radius: 16px; background: var(--surface-2); min-width: 0; }
	.lib-file p { margin: 0; }
	.lib-file-top { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; font: 700 16px/1.2 var(--font-ui); }
	.lib-file-top b { margin-right: 4px; font-weight: 700; }
	.lib-badge, .lib-lang { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 5px; border: 1px solid color-mix(in srgb, var(--text) 30%, transparent); font: 700 11px/1 var(--font-ui); letter-spacing: .03em; color: var(--text-muted); }
	.lib-lang { border-radius: 99px; }
	.lib-file-meta { color: var(--text-muted); font: 500 13px/1.4 var(--font-ui); }
	.lib-langs { display: flex; flex-wrap: wrap; align-items: center; gap: 5px; font: 500 12px/1 var(--font-ui); }
	.lib-langs .muted:not(:first-child) { margin-left: 8px; }
	.lib-file-menu { margin-left: auto; }
	.lib-tiles { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
	.lib-tiles > div { display: grid; gap: 2px; min-width: 0; padding: 9px 12px; border-radius: 10px; background: color-mix(in srgb, var(--text) 5%, transparent); }
	.lib-tiles span { color: var(--text-muted); font: 500 11.5px/1.2 var(--font-ui); }
	.lib-tiles b { font: 600 14px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lib-file-name svg { flex: none; width: 15px; height: 15px; stroke: currentColor; fill: none; stroke-width: 1.8; }
	.lib-file-name span { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	.lib-file-name { display: flex !important; align-items: center; gap: 8px; margin-top: 2px !important; padding-top: 10px; border-top: 1px solid color-mix(in srgb, var(--text) 8%, transparent); color: var(--text-muted); font: 500 12px/1.4 var(--font-mono, ui-monospace, monospace); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lib-link { justify-self: start; margin-top: 6px; padding: 0; border: 0; background: none; color: var(--accent-text); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.lib-rows { display: grid; grid-template-columns: minmax(0, 1fr); border-radius: 16px; background: var(--surface-2); overflow: hidden; }
	.lib-rows.danger { margin-top: 16px; }
	.lib-rows.danger b, .lib-rows.danger :global(svg) { color: var(--fail-text); }
	.lib-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; padding: 14px 20px; border: 0; background: none; color: var(--text); text-align: left; text-decoration: none; cursor: pointer; font: inherit; }
	.lib-row + .lib-row { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.lib-row:hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.lib-row:focus-visible, .lib-sr-main:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.lib-row span { display: grid; gap: 3px; min-width: 0; }
	.lib-row b { font: 600 15px/1.2 var(--font-ui); }
	.lib-row small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lib-row :global(svg) { flex: none; color: var(--text-muted); }
	.lib-row.static { cursor: default; }
	.lib-row.static:hover { background: none; }
	.lib-row small.wrap { white-space: normal; }
	.lib-pick { flex: none; max-width: 55%; min-width: 0; }
	.lib-pick :global(.ui-pop) { display: flex; max-width: 100%; }
	.lib-pick :global(.trigger) { max-width: 100%; min-width: 0; overflow: hidden; height: 34px; padding: 0 10px; border-radius: 9px; color: var(--text-muted); font: 500 14px/1 var(--font-ui); }
	.lib-pick :global(.trigger span) { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lib-pick :global(.trigger svg) { flex: none; }
	.lib-pick :global(.trigger:hover), .lib-pick :global(.trigger[aria-expanded='true']) { background: color-mix(in srgb, var(--text) 7%, transparent); color: var(--text); }
	.lib-move-path { margin: 0 0 16px; color: var(--text-muted); font: 500 13px/1.4 var(--font-mono, ui-monospace, monospace); overflow-wrap: anywhere; }
	.lib-move-actions { display: flex; justify-content: flex-end; gap: 8px; }
	.q-row { display: grid; gap: 10px; }
	.q-seg { display: flex; gap: 2px; padding: 3px; border-radius: 10px; background: color-mix(in srgb, var(--text) 7%, transparent); min-width: 0; }
	.q-seg > button, .q-more :global(.trigger) { flex: 1 1 auto; min-width: 0; display: inline-flex; align-items: center; justify-content: center; gap: 3px; height: 32px; padding: 0 8px; border: 0; border-radius: 8px; background: none; color: color-mix(in srgb, var(--text) 82%, transparent); font: 600 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.q-seg > button:hover:not(.on), .q-more:not(.on) :global(.trigger:hover) { color: var(--text); }
	.q-seg > button.on, .q-more.on :global(.trigger) { background: var(--surface-1, var(--bg)); color: var(--text); box-shadow: 0 0 0 1px color-mix(in srgb, var(--text) 10%, transparent), 0 1px 2px rgb(0 0 0 / .2); }
	.q-more { flex: 0 0 auto; display: flex; }
	.q-more :global(.ui-pop) { display: flex; flex: 1 1 auto; }
	.q-more :global(.trigger) { width: 100%; }
	/* The shared menu button has a taller minimum; match the other segments so Custom sits on the same line. */
	.q-more :global(.ui-pop) { align-self: stretch; }
	.q-more :global(.trigger) { min-height: 0; line-height: 1; }
	.q-more i { font-style: normal; overflow: hidden; text-overflow: ellipsis; }
	.q-seg > button, .q-more :global(.trigger) { transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.q-seg > button:focus-visible, .q-more :global(.trigger:focus-visible) { box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .q-seg > button, .q-more :global(.trigger) { height: 40px; } .lib-ico { width: 40px; height: 40px; } .lib-keep { position: relative; } .lib-keep::after { content: ''; position: absolute; inset: -10px; } }
	@media (max-width: 420px) { .q-seg > button, .q-more :global(.trigger) { padding: 0 6px; font-size: 12.5px; } }
	.lib-val { flex: none; color: var(--text-muted); font: 500 14px/1.3 var(--font-ui); text-align: right; font-variant-numeric: tabular-nums; }
	.lib-more :global(.tp-h2) { font: 700 20px/1.2 var(--font-ui); letter-spacing: -.01em; margin-bottom: 12px; }
	.lib-more :global(.tp-vrow) { grid-template-columns: minmax(0, 1fr); gap: 8px; }
	.lib-more :global(.tp-vrow > li) { flex-direction: row; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 14px 20px; border-radius: 16px; }
	.lib-more :global(.tp-vrow .tp-row-main b) { font: 600 15px/1.2 var(--font-ui); }
	.lib-more :global(.tp-vrow .tp-row-end) { justify-content: flex-end; }
	@media (max-width: 600px) { .lib-more :global(.tp-vrow > li) { flex-direction: column; padding: 12px 16px; } .lib-more :global(.tp-vrow .tp-row-end) { justify-content: flex-start; } }
	.lib-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); transition: background var(--dur-fast) var(--ease-out); }
	.lib-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; box-shadow: 0 1px 3px rgb(0 0 0 / 25%); transition: transform var(--dur-fast) var(--ease-out); }
	.lib-switch.on { background: var(--ok); }
	.lib-switch.on::after { transform: translateX(18px); }
	@media (max-width: 600px) { .lib-status { padding: 18px; } .lib-btn { flex: 1; justify-content: center; } .lib-files { grid-template-columns: 1fr; } }
	.lib-hist { display: grid; gap: 2px; margin: 0 -12px; }
	/* A 44px-tall hit area over the 16px link line on touch phones. */
</style>
