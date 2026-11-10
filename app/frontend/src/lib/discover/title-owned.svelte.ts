import { type MenuItem } from '#lib/ui/Menu.svelte';
import { ShowVersions } from '#lib/shows/index.ts';
import { qualityBadges } from '#lib/mediafiles/index.ts';
import { MovieVersions } from '#lib/movies/index.ts';
import { replaceState } from '$app/navigation';
import { page } from '$app/state';
import { tick } from 'svelte';
import { to } from '#lib/core/paths.ts';
import { api, message } from '#lib/core/api.ts';
import { session } from '#lib/core/session.svelte.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import type { EventRecord, TitleView } from '#lib/core/types.ts';
import { live } from '#lib/core/live.svelte.ts';
import { date } from '#lib/core/format.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type OwnedTitle = ReturnType<typeof createOwnedTitle>;

export function createOwnedTitle() {
		const manage = $derived(session.can('manage_library'));
		// An owned title's library record (files, versions, status), shown on this same page.
		let owned = $state<TitleView | null>(null);
		let ownedEvents = $state<EventRecord[]>([]);
		let ownedFor = 0;
		async function loadOwned(id: number) {
			ownedFor = id;
			try {
				// Both asks go out together, in one round trip.
				const events = api.get<{ events?: typeof ownedEvents }>(`/events?title=${id}&limit=30`).catch(() => ({ events: [] }));
				owned = await api.get<TitleView>(`/titles/${id}`);
				ownedEvents = (await events).events ?? [];
				ownedFailed = false;
			} catch {
				// A failed live refresh keeps what is shown; a first load says so, with Retry.
				if (owned?.id !== id) {
					owned = null;
					ownedFailed = true;
				}
			}
		}
		let ownedFailed = $state(false);
		let ownTab = $state<'overview' | 'library'>(typeof location !== 'undefined' && ['episodes', 'library'].includes(new URLSearchParams(location.search).get('tab') ?? '') ? 'library' : 'overview');
		// Keeps the open tab in the address, so reload and Back land on the same tab.
		function setTab(v: typeof ownTab) {
			ownTab = v;
			const url = new URL(location.href);
			if (v === 'overview') url.searchParams.delete('tab');
			else url.searchParams.set('tab', v);
			replaceState(url, page.state);
		}
		// The tabs of an owned title: Overview (this page) and Library (status, seasons and episodes, files, activity, settings).
		const ownTabs = $derived.by(() => {
			const o = owned;
			if (!o) return [];
			const out: { v: typeof ownTab; label: string; count?: string; alert?: boolean }[] = [{ v: 'overview', label: t('discover.titleOwned.overview') }];
			out.push({ v: 'library', label: t('discover.titleOwned.library'), alert: ownState.kind === 'failed' || (o.show?.failed ?? 0) > 0 });
			return out;
		});
		// Pick a release for the whole title, one season or one episode.
		let pickScope = $state<{ label: string; query: string; body: Record<string, unknown> } | null>(null);
		function openPick(scope: { label: string; query: string; body: Record<string, unknown> } | null) {
			pickScope = scope;
			pickOpen = true;
		}
		let busyAction = $state('');
		const ownFiles = $derived((owned?.slots ?? []).filter((x) => x.file));
		const ownBadges = $derived(qualityBadges(ownFiles.map((x) => x.file)));
		const ownDownload = $derived((owned?.slots ?? []).find((x) => x.download?.active || x.state === 'downloading' || x.state === 'importing'));
		const ownFailed = $derived((owned?.slots ?? []).some((x) => x.state === 'failed') || (owned?.show?.failed ?? 0) > 0);
		// One state for the hero button and the status line: have, downloading, failed, missing.
		const ownState = $derived.by((): { kind: 'have' | 'downloading' | 'failed' | 'missing'; label: string } => {
			const o = owned;
			if (!o) return { kind: 'missing', label: '' };
			if (o.show) {
				const sh = o.show;
				if ((sh.downloading ?? 0) > 0) return { kind: 'downloading', label: sh.downloading === 1 ? t('discover.titleOwned.downloadingEpisodeOne') : t('discover.titleOwned.downloadingEpisodeOther', { n: sh.downloading ?? 0 }) };
				if ((sh.failed ?? 0) > 0) return { kind: 'failed', label: t('discover.titleOwned.failedCount', { n: sh.failed ?? 0 }) };
				if (sh.episodes && sh.in_library >= sh.episodes - (sh.not_out_yet ?? 0)) return { kind: 'have', label: t('discover.titleOwned.inLibrary') };
				return { kind: sh.in_library ? 'have' : 'missing', label: t('discover.titleOwned.episodesOf', { have: sh.in_library, total: sh.episodes }) };
			}
			if (ownDownload) return { kind: 'downloading', label: ownDownload.download ? t('discover.titleOwned.downloadingPercent', { percent: Math.floor(Math.min(Math.max(ownDownload.download.progress, 0), 1) * 100) }) : t('discover.titleOwned.downloading') };
			if (ownFiles.length) return { kind: 'have', label: t('discover.titleOwned.inLibrary') };
			if (ownFailed) return { kind: 'failed', label: t('discover.titleOwned.downloadFailed') };
			return { kind: 'missing', label: t('discover.titleOwned.notDownloaded') };
		});
		async function ownRun(what: 'search' | 'refresh' | 'keep') {
			const o = owned;
			if (!o) return;
			busyAction = what;
			try {
				if (what === 'search') {
					await api.post(`/titles/${o.id}/search`, {});
					toasts.push(t('discover.titleOwned.searchingFor', { name: o.name }), 'info');
				} else if (what === 'refresh') {
					await api.post(`/titles/${o.id}/refresh`);
					toasts.push(t('discover.titleOwned.refreshing', { name: o.name }), 'info', { body: t('discover.titleOwned.refreshingBody') });
				} else {
					const r = await api.put<{ title: TitleView }>(`/titles/${o.id}`, { keep_up_to_date: !o.keep_up_to_date });
					owned = r.title;
				}
			} catch (e) {
				toasts.push(what === 'search' ? t('discover.titleOwned.couldntSearch') : what === 'refresh' ? t('discover.titleOwned.couldntRefresh') : t('discover.titleOwned.couldntSave'), 'fail', { body: message(e) });
			} finally {
				busyAction = '';
			}
		}
		// Every manager tool, grouped; all of them open on this page (sheets, or the Library tab).
		type OwnTool = { label: string; icon: 'search' | 'grid' | 'copy' | 'plus' | 'tools' | 'check' | 'refresh' | 'clock' | 'trash'; href?: string; run?: () => void; busy?: boolean; danger?: boolean };
		const ownTools = $derived.by((): { group: string; items: OwnTool[] }[] => {
			const o = owned;
			if (!o) return [];
			return [
				{ group: t('discover.titleOwned.groupFind'), items: [
					{ label: t('discover.titleOwned.searchNow'), icon: 'search', run: () => ownRun('search'), busy: busyAction === 'search' },
					{ label: t('discover.titleOwned.chooseRelease'), icon: 'grid', run: () => (pickOpen = true) }
				] },
				{ group: t('discover.titleOwned.groupFiles'), items: [
					{ label: t('discover.titleOwned.renameFiles'), icon: 'copy', run: () => (renameOpen = true) },
					{ label: t('discover.titleOwned.addVersion'), icon: 'plus', run: () => void addVersion() },
					{ label: t('discover.titleOwned.settings'), icon: 'tools', run: () => setTab('library') }
				] },
				{ group: t('discover.titleOwned.groupKeep'), items: [
					{ label: o.keep_up_to_date ? t('discover.titleOwned.stopKeeping') : t('discover.titleOwned.keepUpToDate'), icon: 'check', run: () => ownRun('keep'), busy: busyAction === 'keep' },
					{ label: t('discover.titleOwned.refreshDetails'), icon: 'refresh', run: () => ownRun('refresh'), busy: busyAction === 'refresh' },
					{ label: t('discover.titleOwned.allHistory'), icon: 'clock', href: to(`/downloads/history?title=${o.id}`) },
					{ label: o.kind === 'show' ? t('discover.titleOwned.deleteShow') : t('discover.titleOwned.deleteMovie'), icon: 'trash', run: () => (deleteOpen = true), danger: true }
				] }
			];
		});
		// Add a Version: the form lives with the Versions on the Library tab.
		let movieVersions: ReturnType<typeof MovieVersions> | undefined = $state();
		let showVersions: ReturnType<typeof ShowVersions> | undefined = $state();
		async function addVersion() {
			setTab('library');
			await tick();
			await (owned?.kind === 'show' ? showVersions : movieVersions)?.openAdd();
		}
		// Keeps the Library record current while the page is open (downloads, imports, renames).
		const ownedId = $derived(owned?.id ?? 0);
		let liveTimer: ReturnType<typeof setTimeout> | undefined;
		function reloadOwned() {
			clearTimeout(liveTimer);
			liveTimer = setTimeout(() => ownedId && void loadOwned(ownedId), 400);
		}
		$effect(() => {
			const id = ownedId;
			if (!id) return;
			const offs = [
				live.on('title', reloadOwned),
				live.on('slot', reloadOwned),
				live.on('download', (x) => {
					const dl = x as { ID?: number; id?: number };
					const dlID = dl?.ID ?? dl?.id;
					if (dlID && owned?.slots.some((s) => s.active_download_id === dlID)) reloadOwned();
				}),
				live.on('event', (x) => {
					if ((x as { TitleID?: number | null })?.TitleID === id) reloadOwned();
				}),
				live.on('resync', reloadOwned)
			];
			return () => {
				clearTimeout(liveTimer);
				offs.forEach((off) => off());
			};
		});
		const ownMenu = $derived<MenuItem[]>(ownTools.flatMap((g, i) => [...(i ? [{ kind: 'separator' as const }] : []), ...g.items.map((t) => ({ label: t.label, icon: t.icon, href: t.href, onselect: t.run, destructive: t.danger, disabled: t.busy }))]));
		// Report a problem: anyone, on a title with files in the Library; members get it as their only More item.
		let reportOpen = $state(false);
		const mayReport = $derived(!!owned && (ownFiles.length > 0 || (owned.show?.in_library ?? 0) > 0));
		const moreMenu = $derived<MenuItem[]>([...(manage ? ownMenu : []), ...(mayReport ? [...(manage ? [{ kind: 'separator' as const }] : []), { label: t('discover.titleOwned.reportProblem'), icon: 'alert' as const, onselect: () => (reportOpen = true) }] : [])]);
		let renameOpen = $state(false);
		let deleteOpen = $state(false);
		let pickOpen = $state(typeof location !== 'undefined' && new URLSearchParams(location.search).has('pick'));

	return {
		get owned() {
			return owned;
		},
		set owned(v) {
			owned = v;
		},
		get ownedEvents() {
			return ownedEvents;
		},
		set ownedEvents(v) {
			ownedEvents = v;
		},
		get busyAction() {
			return busyAction;
		},
		set busyAction(v) {
			busyAction = v;
		},
		get ownTab() {
			return ownTab;
		},
		set ownTab(v) {
			ownTab = v;
		},
		get pickOpen() {
			return pickOpen;
		},
		set pickOpen(v) {
			pickOpen = v;
		},
		get renameOpen() {
			return renameOpen;
		},
		set renameOpen(v) {
			renameOpen = v;
		},
		get deleteOpen() {
			return deleteOpen;
		},
		set deleteOpen(v) {
			deleteOpen = v;
		},
		get reportOpen() {
			return reportOpen;
		},
		set reportOpen(v) {
			reportOpen = v;
		},
		get movieVersions() {
			return movieVersions;
		},
		set movieVersions(v) {
			movieVersions = v;
		},
		get showVersions() {
			return showVersions;
		},
		set showVersions(v) {
			showVersions = v;
		},
		get ownedFor() {
			return ownedFor;
		},
		set ownedFor(v) {
			ownedFor = v;
		},
		get manage() {
			return manage;
		},
		get ownedFailed() {
			return ownedFailed;
		},
		get ownTabs() {
			return ownTabs;
		},
		get pickScope() {
			return pickScope;
		},
		get ownFiles() {
			return ownFiles;
		},
		get ownBadges() {
			return ownBadges;
		},
		get ownDownload() {
			return ownDownload;
		},
		get ownState() {
			return ownState;
		},
		get mayReport() {
			return mayReport;
		},
		get moreMenu() {
			return moreMenu;
		},
		loadOwned,
		setTab,
		openPick,
		ownRun,
		addVersion
	};
}
