<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import Isolated from '#lib/ui/Isolated.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import { densityLabels, loadDensity, loadPosterSize, posterLabels, saveDensity, savePosterSize, type Density, type PosterSize } from '#lib/ui/library-art.ts';
	import { reaskWhileFilling } from '#lib/music/index.ts';
	import { DlFind } from '#lib/downloads/index.ts';
	import { ImportList, ImportToolbar, adoptInParts, askThrough, canAdopt, cutFromJobs, cutKeyOf, cutsGone, findCut, findKept, holdWhile, importKey, itemsAreFiles, foldersThenNotes, jobForgotten, notesThenLocks, scansGone, staleCuts, keepAsking, partEnd, lookFail, quiet, scanEnd, scanFromFirstLeft, throughNow, type Cut, type PartResult, LibraryToolPage, PosterLightbox, TidyFolders, choices, fileName, findFolders, finished, followJob, throughJobs, scanCount, waitOf, matchKey, canTick, pickedOf, readImportView, showIdOf, sortFolders, startPicks, statusCounts, statusLabel, tickCounts, type AdoptCandidate, type AdoptPlan, type AdoptResult, type ImportFilter, type ImportView, type JobView, type Named } from '#lib/mediafiles/index.ts';
	import type { LookupArtist } from '#lib/music/index.ts';
	import { onDestroy, onMount } from 'svelte';
	import { beforeNavigate } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { ApiError, api, message } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { views as savedViews } from '#lib/core/views.svelte.ts';
	import type { LibraryFolder, LookupMovie } from '#lib/core/types.ts';
	import { unit } from '#lib/mediafiles/index.ts';
	import type { LookupShow } from '#lib/core/types.ts';
	import { appLocale, ltr, t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';

	let folders = $state<LibraryFolder[]>([]);
	let loaded = $state(false);
	let folderId = $state(0);
	let error = $state('');

	let scanning = $state(false);
	let job = $state<JobView | null>(null);
	let plan = $state<AdoptPlan | null>(null);
	/** The movie (TMDB id) or show ("<source>:<id>") chosen for each folder. */
	let sel = $state<Record<string, string>>({});
	/** Movies, shows or artists picked by a lookup, by id, to show what was chosen. */
	let named = $state<Record<string, Named>>({});
	let ticked = $state<Record<string, boolean>>({});
	let adopting = $state(false);
	// Follow what is brought in: it goes on my calendar (ticked by default).
	let followAll = $state(true);
	let results = $state<PartResult[] | null>(null);
	// A large selection goes in parts: how many folders are through so far.
	let sent = $state<{ done: number; total: number } | null>(null);
	// Every part is with the server: it goes on with this page closed, so leaving stops nothing.
	let handed = $state(false);
	// A large folder scanned in parts: where the next scan starts, kept after adopting.
	let restFrom = $state('');
	// Where this scan began, and where a scan reaches the first folder that did not go through.
	let scanFrom = '';
	let retryFrom = $state<string | null>(null);
	// The page sends the parts: leaving it stops the sending. An import cut short that way is noted
	// in this browser with its part jobs and why it stopped, so coming back asks the server how far it got.
	let gone = false;
	let cut = $state<{ key: string; lf: number; done: number; total: number } | null>(null);
	// Each tab keeps its own note, so two tabs importing never clear each other's.
	const tab = Math.random().toString(36).slice(2, 10);
	const cutBase = $derived(`holyarrs.import.running.${session.s.id}`);
	const cutKey = $derived(cutKeyOf(cutBase, tab));
	// The note found on the way in (an older page's): cleared with this tab's own.
	let wasKey = '';
	let track: Cut | null = null;
	// The key of the person who started the import: a sign-in that ends clears the session's id.
	let markKey = '';
	// live: this page is still sending, so it keeps the note fresh and other tabs leave it be.
	function noteCut(v: Cut | null, live = true) {
		track = v;
		try {
			if (v) localStorage.setItem(markKey || cutKey, JSON.stringify({ ...v, at: live ? Date.now() : 0 }));
			else {
				localStorage.removeItem(markKey || cutKey);
				if (wasKey) localStorage.removeItem(wasKey);
				wasKey = '';
			}
		} catch {
			// No storage: nothing to come back to.
		}
	}
	function noteWhy(why: Cut['why']) {
		if (track) noteCut({ ...track, why }, false);
	}
	$effect(() => {
		if (!adopting) return;
		// A note that already says why the import stopped is never made fresh again.
		const iv = setInterval(() => track?.why === 'closed' && noteCut(track), 5000);
		return () => clearInterval(iv);
	});
	// The sign-in ended mid-import: the sign-in page says the import paused.
	let lost = false;
	function signedOut() {
		noteWhy('signedOut');
		try {
			sessionStorage.setItem('holyarrs-import-paused', '1');
		} catch {
			// No storage: the sign-in page stays plain.
		}
	}
	// A scan is remembered here, so coming back (or a scan that outlasts the wait) still shows its plan.
	// Each tab keeps its own, so an import in one tab never clears a scan running in another.
	const scanBase = $derived(`holyarrs.import.scan.${session.s.id}`);
	const scanKey = $derived(cutKeyOf(scanBase, tab));
	// The scan found on the way in (an older page's): cleared with this tab's own.
	let scanWas = '';
	function noteScan(v: { lf: number; job: number; from: string } | null) {
		try {
			if (v) localStorage.setItem(scanKey, JSON.stringify(v));
			else {
				localStorage.removeItem(scanKey);
				if (scanWas) localStorage.removeItem(scanWas);
				scanWas = '';
			}
		} catch {
			// No storage: a scan is not picked up again.
		}
	}
	// When HolyArrs last answered: a few seconds of silence says so on the page.
	let heard = $state(0);
	let clock = $state(0);
	$effect(() => {
		const iv = setInterval(() => (clock = Date.now()), 1000);
		return () => clearInterval(iv);
	});
	// A part waiting on the metadata server says so, with when it tries again.
	const waiting = $derived(waitOf(job?.ProgressNote));
	const silent = $derived((adopting || scanning) && quiet(heard, clock));
	function answered<T>(p: Promise<T>): Promise<T> {
		return p.then(
			(v) => {
				heard = Date.now();
				return v;
			},
			(e) => {
				if (e instanceof ApiError && e.status === 401) lost = true;
				throw e;
			}
		);
	}

	let lookFor = $state<string | null>(null);
	let term = $state('');
	let found = $state<{ key: string; name: string; year?: number; poster?: string; title_id?: number }[]>([]);
	let looking = $state(false);
	let lookError = $state('');

	/** The kind of the picked Library folder: movies, shows or music. */
	const kind = $derived<'movie' | 'show' | 'music'>(plan?.kind ?? folderKind(folders.find((f) => f.id === folderId)?.kind));
	const word = $derived(kind === 'show' ? 'show' : kind === 'music' ? 'artist' : 'movie');
	/** The picked Library folder keeps movies as files right in it. */
	const flat = $derived(plan ? itemsAreFiles(plan) : kind === 'movie' && !!folders.find((f) => f.id === folderId)?.flat);
	const w = $derived(t(`library.import.word.${word}`));
	const ws = $derived(t(`library.import.words.${word}`));
	const lookFolder = $derived(lookFor === null ? undefined : plan?.folders.find((f) => f.folder === lookFor));
	const picks = $derived(choices(Object.fromEntries(Object.entries(ticked).filter(([, on]) => on).map(([f]) => [f, sel[f] ?? ''])), kind));
	// A scan's note says how many folders are read: shown in the person's words.
	const scanned = $derived(scanning ? scanCount(job?.ProgressNote) : null);
	const running = $derived(!!job && !['done', 'failed', 'cancelled'].includes(job.State));
	const okCount = $derived(results ? results.filter((r) => r.title_id && !r.error).length : 0);
	const unknownCount = $derived(results ? results.filter((r) => r.notKnown).length : 0);
	// Folders the metadata server did not answer for: all of them, or how many.
	const fail = $derived(plan ? lookFail(plan.folders) : null);

	// The scanned folders: unsure ones first, narrowed by the search words and the status filter.
	let q = $state('');
	let filter = $state<ImportFilter>('all');
	let findOpen = $state(false);
	let zoom = $state<AdoptCandidate | null>(null);
	const sorted = $derived(plan ? sortFolders(plan.folders, appLocale()) : []);
	const shown = $derived(findFolders(sorted, q, filter));
	const tickNow = $derived(tickCounts(shown, sel, ticked));
	const allTicked = $derived(tickNow.can > 0 && tickNow.on === tickNow.can);
	function tickShown() {
		const on = !allTicked;
		for (const f of shown) if (canTick(f, sel)) ticked[f.folder] = on;
	}
	const filters = $derived<{ value: ImportFilter; label: string; n: number }[]>([
		{ value: 'all', label: t('mediafiles.filter.all'), n: plan?.folders.length ?? 0 },
		...statusCounts(plan?.folders ?? []).map((c) => ({ value: c.status, label: statusLabel(c.status, kind), n: c.n }))
	]);
	// A filter whose folders are all gone falls back to All.
	$effect(() => {
		if (filter !== 'all' && !filters.some((c) => c.value === filter)) filter = 'all';
	});

	// The view and art size are this page's own, kept per person the way the Library keeps its own.
	const myId = $derived(session.s.id);
	let view = $state<ImportView>(readImportView(savedViews.get('library.import.view')));
	let posterSize = $state<PosterSize>('m');
	let density = $state<Density>('large');
	let planWidth = $state(1000);
	const phone = $derived(planWidth < 640);
	$effect(() => {
		posterSize = loadPosterSize(myId, undefined, 'import');
		density = loadDensity(myId, undefined, 'import');
	});
	function setView(v: ImportView) {
		view = v;
		savedViews.set('library.import.view', v);
	}
	const sizeItems = $derived<MenuItem[] | null>(
		phone || (view !== 'posters' && view !== 'banners')
			? null
			: view === 'posters'
				? [{ kind: 'heading', label: t('library.page.posterSize') }, ...(['s', 'm', 'l'] as const).map((z) => ({ label: posterLabels[z], checked: posterSize === z, onselect: () => { posterSize = z; savePosterSize(myId, z, undefined, 'import'); } }))]
				: [{ kind: 'heading', label: t('library.page.bannerSize') }, ...(['large', 'comfortable', 'compact'] as const).map((d) => ({ label: densityLabels[d], checked: density === d, onselect: () => { density = d; saveDensity(myId, d, undefined, 'import'); } }))]
	);

	function folderKind(k: string | undefined): 'movie' | 'show' | 'music' {
		return k === 'show' || k === 'music' ? k : 'movie';
	}

	let foldersFailed = $state(false);
	async function loadFolders() {
		foldersFailed = false;
		try {
			folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders.filter(
				(f) => f.kind === 'movie' || f.kind === 'show' || f.kind === 'music'
			);
			folderId = folders[0]?.id ?? 0;
		} catch {
			foldersFailed = true;
		} finally {
			loaded = true;
		}
	}
	onMount(async () => {
		let was: Cut | null = null;
		let sc: { lf: number; job: number; from?: string } | null = null;
		let scKey = '';
		// A tab still sending holds a lock: its import is running, however rarely a hidden tab writes its note.
		// The notes are read before the locks are asked, so a tab that starts in between is never called stopped;
		// both after the folders load, so a slow load never makes a note another tab keeps fresh look old.
		// No Library folders: HolyArrs (or a proxy in front of it, which answers 404 while it is down) did not answer, so no note is touched.
		const read = await foldersThenNotes(async () => (await loadFolders(), !foldersFailed), () => localStorage, cutBase, navigator.locks);
		if (read) {
			const { store, held, now } = read;
			const ids = folders.map((f) => f.id);
			try {
				// A stopped note of a Library folder that is gone can never show: it goes, and never hides another folder's note.
				for (const k of cutsGone(store, cutBase, ids, now, held)) localStorage.removeItem(k);
				const v = findCut(store, cutBase, now, held, ids);
				if (v) [was, wasKey] = [v.cut, v.key];
			} catch {
				was = null;
			}
		}
		if (was && !foldersFailed) {
			// Ask the server how far the parts got, never what the page last wrote.
			const jobs = await Promise.all(was.jobs.map((id) => api.get<JobView>(`/jobs/${id}`).catch((e) => (jobForgotten(e) ? null : undefined))));
			// A part the server no longer knows: how far it got cannot be told, so the note goes.
			// A part not answered this time: the note stays as it is for the next visit, and this tab never clears it.
			const note = cutFromJobs(was, jobs);
			// Parts still with the server go on without this page: the page follows them again.
			if (note !== undefined && jobs.some((j) => j && !finished(j))) void watchHanded(was, jobs);
			else if (note === undefined) wasKey = '';
			else if (!note) noteCut(null);
			else if (folders.some((f) => f.id === was?.lf)) {
				cut = { ...note, lf: was.lf };
				folderId = was.lf;
			}
		}
		if (!foldersFailed) {
			try {
				const ids = folders.map((f) => f.id);
				// A scan of a Library folder that is gone can never be followed: its record goes, and never hides another tab's scan.
				for (const k of scansGone(localStorage, scanBase, ids)) localStorage.removeItem(k);
				const w = findKept<{ lf: number; job: number; from?: string }>(localStorage, scanBase, (x) => typeof x.job === 'number' && ids.includes(x.lf));
				if (w) [sc, scKey] = [w.v, w.key];
			} catch {
				// No storage: no scan to pick up.
			}
		}
		if (sc && !cut && folders.some((f) => f.id === sc?.lf)) {
			scanWas = scKey;
			folderId = sc.lf;
			scanFrom = sc.from ?? '';
			void watchScan(sc.job, true);
		}
	});
	onDestroy(() => {
		gone = true;
		// A sign-in that ended somewhere else (another tab) takes the page away without a page change of its own.
		if (adopting && !handed && session.s.state !== 'signed_in') signedOut();
	});
	beforeNavigate((nav) => {
		if (!adopting || handed || nav.willUnload) return;
		if (session.s.state !== 'signed_in') return signedOut();
		if (!confirm(t(importKey('leaveAsk', flat)))) nav.cancel();
		else noteWhy('left');
	});

	// An import handed to the server before this page opened: shown until its parts finish, then
	// told like one this page sent. A part not answered this time leaves the note for the next visit.
	async function watchHanded(c: Cut, first: (JobView | null | undefined)[]) {
		adopting = handed = true;
		folderId = c.lf;
		// Held like a sending tab's, so other tabs leave this note be while it is followed here.
		const free = holdWhile(navigator.locks, wasKey);
		let jobs = first;
		try {
			while (!gone && jobs.some((j) => j && !finished(j))) {
				sent = { done: throughJobs(jobs), total: c.total };
				await new Promise((r) => setTimeout(r, 3000));
				// A poll not answered keeps what was last heard, so one blip never ends the watch.
				const was = jobs;
				jobs = await Promise.all(c.jobs.map((id, k) => api.get<JobView>(`/jobs/${id}`).catch((e) => (jobForgotten(e) ? null : was[k]))));
			}
			if (gone) return;
			const note = cutFromJobs(c, jobs);
			if (note === undefined) return;
			if (note) {
				cut = { ...note, lf: c.lf };
				return;
			}
			noteCut(null);
			const n = jobs.flatMap((j) => (j?.Result?.results as AdoptResult[] | undefined) ?? []).filter((x) => x.title_id && !x.error).length;
			toasts.push(t(n === 1 ? 'library.import.adoptedOne' : 'library.import.adoptedMany', { n: n.toLocaleString(appLocale()), word: w, words: ws }), n ? 'ok' : 'info', { body: t('library.import.noFileMoved') });
		} finally {
			free();
			adopting = handed = false;
			sent = null;
		}
	}

	function follow(id: number) {
		return followJob(
			(x) => (gone ? Promise.reject(new Error('')) : answered(api.get<JobView>(`/jobs/${x}`))),
			id,
			(j) => (job = j),
			{
				poke: (wake) =>
					live.on('job', (d) => {
						if ((d as { id?: number })?.id === id) wake();
					})
			}
		);
	}

	async function scan(after = '') {
		if (!folderId) return;
		scanFrom = after;
		retryFrom = null;
		cut = null;
		noteCut(null);
		error = '';
		plan = null;
		results = null;
		restFrom = '';
		q = '';
		filter = 'all';
		scanning = true;
		heard = Date.now();
		try {
			const r = await answered(api.post<{ job_id: number }>(`/library-folders/${folderId}/scan`, after ? { after } : undefined));
			noteScan({ lf: folderId, job: r.job_id, from: after });
			await watchScan(r.job_id);
		} catch (e) {
			error = scanEnd(e, false, t('core.api.notAnswering')).error;
			scanning = false;
		}
	}

	// Follows a scan to its plan; past the long-time line it keeps waiting. back: a scan picked up on the way back.
	async function watchScan(id: number, back = false) {
		scanning = true;
		heard = Date.now();
		try {
			let end: JobView;
			for (;;) {
				try {
					end = await askThrough(() => follow(id), () => gone);
					break;
				} catch (e) {
					if (gone || message(e) !== t('mediafiles.organise.slow')) throw e;
					error = message(e);
				}
			}
			error = '';
			// A stopped music scan keeps the artist folders it finished.
			const kept = end.State === 'cancelled' ? (end.Result?.plan as AdoptPlan | undefined) : undefined;
			if (end.State !== 'done' && !kept?.partial) throw new Error(end.Error?.message || t('library.import.scanUnfinished'));
			const p = (kept ?? end.Result?.plan) as AdoptPlan;
			plan = p;
			noteScan(null);
			const start = startPicks(p.folders);
			const s: Record<string, string> = {};
			for (const f of p.folders) if (f.match) s[f.folder] = matchKey(f.match);
			sel = s;
			named = {};
			ticked = Object.fromEntries(Object.keys(start).map((f) => [f, true]));
		} catch (e) {
			// A scan HolyArrs did not answer for is kept for the next visit; one it forgot, picked up on the way back, is simply gone.
			const left = scanEnd(e, back, t('core.api.notAnswering'));
			error = left.error;
			if (!gone && !left.keep) noteScan(null);
		} finally {
			scanning = false;
			job = null;
		}
	}

	function openLookup(f: AdoptCandidate) {
		lookFor = f.folder;
		term = f.parsed_name + (f.parsed_year ? ` ${f.parsed_year}` : '');
		found = [];
		lookError = '';
		// The folder name is already in the box: search it straight away.
		void lookup();
	}

	async function lookup(e?: SubmitEvent) {
		e?.preventDefault();
		if (!term.trim()) return;
		looking = true;
		lookError = '';
		try {
			if (kind === 'music') {
				const ask = () => api.get<{ artists: LookupArtist[]; pending?: boolean }>(`/artists/lookup?term=${encodeURIComponent(term.trim())}`);
				// A name never searched before answers "pending" while the search runs: ask again quietly.
				const artists = (await reaskWhileFilling(await ask(), ask, () => {})).artists;
				found = artists.map((a) => ({ key: `mbid:${a.mbid}`, name: a.disambiguation ? `${a.name} (${a.disambiguation})` : a.name, poster: a.image, title_id: a.title_id }));
				if (!found.length) lookError = t('library.import.nothingFound', { id: 'mbid:<id>' });
			} else if (kind === 'show') {
				const shows = (await api.get<{ shows: LookupShow[] }>(`/shows/lookup?term=${encodeURIComponent(term.trim())}`)).shows;
				found = shows.map((s) => ({ key: showIdOf(s), name: s.name, year: s.year, poster: s.poster, title_id: s.title_id })).filter((s) => s.key);
				if (!found.length) lookError = t('library.import.nothingFound', { id: 'tvdb:<id>' });
			} else {
				const m = term.trim().match(/^(.*?)\s+\(?((?:19|20)\d\d)\)?$/);
				const ask = m ? `term=${encodeURIComponent(m[1])}&year=${m[2]}` : `term=${encodeURIComponent(term.trim())}`;
				const movies = (await api.get<{ movies: LookupMovie[] }>(`/titles/lookup?${ask}`)).movies;
				found = movies.map((x) => ({ key: x.ids.tmdb ?? '', name: x.name, year: x.year, poster: x.poster, title_id: x.title_id })).filter((x) => x.key);
				if (!found.length) lookError = t('library.import.nothingFound', { id: 'tmdb:<id>' });
			}
		} catch (err) {
			lookError = message(err);
		} finally {
			looking = false;
		}
	}

	function use(folder: string, m: { key: string; name: string; year?: number; poster?: string }) {
		const id = m.key;
		if (!id) return;
		sel[folder] = id;
		named[id] = { name: m.name, year: m.year, poster: m.poster };
		ticked[folder] = true;
		lookFor = null;
	}

	async function adopt() {
		if (!plan || !picks.length) return;
		adopting = true;
		error = '';
		lost = false;
		heard = Date.now();
		const lf = plan.library_folder_id;
		const order = plan.folders.map((f) => f.folder);
		noteScan(null);
		markKey = cutKey;
		const free = holdWhile(navigator.locks, markKey);
		noteCut({ lf, total: picks.length, jobs: [], why: 'closed' });
		// A part whose Follow step failed after its folders went in: shown once the results are.
		let partFail = '';
		try {
			const all = await adoptInParts(
				picks,
				async (part) => {
					const r = await answered(api.post<{ job_id: number }>(`/library-folders/${lf}/adopt`, { folders: part, follow: followAll }));
					if (track) noteCut({ ...track, jobs: [...track.jobs, r.job_id] }, track.why === 'closed');
					// The server took the part: from here a lost answer means "not known", never "not sent".
					return async () => {
						const end = await keepAsking(() => follow(r.job_id), () => gone);
						const { res, fail } = partEnd(end, (m) => serverWords(m) || t('library.import.adoptUnfinished'));
						if (fail && !partFail) partFail = fail;
						return res;
					};
				},
				{ unknown: t('library.import.resultUnknown'), notSent: t('library.import.notSentRest') },
				(done, total) => {
					sent = { done, total };
					job = null;
				},
				() => gone || lost,
				() => (handed = true)
			);
			if (lost) signedOut();
			if (gone || lost) return;
			noteCut(null);
			// Notes of earlier imports of this folder that stopped say nothing true any more.
			try {
				const { store, held } = await notesThenLocks(localStorage, cutBase, navigator.locks);
				for (const k of staleCuts(store, cutBase, lf, Date.now(), held)) localStorage.removeItem(k);
			} catch {
				// No storage: nothing was kept.
			}
			// The ones not adopted come first, so a long list shows them.
			results = [...all].sort((a, b) => Number(!!a.title_id) - Number(!!b.title_id));
			error = partFail;
			const n = results.filter((x) => x.title_id && !x.error).length;
			toasts.push(t(n === 1 ? 'library.import.adoptedOne' : 'library.import.adoptedMany', { n: n.toLocaleString(appLocale()), word: w, words: ws }), n ? 'ok' : 'info', { body: t('library.import.noFileMoved') });
			retryFrom = scanFromFirstLeft(order, results, scanFrom);
			restFrom = plan?.more ? (plan.next ?? '') : '';
			plan = null;
		} catch (e) {
			error = message(e);
		} finally {
			free();
			adopting = false;
			handed = false;
			job = null;
			sent = null;
		}
	}

	// Scanning the rest clears the ticks here: ask first when some are ticked.
	function scanRest(next: string) {
		if (picks.length && !confirm(t(importKey(picks.length === 1 ? 'askScanRestOne' : 'askScanRestMany', flat), { n: picks.length.toLocaleString(appLocale()) }))) return;
		void scan(next);
	}
</script>

<svelte:window onbeforeunload={(e) => { if (adopting && !handed) e.preventDefault(); }} onpagehide={() => { if (adopting && track) noteCut(track, false); }} />

<svelte:head><title>{t('library.import.title')} · HolyArrs</title></svelte:head>

<LibraryToolPage title={t('library.import.title')}>

<p class="tp-msg good" data-testid="nothing-moved">
	<Icon name="shield" size={18} /><span>{t('library.import.stay')}</span>
</p>

<PageLoader loading={!loaded} label={t('library.import.loadingFolders')}>
{#if foldersFailed}
	<LoadError hasData={false} message={t('library.import.foldersFailed')} onretry={loadFolders} />
{:else if !folders.length}
	<EmptyState icon="folder" title={t('library.import.noFolder')} text={t('library.import.noFolderHint')} action={session.can('settings') ? { label: t('library.import.libraryFolders'), href: to('/settings/library-folders') } : undefined} />
{:else}
	<section class="tp-sheet tp-pad" aria-labelledby="h-pick">
		<h2 id="h-pick" class="h2">{t('library.byHand.step1')}</h2>
		<div class="row pick">
			<div class="field grow">
				<label for="lf">{t('library.import.libraryFolder')}</label>
				<select id="lf" class="input" bind:value={folderId} disabled={scanning || adopting}>
					{#each folders as f (f.id)}<option value={f.id}
							>{f.kind === 'show' ? t('library.import.showsPrefix') : f.kind === 'music' ? t('library.import.musicPrefix') : ''}{f.label ? `${f.label} (${f.path})` : f.path}</option
						>{/each}
				</select>
			</div>
			<Button variant="primary" icon="search" onclick={() => scan()} disabled={adopting || !folderId} busy={scanning}>{t('library.byHand.scan')}</Button>
		</div>
		{#if cut && !plan && !results && !scanning}
			{@const cutFlat = !!folders.find((f) => f.id === cut?.lf)?.flat}
			<p class="notice bad" role="status" data-testid="adopt-cut">{t(importKey(cut.key, cutFlat), { done: cut.done.toLocaleString(appLocale()), total: cut.total.toLocaleString(appLocale()) })}</p>
		{/if}
		{#if sent}
			{@const through = throughNow(sent, scanCount(job?.ProgressNote))}
			<div aria-live="polite" data-testid="adopt-progress">
				<div class="bar"><ProgressBar value={sent.total ? (through / sent.total) * 100 : 0} label={t('library.import.progress')} /></div>
				<p class="xs muted">{t('library.import.adoptCount', { done: through.toLocaleString(appLocale()), total: sent.total.toLocaleString(appLocale()) })} {t(handed ? 'library.import.closeOk' : 'library.import.keepOpen')}</p>
				{#if waiting}<p class="xs muted" role="status" data-testid="adopt-waiting">{t('library.import.waiting', { left: waiting.left.toLocaleString(appLocale()), min: waiting.min.toLocaleString(appLocale()) })}</p>{/if}
			</div>
		{:else if running}
			<div aria-live="polite">
				<div class="bar"><ProgressBar value={(job?.Progress ?? 0) * 100} label={t('library.import.progress')} /></div>
				<p class="xs muted">{scanned ? t(importKey('readCount', flat), { done: scanned.done.toLocaleString(appLocale()), total: scanned.total.toLocaleString(appLocale()) }) : job?.ProgressNote || (scanning ? t('library.import.reading', { word: w }) : t('library.import.adopting'))}</p>
			</div>
		{/if}
		{#if silent}<p class="xs bad-text" role="status" data-testid="not-answering">{t('library.import.stillTrying')}</p>{/if}
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<p class="xs muted"><Words key="library.import.noMatchFiles" links={{ a: to('/library/import-files') }} /></p>
	</section>

	{#if results}
		<section class="tp-sheet tp-pad" aria-labelledby="h-done" data-testid="adopt-results">
			<h2 id="h-done" class="h2">{t('library.import.done')}</h2>
			<p class="small">{t(okCount === 1 ? 'library.import.nowOne' : 'library.import.nowMany', { n: okCount.toLocaleString(appLocale()), word: w, words: ws })}</p>
			{#if results.length > okCount + unknownCount}<p class="small bad-text" data-testid="adopt-not-count">{t('library.import.notCount', { n: (results.length - okCount - unknownCount).toLocaleString(appLocale()) })}</p>{/if}
			{#if unknownCount}<p class="small" data-testid="adopt-unknown-count">{t('library.import.unknownCount', { n: unknownCount.toLocaleString(appLocale()) })}</p>{/if}
			{#if retryFrom !== null}<p class="small" data-testid="adopt-left">{t(importKey('leftHint', flat))}</p>{/if}
			<ul class="list">
				{#each results as r (r.folder)}
					<li>
						<span class="mono small path"><bdi>{r.folder}</bdi></span>
						{#if r.title_id}
							<a class="small" href={to(`/library/${r.title_id}`)}>{t('library.import.openThe', { word: w })}</a>
							{#if r.error}<span class="small bad-text" data-testid="adopt-added-problem">{serverWords(r.error)}</span>{/if}
							{#if r.episodes !== undefined || r.unknown?.length}
								<span class="xs muted"
									>{unit(kind === 'music' ? 'track' : 'episode', r.episodes ?? 0)}{r.unknown?.length
										? `, ${t('mediafiles.organise.filesLeftAlone', { files: unit('file', r.unknown.length) })}`
										: ''}</span
								>
							{/if}
						{:else if r.notKnown}
							<span class="small">{t('library.import.resultUnknown')}</span>
						{:else}
							<span class="small bad-text">{t('library.import.notAdopted', { why: serverWords(r.error) ?? '' })}</span>
						{/if}
					</li>
				{/each}
			</ul>
			<div class="row end">{#if retryFrom !== null}<Button variant="primary" onclick={() => scan(retryFrom ?? '')} disabled={scanning || adopting} data-testid="adopt-scan-again">{t('library.import.scanAgain')}</Button>{:else if restFrom}<Button onclick={() => scan(restFrom)} disabled={scanning || adopting}>{t('library.import.scanRest')}</Button>{/if}<Button href={to('/library')}>{t('library.import.goLibrary')}</Button></div>
		</section>
	{/if}

	{#if plan}
		<section class="tp-sheet tp-pad" aria-labelledby="h-check" data-testid="adopt-plan">
			<h2 id="h-check" class="h2">{t('library.import.step2')}</h2>
			<p class="small muted" data-testid="plan-count">
				{#if (plan.total ?? 0) > plan.folders.length}
					<Isolated text={t(importKey('foldersPart', flat), { total: (plan.total ?? 0).toLocaleString(appLocale()), path: ltr(plan.path), from: ((plan.offset ?? 0) + 1).toLocaleString(appLocale()), to: ((plan.offset ?? 0) + plan.folders.length).toLocaleString(appLocale()) })} />
				{:else}
					<Isolated text={t(importKey(plan.folders.length === 1 ? 'foldersInOne' : 'foldersInMany', flat), { n: plan.folders.length.toLocaleString(appLocale()), path: ltr(plan.path), words: ws })} />
				{/if}
			</p>
			{#if fail}<p class="notice bad" role="alert" data-testid="scan-warning">{t(fail.all ? 'library.import.lookFailAll' : 'library.import.lookFailSome', { n: fail.n.toLocaleString(appLocale()), total: fail.of.toLocaleString(appLocale()) })}</p>
			{:else if plan.warning}<p class="notice bad" role="alert" data-testid="scan-warning">{serverWords(plan.warning)}</p>{/if}
			{#if plan.kind !== 'show' && plan.kind !== 'music'}<TidyFolders folderId={plan.library_folder_id} loose={plan.loose_movies ?? 0} looseTop={plan.loose_top ?? 0} suggestFlat={!!plan.suggest_flat} onDone={() => scan()} />{/if}
			{#if !plan.folders.length}<p class="notice">{t(importKey('noFolders', flat), { word: w })}</p>{/if}
			{#if plan.more && (plan.partial || !fail?.every)}
				<div class="notice" data-testid="scan-more">
					{#if plan.partial}<p class="small"><Isolated text={t('library.import.stopped', { next: ltr(plan.next ?? '') })} /></p>{/if}
					{#if plan.folders.some(canAdopt)}<p class="small">{t('library.import.partNext')}</p>{/if}
					<Button onclick={() => scanRest(plan?.next ?? '')} disabled={scanning || adopting}>{t('library.import.scanRest')}</Button>
				</div>
			{/if}
			{#if plan.folders.length}
				<div class="checkbar" class:held={adopting} bind:clientWidth={planWidth} inert={adopting} aria-busy={adopting}>
					<ImportToolbar {phone} compact={phone || planWidth < 900} {view} {setView} {sizeItems} sizeLabel={view === 'posters' ? posterLabels[posterSize] : densityLabels[density]} {filters} bind:filter findLabel={t(importKey('searchFolders', flat))} bind:findOpen hasWords={!!q.trim()}>
						{#snippet find()}<DlFind bind:term={q} shown={shown.length} total={plan?.folders.length ?? 0} searchKey="" placeholder={t(importKey('searchFolders', flat))} note={t(importKey(shown.length === 1 ? 'folderCountOne' : 'folderCountMany', flat), { n: shown.length.toLocaleString(appLocale()) })} testid="import-find" />{/snippet}
					</ImportToolbar>
					{#if shown.length}
						<div class="tickbar" data-testid="import-tickbar">
							<span class="count" aria-live="polite">{t('mediafiles.select.count', { n: tickNow.on.toLocaleString(appLocale()) })}</span>
							{#if tickNow.can > 0}<Button variant="plain" size="md" onclick={tickShown} data-testid="import-select-all">{allTicked ? t('mediafiles.select.none') : t('mediafiles.select.all', { n: tickNow.can.toLocaleString(appLocale()) })}</Button>{/if}
						</div>
						<ImportList {flat} folders={shown} {view} size={posterSize} {density} {kind} {phone} bind:sel bind:ticked {named} onlook={openLookup} onzoom={(f) => (zoom = f)} />
					{:else}
						<p class="notice" data-testid="import-none">{t('library.import.noneMatch')}</p>
					{/if}
				</div>
			{/if}
			<div class="row end">
				<label class="checkline follow-all" data-testid="follow-box">
					<input type="checkbox" bind:checked={followAll} disabled={adopting} />
					<span><b>{t('library.import.follow')}</b><br /><span class="hint">{t('library.import.followHint')}</span></span>
				</label>
				<span class="small muted">{t('library.byHand.ticked', { n: picks.length.toLocaleString(appLocale()) })}</span>
				<Button variant="primary" onclick={adopt} disabled={!picks.length} busy={adopting}>
					{t(picks.length === 1 ? 'library.import.adoptOne' : 'library.import.adoptMany', { n: picks.length.toLocaleString(appLocale()), word: w, words: ws })}
				</Button>
			</div>
		</section>
	{/if}
{/if}
</PageLoader>
</LibraryToolPage>

{#if lookFolder}
	<Sheet open title={t('library.import.lookUpFor', { word: w, folder: ltr(lookFolder.folder) })} size="md" placement="top" onclose={() => (lookFor = null)}>
		<form class="look" onsubmit={lookup} data-testid="import-lookup">
			<label class="sr-only" for="look-term"><Isolated text={t('library.import.lookUpFor', { word: w, folder: ltr(lookFolder.folder) })} /></label>
			<!-- svelte-ignore a11y_autofocus -->
			<input id="look-term" class="input" bind:value={term} placeholder={kind === 'show' ? t('library.import.phShow') : kind === 'music' ? t('library.import.phMusic') : t('library.import.phMovie')} autocomplete="off" autofocus />
			<Button type="submit" busy={looking}>{t('library.byHand.lookUp')}</Button>
		</form>
		{#if lookError}<p class="form-error" role="alert">{lookError}</p>{/if}
		{#if found.length}
			<ul class="found">
				{#each found as m (m.key)}
					<li>
						<span class="small">{m.name}{m.year ? ` (${m.year})` : ''}</span>
						{#if m.title_id}
							<span class="xs muted">{t('mediafiles.adopt.inLibrary')}</span>
						{:else}
							<Button variant="plain" onclick={() => lookFolder && use(lookFolder.folder, m)}>{t('library.byHand.useThis')}</Button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</Sheet>
{/if}

{#if zoom}
	{@const p = pickedOf(zoom, sel[zoom.folder], named)}
	<PosterLightbox art={p.poster} title={`${p.name}${p.year ? ` (${p.year})` : ''}`} sub={[zoom.folder, zoom.main ? fileName(zoom.main) : ''].filter(Boolean).join(' · ')} square={kind === 'music'} onclose={() => (zoom = null)} />
{/if}

<style>
	:global(.tp) .h2 { margin: 0 0 12px; font: var(--t-title3); }
	.notice { display: flex; gap: 10px; align-items: flex-start; margin: 0; padding: 12px 14px; border: 0; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-callout); }
	.notice.bad { color: var(--fail); }
	.notice[data-testid='scan-more'] { flex-direction: column; }
	.pick {
		align-items: flex-end;
		gap: var(--s-3);
		flex-wrap: wrap;
		padding: var(--s-3);
		border-radius: var(--r-btn);
		background: var(--surface-2);
	}
	.grow {
		flex: 1 1 240px;
		margin-bottom: 0;
	}
	.follow-all { margin-inline-end: auto; }
	.bar {
		width: 100%;
		margin-top: var(--s-4);
	}
	/* The toolbar and the folders sit a step under the intro, as on the Library page. */
	.checkbar { display: flex; flex-direction: column; gap: 12px; min-width: 0; margin-top: 14px; }
	.checkbar.held { opacity: 0.6; }
	.tickbar { display: flex; align-items: center; gap: 6px; min-width: 0; }
	.tickbar .count { padding: 0 8px; font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.tickbar :global(.ui-btn.plain) { padding-inline: 6px; }
	.list,
	.found {
		list-style: none;
		padding: 0;
		margin: var(--s-2) 0 0;
	}
	.list { border: 1px solid var(--line); border-radius: var(--r-btn); background: var(--surface-1); overflow: hidden; }
	.list > li {
		display: flex;
		flex-direction: column;
		gap: var(--s-1);
		padding: var(--s-3) var(--s-4);
	}
	.list > li + li { border-top: 1px solid var(--line); }
	.path {
		overflow-wrap: anywhere;
	}
	.look {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-2);
	}
	select.input { min-height: 40px; border-radius: 12px; border-color: color-mix(in srgb, var(--text) 10%, transparent); background: color-mix(in srgb, var(--text) 5%, transparent); font: 400 14.5px/1.3 var(--font-ui); }
	@media (pointer: coarse) { select.input { min-height: 44px; } }
	.look .input {
		flex: 1 1 200px;
	}
	.found li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: var(--s-2);
		min-height: 44px;
		padding: var(--s-1) 0;
	}
	.found li + li { border-top: 1px solid var(--line); }
	.end {
		justify-content: flex-end;
		align-items: center;
		gap: var(--s-3);
		margin-top: var(--s-5);
	}
	.bad-text {
		color: var(--fail);
	}
	@media (max-width: 600px) {
		.pick > :global(.ui-btn), .look > :global(.ui-btn), .end > :global(.ui-btn) { width: 100%; }
	}
</style>
